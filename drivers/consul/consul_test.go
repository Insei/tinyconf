package consul

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/yaml"
)

func init() {
	retryInterval = time.Millisecond
}

func str(s string) *string {
	return &s
}

type server struct {
	Host string `consul:"host"`
	Port int    `consul:"port"`
}

type testConfig struct {
	Name     string            `consul:"name" doc:"application name"`
	Server   server            `consul:"server"`
	Hosts    []string          `consul:"hosts"`
	Labels   map[string]string `consul:"labels"`
	Password string            `consul:"password" hidden:"true"`
	NoTag    string
	Nested   struct {
		Level string `consul:"level"`
	}
}

func newManager(t *testing.T, driver tinyconf.Driver) *tinyconf.Manager {
	t.Helper()
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	return manager
}

func TestDriver_Parse(t *testing.T) {
	consul := newFakeConsul(map[string]string{
		"app/name":         "app",
		"app/server/host":  "localhost",
		"app/server/port":  "8080",
		"app/hosts":        "a,b",
		"app/labels":       "env:prod",
		"app/password":     "secret",
		"app/level":        "debug",
		"application/name": "other",
	})
	defer consul.Close()
	// folders have no values
	consul.values["app/server/"] = nil

	driver, err := New("/app/", WithAddress(consul.URL))
	assert.NoError(t, err)
	assert.Equal(t, "consul", driver.GetName())
	manager := newManager(t, driver)
	conf := &testConfig{}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))

	assert.Equal(t, "app", conf.Name)
	assert.Equal(t, server{Host: "localhost", Port: 8080}, conf.Server)
	assert.Equal(t, []string{"a", "b"}, conf.Hosts)
	assert.Equal(t, map[string]string{"env": "prod"}, conf.Labels)
	assert.Equal(t, "secret", conf.Password)
	// the parent without the tag is skipped in the key
	assert.Equal(t, "debug", conf.Nested.Level)
	assert.Equal(t, []tinyconf.FieldSource{
		{Field: "Name", Driver: "consul", Source: "app/name"},
		{Field: "Server.Host", Driver: "consul", Source: "app/server/host"},
		{Field: "Server.Port", Driver: "consul", Source: "app/server/port"},
		{Field: "Hosts", Driver: "consul", Source: "app/hosts"},
		{Field: "Labels", Driver: "consul", Source: "app/labels"},
		{Field: "Password", Driver: "consul", Source: "app/password"},
		{Field: "Nested.Level", Driver: "consul", Source: "app/level"},
	}, manager.Sources(conf))
	// all keys are read by one request
	assert.Len(t, consul.requests, 1)
	assert.Equal(t, "/v1/kv/app/", consul.lastRequest().URL.Path)
	assert.Equal(t, "true", consul.lastRequest().URL.Query().Get("recurse"))
}

func TestDriver_GetValue(t *testing.T) {
	consul := newFakeConsul(map[string]string{"port": "not a number", "server/host": "localhost", "level": "debug"})
	defer consul.Close()
	storage, err := fmap.Get[testConfig]()
	assert.NoError(t, err)

	// without the prefix keys are read from the root, values are loaded on the first call without Manager
	driver, err := New("", WithAddress(consul.URL))
	assert.NoError(t, err)
	val, err := driver.GetValue(storage.MustFind("Server.Host"))
	assert.NoError(t, err)
	assert.Equal(t, &tinyconf.Value{Source: "server/host", Value: "localhost"}, val)

	_, err = driver.GetValue(storage.MustFind("Name"))
	assert.ErrorIs(t, err, tinyconf.ErrValueNotFound)
	_, err = driver.GetValue(storage.MustFind("NoTag"))
	assert.ErrorIs(t, err, tinyconf.ErrIncorrectTagSettings)

	val, err = driver.GetValue(storage.MustFind("Nested.Level"))
	assert.NoError(t, err)
	assert.Equal(t, "parent fields Nested have no 'consul' tag, the value is read from the 'level' key", val.Warning)

	type portConfig struct {
		Port int `consul:"port"`
	}
	portStorage, err := fmap.Get[portConfig]()
	assert.NoError(t, err)
	_, err = driver.GetValue(portStorage.MustFind("Port"))
	assert.ErrorContains(t, err, "failed to parse consul value from key port for Port config field")
}

func TestDriver_Errors(t *testing.T) {
	consul := newFakeConsul(nil)
	defer consul.Close()
	storage, err := fmap.Get[testConfig]()
	assert.NoError(t, err)

	t.Run("status", func(t *testing.T) {
		consul.setFails(1)
		driver, err := New("app", WithAddress(consul.URL))
		assert.NoError(t, err)
		_, err = driver.GetValue(storage.MustFind("Name"))
		assert.ErrorContains(t, err, "failed to read app/: 500 Internal Server Error: unavailable")
	})
	t.Run("token", func(t *testing.T) {
		consul.token = "token"
		defer func() { consul.token = "" }()
		driver, err := New("app", WithAddress(consul.URL))
		assert.NoError(t, err)
		_, err = driver.GetValue(storage.MustFind("Name"))
		assert.ErrorContains(t, err, "403 Forbidden: Permission denied")
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)

		driver, err = New("app", WithAddress(consul.URL), WithToken("token"), WithDatacenter("dc2"))
		assert.NoError(t, err)
		_, err = driver.GetValue(storage.MustFind("Name"))
		assert.ErrorIs(t, err, tinyconf.ErrValueNotFound)
		assert.Equal(t, "dc2", consul.lastRequest().URL.Query().Get("dc"))
	})
	t.Run("keys hidden by ACL", func(t *testing.T) {
		srv := newFakeConsul(map[string]string{"app/name": "app", "app/password": "secret", "empty/": ""})
		defer srv.Close()
		srv.denied = []string{"app/password"}
		driver, err := New("app", WithAddress(srv.URL))
		assert.NoError(t, err)
		_, err = driver.GetValue(storage.MustFind("Name"))
		assert.EqualError(t, err, "access denied: failed to read app/: some keys are hidden by ACL, the token has no permission to read them")
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)

		// without the permission to read the prefix the empty list is returned, the prefix is checked
		srv.denied = []string{"app/"}
		driver, err = New("app", WithAddress(srv.URL))
		assert.NoError(t, err)
		_, err = driver.GetValue(storage.MustFind("Name"))
		assert.ErrorContains(t, err, "failed to read app/: 403 Forbidden: Permission denied")
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)

		// the prefix without keys is not an error
		srv.denied = nil
		for _, prefix := range []string{"absent", "empty"} {
			driver, err = New(prefix, WithAddress(srv.URL))
			assert.NoError(t, err)
			_, err = driver.GetValue(storage.MustFind("Name"))
			assert.ErrorIs(t, err, tinyconf.ErrValueNotFound)
		}
	})
	t.Run("invalid response", func(t *testing.T) {
		srv := newFakeConsul(nil)
		defer srv.Close()
		srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not json"))
		})
		driver, err := New("app", WithAddress(srv.URL))
		assert.NoError(t, err)
		_, err = driver.GetValue(storage.MustFind("Name"))
		assert.ErrorContains(t, err, "failed to decode response of app/")
	})
	t.Run("unavailable", func(t *testing.T) {
		driver, err := New("app", WithAddress("127.0.0.1:1"))
		assert.NoError(t, err)
		_, err = driver.GetValue(storage.MustFind("Name"))
		assert.Error(t, err)
	})
	t.Run("invalid address", func(t *testing.T) {
		driver, err := New("app", WithAddress("http://consul\n"))
		assert.NoError(t, err)
		_, err = driver.GetValue(storage.MustFind("Name"))
		assert.Error(t, err)
	})
}

func TestNewClient(t *testing.T) {
	t.Setenv("CONSUL_HTTP_ADDR", "")
	t.Setenv("CONSUL_HTTP_TOKEN", "")
	t.Setenv("CONSUL_HTTP_SSL", "")
	c := newClient(nil)
	assert.Equal(t, "http://127.0.0.1:8500", c.address)
	assert.Equal(t, "", c.token)
	assert.Equal(t, DefaultWaitTime, c.waitTime)
	assert.Equal(t, http.DefaultClient, c.httpClient)

	t.Setenv("CONSUL_HTTP_ADDR", "consul:8501")
	t.Setenv("CONSUL_HTTP_TOKEN", "env-token")
	t.Setenv("CONSUL_HTTP_SSL", "true")
	c = newClient(nil)
	assert.Equal(t, "https://consul:8501", c.address)
	assert.Equal(t, "env-token", c.token)

	httpClient := &http.Client{}
	c = newClient([]Option{WithAddress("http://localhost:8500/"), WithToken("token"), WithHTTPClient(httpClient), WithWaitTime(time.Second)})
	assert.Equal(t, "http://localhost:8500", c.address)
	assert.Equal(t, "token", c.token)
	assert.Equal(t, httpClient, c.httpClient)
	assert.Equal(t, time.Second, c.waitTime)
}

func TestDriver_GenDoc(t *testing.T) {
	driver, err := New("app")
	assert.NoError(t, err)
	manager := newManager(t, driver)
	conf := &testConfig{
		Name:     "my app",
		Server:   server{Host: "localhost", Port: 8080},
		Hosts:    []string{"a", "b"},
		Password: "secret",
		Nested: struct {
			Level string `consul:"level"`
		}{Level: "it's"},
	}
	assert.NoError(t, manager.Register(conf))
	type other struct {
		Name string `consul:"name"`
	}
	assert.NoError(t, manager.Register(&other{}))
	assert.Equal(t, `# application name
consul kv put app/name 'my app'
consul kv put app/server/host localhost
consul kv put app/server/port 8080
consul kv put app/hosts a,b
consul kv put app/labels ''
consul kv put app/password ''
`, manager.GenDoc("consul"))
	assert.Equal(t, `'it'\''s'`, shellQuote("it's"))
}

func TestDriver_Watch(t *testing.T) {
	consul := newFakeConsul(map[string]string{"app/name": "first"})
	defer consul.Close()
	driver, err := New("app", WithAddress(consul.URL), WithWaitTime(time.Minute))
	assert.NoError(t, err)
	manager := newManager(t, driver)
	conf := &testConfig{}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))

	ctx, cancel := context.WithCancel(context.Background())
	changes := make(chan *testConfig)
	done := make(chan error)
	go func() {
		done <- manager.Watch(ctx, conf, func(c any, err error) {
			assert.NoError(t, err)
			changes <- c.(*testConfig)
		})
	}()

	// the change made after Parse and before Watch is reported too
	consul.put("app/name", str("second"))
	assert.Equal(t, "second", (<-changes).Name)

	consul.put("app/server/port", str("9090"))
	changed := <-changes
	assert.Equal(t, "second", changed.Name)
	assert.Equal(t, 9090, changed.Server.Port)

	// keys out of the prefix are not watched;
	// the deleted key gets the value from the config passed to Watch
	consul.put("other/name", str("other"))
	consul.put("app/server/port", nil)
	changed = <-changes
	assert.Equal(t, "second", changed.Name)
	assert.Equal(t, 0, changed.Server.Port)

	cancel()
	assert.NoError(t, <-done)
}

func TestDriver_WatchAfterFailedLoad(t *testing.T) {
	consul := newFakeConsul(map[string]string{"app/name": "consul", "app/config": "name: document\n"})
	defer consul.Close()
	type config struct {
		Name     string `consul:"name"`
		Document string `yaml:"name"`
	}
	consulDriver, err := New("app", WithAddress(consul.URL))
	assert.NoError(t, err)
	yamlDriver, err := yaml.New("", yaml.WithSource(Document("app/config", WithAddress(consul.URL))))
	assert.NoError(t, err)
	manager, err := tinyconf.New(tinyconf.WithDriver(consulDriver), tinyconf.WithDriver(yamlDriver))
	assert.NoError(t, err)
	conf := &config{}
	assert.NoError(t, manager.Register(conf))
	// Consul is not available at the start
	consul.setFails(2)
	assert.Error(t, manager.Parse(conf))
	assert.Equal(t, config{}, *conf)

	ctx, cancel := context.WithCancel(context.Background())
	changes := make(chan *config, 10)
	done := make(chan error)
	go func() {
		done <- manager.Watch(ctx, conf, func(c any, err error) {
			if err == nil {
				changes <- c.(*config)
			}
		})
	}()
	// Consul is available, the values are loaded without changes of the keys
	assert.Equal(t, config{Name: "consul", Document: "document"}, *<-changes)
	cancel()
	assert.NoError(t, <-done)
}

func TestClient_Watch(t *testing.T) {
	consul := newFakeConsul(map[string]string{"key": "value"})
	defer consul.Close()
	c := newClient([]Option{WithAddress(consul.URL), WithWaitTime(10 * time.Millisecond)})

	t.Run("loaded index", func(t *testing.T) {
		_, index, err := c.load("key", false)
		assert.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		changes := make(chan struct{}, 10)
		done := make(chan error)
		go func() { done <- c.watch(ctx, "key", false, index, func() { changes <- struct{}{} }) }()
		// the wait time is over without changes
		time.Sleep(50 * time.Millisecond)
		assert.Len(t, changes, 0)
		consul.put("key", str("new"))
		<-changes
		cancel()
		assert.NoError(t, <-done)
	})
	t.Run("without loaded index", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		changes := make(chan struct{}, 10)
		done := make(chan error)
		go func() { done <- c.watch(ctx, "key", false, 0, func() { changes <- struct{}{} }) }()
		// the data is not in the config, it is reported at once
		<-changes
		cancel()
		assert.NoError(t, <-done)
	})
	t.Run("failed requests are retried", func(t *testing.T) {
		consul := newFakeConsul(map[string]string{"key": "value"})
		defer consul.Close()
		c := newClient([]Option{WithAddress(consul.URL), WithWaitTime(10 * time.Millisecond)})
		_, index, err := c.load("key", false)
		assert.NoError(t, err)
		consul.setFails(2)
		ctx, cancel := context.WithCancel(context.Background())
		changes := make(chan struct{}, 10)
		done := make(chan error)
		go func() { done <- c.watch(ctx, "key", false, index, func() { changes <- struct{}{} }) }()
		// the index is the same, but the data could be changed while Consul was not available
		<-changes
		consul.mu.Lock()
		indexes := make([]string, 0, len(consul.requests))
		for _, r := range consul.requests[:4] {
			indexes = append(indexes, r.URL.Query().Get("index"))
		}
		consul.mu.Unlock()
		// load, failed waiting, failed and successful requests of the current state without waiting
		assert.Equal(t, []string{"", fmt.Sprint(index), "", ""}, indexes)
		cancel()
		assert.NoError(t, <-done)
	})
	t.Run("access denied stops watching", func(t *testing.T) {
		srv := newFakeConsul(nil)
		defer srv.Close()
		srv.denied = []string{"key"}
		c := newClient([]Option{WithAddress(srv.URL)})
		err := c.watch(context.Background(), "key", false, 1, func() {})
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
	})
	t.Run("zero index of the response", func(t *testing.T) {
		srv := newFakeConsul(nil)
		defer srv.Close()
		var mu sync.Mutex
		var indexes []string
		srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			indexes = append(indexes, r.URL.Query().Get("index"))
			w.WriteHeader(http.StatusNotFound)
		})
		c := newClient([]Option{WithAddress(srv.URL)})
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()
		assert.NoError(t, c.watch(ctx, "key", false, 5, func() {}))
		// the index is reset to 1 to keep waiting for changes
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, "5", indexes[0])
		assert.Equal(t, "1", indexes[1])
	})
	t.Run("canceled while retrying", func(t *testing.T) {
		original := retryInterval
		retryInterval = time.Hour
		defer func() { retryInterval = original }()
		consul.setFails(1)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()
		assert.NoError(t, c.watch(ctx, "key", false, 1, func() {}))
	})
	t.Run("retry interval limit", func(t *testing.T) {
		originalRetry, originalMax := retryInterval, maxRetryInterval
		retryInterval, maxRetryInterval = 2*time.Millisecond, 3*time.Millisecond
		defer func() { retryInterval, maxRetryInterval = originalRetry, originalMax }()
		consul.setFails(3)
		ctx, cancel := context.WithCancel(context.Background())
		changes := make(chan struct{}, 10)
		done := make(chan error)
		go func() { done <- c.watch(ctx, "key", false, 1, func() { changes <- struct{}{} }) }()
		<-changes
		cancel()
		assert.NoError(t, <-done)
	})
}

func TestDocument(t *testing.T) {
	type yamlServer struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	}
	type config struct {
		Name   string     `yaml:"name"`
		Server yamlServer `yaml:"server"`
	}
	consul := newFakeConsul(map[string]string{"app/config": "name: consul\nserver:\n  port: 9090\n"})
	defer consul.Close()

	file := t.TempDir() + "/config.yaml"
	assert.NoError(t, os.WriteFile(file, []byte("name: file\nserver:\n  host: localhost\n  port: 8080\n"), 0o600))
	driver, err := yaml.New(file, yaml.WithSource(Document("/app/config", WithAddress(consul.URL))))
	assert.NoError(t, err)
	manager := newManager(t, driver)
	conf := &config{}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))
	// the document is merged with the file by the yaml tags
	assert.Equal(t, config{Name: "consul", Server: yamlServer{Host: "localhost", Port: 9090}}, *conf)

	ctx, cancel := context.WithCancel(context.Background())
	changes := make(chan *config)
	done := make(chan error)
	go func() {
		done <- manager.Watch(ctx, conf, func(c any, err error) {
			assert.NoError(t, err)
			changes <- c.(*config)
		})
	}()
	consul.put("app/config", str("name: changed\n"))
	assert.Equal(t, config{Name: "changed", Server: yamlServer{Host: "localhost", Port: 8080}}, *<-changes)
	cancel()
	assert.NoError(t, <-done)
}

func TestDocument_Absent(t *testing.T) {
	type config struct {
		Name string `yaml:"name"`
	}
	consul := newFakeConsul(nil)
	defer consul.Close()

	driver, err := yaml.New("", yaml.WithSource(Document("app/config", WithAddress(consul.URL))))
	assert.NoError(t, err)
	manager := newManager(t, driver)
	conf := &config{}
	assert.NoError(t, manager.Register(conf))
	err = manager.Parse(conf)
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.ErrorContains(t, err, "failed to load consul key app/config")

	driver, err = yaml.New("", yaml.WithOptional(), yaml.WithSource(Document("app/config", WithAddress(consul.URL))))
	assert.NoError(t, err)
	manager = newManager(t, driver)
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))

	consul.setFails(1)
	err = manager.Parse(conf)
	assert.ErrorContains(t, err, "500 Internal Server Error")
	assert.False(t, errors.Is(err, fs.ErrNotExist))
}
