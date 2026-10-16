package etcd

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
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
	Host string `etcd:"host"`
	Port int    `etcd:"port"`
}

type testConfig struct {
	Name     string            `etcd:"name" doc:"application name"`
	Server   server            `etcd:"server"`
	Hosts    []string          `etcd:"hosts"`
	Labels   map[string]string `etcd:"labels"`
	Password string            `etcd:"password" hidden:"true"`
	NoTag    string
	Nested   struct {
		Level string `etcd:"level"`
	}
}

func newManager(t *testing.T, driver tinyconf.Driver) *tinyconf.Manager {
	t.Helper()
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	return manager
}

func parse(t *testing.T, driver tinyconf.Driver) (*testConfig, *tinyconf.Manager, error) {
	t.Helper()
	manager := newManager(t, driver)
	conf := &testConfig{}
	assert.NoError(t, manager.Register(conf))
	return conf, manager, manager.Parse(conf)
}

func TestDriver_Parse(t *testing.T) {
	etcd := newFakeEtcd(map[string]string{
		"app/name":         "app",
		"app/server/host":  "localhost",
		"app/server/port":  "8080",
		"app/hosts":        "a,b",
		"app/labels":       "env:prod",
		"app/password":     "secret",
		"app/level":        "debug",
		"application/name": "other",
	})
	t.Cleanup(etcd.Close)

	driver, err := New("/app/", WithEndpoints(etcd.URL))
	assert.NoError(t, err)
	assert.Equal(t, "etcd", driver.GetName())
	conf, manager, err := parse(t, driver)
	assert.NoError(t, err)
	assert.Equal(t, testConfig{
		Name:     "app",
		Server:   server{Host: "localhost", Port: 8080},
		Hosts:    []string{"a", "b"},
		Labels:   map[string]string{"env": "prod"},
		Password: "secret",
		Nested: struct {
			Level string `etcd:"level"`
		}{Level: "debug"},
	}, *conf)
	assert.Contains(t, manager.Sources(conf), tinyconf.FieldSource{Field: "Server.Port", Driver: "etcd", Source: "app/server/port"})
	// all keys are read by one request
	assert.Equal(t, []string{"/v3/kv/range"}, etcd.paths())
}

func TestDriver_GetValue(t *testing.T) {
	etcd := newFakeEtcd(map[string]string{"port": "not a number", "server/host": "localhost", "level": "debug"})
	t.Cleanup(etcd.Close)
	storage, err := fmap.Get[testConfig]()
	assert.NoError(t, err)

	// without the prefix keys are read from the root, values are loaded on the first call without Manager
	driver, err := New("", WithEndpoints(etcd.URL))
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
	assert.Equal(t, "parent fields Nested have no 'etcd' tag, the value is read from the 'level' key", val.Warning)

	type portConfig struct {
		Port int `etcd:"port"`
	}
	portStorage, err := fmap.Get[portConfig]()
	assert.NoError(t, err)
	_, err = driver.GetValue(portStorage.MustFind("Port"))
	assert.ErrorContains(t, err, "failed to parse etcd value from key port for Port config field")
}

func TestDriver_Errors(t *testing.T) {
	storage, err := fmap.Get[testConfig]()
	assert.NoError(t, err)
	getValue := func(opts ...Option) error {
		driver, err := New("app", opts...)
		assert.NoError(t, err)
		_, err = driver.GetValue(storage.MustFind("Name"))
		return err
	}

	t.Run("permission denied", func(t *testing.T) {
		etcd := newFakeEtcd(nil)
		t.Cleanup(etcd.Close)
		etcd.denied = []string{"app/"}
		err := getValue(WithEndpoints(etcd.URL))
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
		assert.EqualError(t, err, "access denied: failed to read app/: etcdserver: permission denied")
	})
	t.Run("not etcd", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v3/kv/range" && r.Header.Get("X-Status") == "" {
				_, _ = w.Write([]byte("not json"))
				return
			}
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}))
		defer srv.Close()
		assert.ErrorContains(t, getValue(WithEndpoints(srv.URL)), "failed to read app/")
		assert.ErrorContains(t, getValue(WithEndpoints(srv.URL), WithAuth("user", "password")),
			"failed to authenticate user user: 503 Service Unavailable: unavailable")
	})
	t.Run("unavailable", func(t *testing.T) {
		assert.ErrorContains(t, getValue(WithEndpoints("127.0.0.1:1")), "failed to read app/")
		assert.ErrorContains(t, getValue(WithEndpoints("127.0.0.1:1"), WithAuth("user", "password")), "connection refused")
	})
	t.Run("invalid endpoint", func(t *testing.T) {
		assert.Error(t, getValue(WithEndpoints("http://et cd:2379")))
	})
	t.Run("invalid token response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}))
		defer srv.Close()
		assert.ErrorContains(t, getValue(WithEndpoints(srv.URL), WithAuth("user", "password")), "failed to decode the token")
	})
}

func TestDriver_Auth(t *testing.T) {
	etcd := newFakeEtcd(map[string]string{"app/name": "app"})
	t.Cleanup(etcd.Close)
	etcd.users = map[string]string{"user": "password"}

	// authentication is required
	_, _, err := parse(t, must(New("app", WithEndpoints(etcd.URL))))
	assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
	assert.ErrorContains(t, err, "user name is empty")

	_, _, err = parse(t, must(New("app", WithEndpoints(etcd.URL), WithAuth("user", "wrong"))))
	assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
	assert.ErrorContains(t, err, "failed to authenticate user user")

	since := func() func() []string {
		n := len(etcd.paths())
		return func() []string { return etcd.paths()[n:] }
	}
	requests := since()
	driver := must(New("app", WithEndpoints(etcd.URL), WithAuth("user", "password")))
	conf, manager, err := parse(t, driver)
	assert.NoError(t, err)
	assert.Equal(t, "app", conf.Name)
	// the token is used for the next requests
	assert.NoError(t, manager.Parse(conf))
	assert.Equal(t, []string{"/v3/auth/authenticate", "/v3/kv/range", "/v3/kv/range"}, requests())

	// the expired token is taken again once
	requests = since()
	etcd.set(func(f *fakeEtcd) { f.tokens = map[string]bool{} })
	assert.NoError(t, manager.Parse(conf))
	assert.Equal(t, []string{"/v3/kv/range", "/v3/auth/authenticate", "/v3/kv/range"}, requests())

	// the token is expired and the password is changed
	etcd.set(func(f *fakeEtcd) {
		f.tokens = map[string]bool{}
		f.users["user"] = "changed"
	})
	err = manager.Parse(conf)
	assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
	assert.ErrorContains(t, err, "failed to authenticate user user")

	// authentication is not enabled, but the user is set
	etcd.set(func(f *fakeEtcd) { f.users = nil })
	_, _, err = parse(t, must(New("app", WithEndpoints(etcd.URL), WithAuth("user", "password"))))
	assert.ErrorContains(t, err, "authentication is not enabled")
	assert.NotErrorIs(t, err, tinyconf.ErrAccessDenied)
}

func must(driver tinyconf.Driver, err error) tinyconf.Driver {
	if err != nil {
		panic(err)
	}
	return driver
}

func TestDriver_Endpoints(t *testing.T) {
	etcd := newFakeEtcd(map[string]string{"app/name": "app"})
	t.Cleanup(etcd.Close)
	// the endpoint that is not available is skipped
	conf, manager, err := parse(t, must(New("app", WithEndpoints("127.0.0.1:1", etcd.URL))))
	assert.NoError(t, err)
	assert.Equal(t, "app", conf.Name)
	// the endpoint that has answered is used first
	etcd.Close()
	assert.Error(t, manager.Parse(conf))
}

func TestNewClient(t *testing.T) {
	t.Setenv("ETCDCTL_ENDPOINTS", "")
	t.Setenv("ETCDCTL_USER", "")
	t.Setenv("ETCDCTL_PASSWORD", "")
	c := newClient(nil)
	assert.Equal(t, []string{DefaultEndpoint}, c.endpoints)
	assert.Equal(t, "", c.username)
	assert.Equal(t, http.DefaultClient, c.httpClient)

	t.Setenv("ETCDCTL_ENDPOINTS", "etcd-1:2379, https://etcd-2:2379/,")
	t.Setenv("ETCDCTL_USER", "user:password")
	c = newClient(nil)
	assert.Equal(t, []string{"http://etcd-1:2379", "https://etcd-2:2379"}, c.endpoints)
	assert.Equal(t, "user", c.username)
	assert.Equal(t, "password", c.password)

	t.Setenv("ETCDCTL_USER", "user")
	t.Setenv("ETCDCTL_PASSWORD", "other")
	c = newClient(nil)
	assert.Equal(t, "other", c.password)

	httpClient := &http.Client{}
	c = newClient([]Option{WithEndpoints("https://etcd:2379"), WithAuth("root", "root"), WithHTTPClient(httpClient)})
	assert.Equal(t, []string{"https://etcd:2379"}, c.endpoints)
	assert.Equal(t, "root", c.username)
	assert.Equal(t, httpClient, c.httpClient)
}

func TestRangeOf(t *testing.T) {
	tests := []struct {
		key        string
		prefix     bool
		start, end string
	}{
		{"app/name", false, "app/name", ""},
		{"", true, "\x00", "\x00"},
		{"app/", true, "app/", "app0"},
		{"a\xff", true, "a\xff", "b"},
		{"\xff\xff", true, "\xff\xff", "\x00"},
	}
	for _, tt := range tests {
		start, end := rangeOf(tt.key, tt.prefix)
		assert.Equal(t, tt.start, string(start), tt.key)
		assert.Equal(t, tt.end, string(end), tt.key)
	}
}

func TestDriver_GenDoc(t *testing.T) {
	driver := must(New("app"))
	manager := newManager(t, driver)
	assert.NoError(t, manager.Register(&testConfig{
		Name:     "my app",
		Server:   server{Host: "localhost", Port: 8080},
		Hosts:    []string{"a", "b"},
		Password: "secret",
	}))
	assert.Equal(t, `# application name
etcdctl put -- app/name 'my app'
etcdctl put -- app/server/host localhost
etcdctl put -- app/server/port 8080
etcdctl put -- app/hosts a,b
etcdctl put -- app/labels ''
etcdctl put -- app/password ''
`, manager.GenDoc("etcd"))
	assert.Equal(t, `'it'\''s'`, shellQuote("it's"))

	// the same keys of several configs are generated once
	type other struct {
		Name string `etcd:"name"`
	}
	assert.NoError(t, manager.Register(&other{}))
	assert.Equal(t, 1, strings.Count(manager.GenDoc("etcd"), "app/name"))
}

func TestRequestError(t *testing.T) {
	assert.EqualError(t, &requestError{gatewayError{Code: 14, Message: "etcdserver: no leader"}}, "etcdserver: no leader")
}

func subscribe(t *testing.T, manager *tinyconf.Manager) <-chan *testConfig {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	changes := make(chan *testConfig, 10)
	assert.NoError(t, tinyconf.Subscribe(ctx, manager, func(prev, next *testConfig) {
		changes <- next
	}))
	return changes
}

func receive(t *testing.T, changes <-chan *testConfig) *testConfig {
	t.Helper()
	select {
	case c := <-changes:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("change is not received")
		return nil
	}
}

func TestDriver_Watch(t *testing.T) {
	etcd := newFakeEtcd(map[string]string{"app/name": "first"})
	t.Cleanup(etcd.Close)
	_, manager, err := parse(t, must(New("app", WithEndpoints(etcd.URL))))
	assert.NoError(t, err)

	// the change made after Parse and before the subscription is reported too
	etcd.put("app/name", str("second"))
	changes := subscribe(t, manager)
	assert.Equal(t, "second", receive(t, changes).Name)

	etcd.put("app/server/port", str("9090"))
	changed := receive(t, changes)
	assert.Equal(t, "second", changed.Name)
	assert.Equal(t, 9090, changed.Server.Port)

	// keys out of the prefix are not watched, the deleted key gets the initial value
	etcd.put("other/name", str("other"))
	etcd.put("app/server/port", nil)
	changed = receive(t, changes)
	assert.Equal(t, 0, changed.Server.Port)
}

func TestDriver_WatchAfterFailedLoad(t *testing.T) {
	etcd := newFakeEtcd(map[string]string{"app/name": "etcd", "app/config": "document: document\n"})
	t.Cleanup(etcd.Close)
	type config struct {
		Name     string `etcd:"name"`
		Document string `yaml:"document"`
	}
	etcdDriver := must(New("app", WithEndpoints(etcd.URL)))
	yamlDriver := must(yaml.New("", yaml.WithSource(Document("app/config", WithEndpoints(etcd.URL)))))
	manager, err := tinyconf.New(tinyconf.WithDriver(etcdDriver), tinyconf.WithDriver(yamlDriver))
	assert.NoError(t, err)
	conf := &config{}
	assert.NoError(t, manager.Register(conf))
	// etcd denies the access at the start
	etcd.set(func(f *fakeEtcd) { f.denied = []string{"app/"} })
	assert.Error(t, manager.Parse(conf))
	etcd.set(func(f *fakeEtcd) { f.denied = nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan *config, 10)
	assert.NoError(t, tinyconf.Subscribe(ctx, manager, func(prev, next *config) {
		changes <- next
	}))
	// etcd is available, the values are loaded without changes of the keys
	select {
	case c := <-changes:
		assert.Equal(t, config{Name: "etcd", Document: "document"}, *c)
	case <-time.After(5 * time.Second):
		t.Fatal("config is not loaded")
	}
}

func TestClient_Watch(t *testing.T) {
	watch := func(t *testing.T, c *client, key string, revision int64) (<-chan struct{}, func() error) {
		ctx, cancel := context.WithCancel(context.Background())
		changes := make(chan struct{}, 10)
		done := make(chan error, 1)
		go func() { done <- c.watch(ctx, key, false, revision, func() { changes <- struct{}{} }) }()
		return changes, func() error {
			cancel()
			return <-done
		}
	}
	wait := func(t *testing.T, changes <-chan struct{}) {
		t.Helper()
		select {
		case <-changes:
		case <-time.After(5 * time.Second):
			t.Fatal("change is not reported")
		}
	}

	t.Run("compacted history", func(t *testing.T) {
		etcd := newFakeEtcd(map[string]string{"key": "value"})
		t.Cleanup(etcd.Close)
		etcd.put("key", str("new"))
		etcd.set(func(f *fakeEtcd) { f.compacted = f.revision })
		// the changes after the revision 1 are compacted, the current state is a change
		changes, stop := watch(t, newClient([]Option{WithEndpoints(etcd.URL)}), "key", 1)
		wait(t, changes)
		etcd.put("key", str("newer"))
		wait(t, changes)
		assert.NoError(t, stop())
	})
	t.Run("access denied stops watching", func(t *testing.T) {
		etcd := newFakeEtcd(nil)
		t.Cleanup(etcd.Close)
		etcd.denied = []string{"key"}
		err := newClient([]Option{WithEndpoints(etcd.URL)}).watch(context.Background(), "key", false, 1, func() {})
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
		assert.ErrorContains(t, err, "PermissionDenied")
	})
	t.Run("expired token is taken again", func(t *testing.T) {
		etcd := newFakeEtcd(map[string]string{"key": "value"})
		t.Cleanup(etcd.Close)
		etcd.users = map[string]string{"user": "password"}
		c := newClient([]Option{WithEndpoints(etcd.URL), WithAuth("user", "password")})
		_, revision, err := c.get("key", false)
		assert.NoError(t, err)
		changes, stop := watch(t, c, "key", revision)
		time.Sleep(20 * time.Millisecond)
		etcd.set(func(f *fakeEtcd) { f.tokens = map[string]bool{} })
		etcd.put("other", str("wakes the stream"))
		// the stream is restarted from the known revision with the new token, no changes are missed
		etcd.put("key", str("new"))
		wait(t, changes)
		assert.NoError(t, stop())
	})
	t.Run("failed authentication stops watching", func(t *testing.T) {
		etcd := newFakeEtcd(nil)
		t.Cleanup(etcd.Close)
		etcd.users = map[string]string{"user": "password"}
		err := newClient([]Option{WithEndpoints(etcd.URL), WithAuth("user", "wrong")}).watch(context.Background(), "key", false, 1, func() {})
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
	})
	t.Run("unavailable etcd is retried", func(t *testing.T) {
		originalRetry, originalMax := retryInterval, maxRetryInterval
		retryInterval, maxRetryInterval = time.Millisecond, 2*time.Millisecond
		defer func() { retryInterval, maxRetryInterval = originalRetry, originalMax }()
		failures := 0
		etcd := newFakeEtcd(map[string]string{"key": "value"})
		t.Cleanup(etcd.Close)
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if failures < 3 {
				failures++
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			etcd.Config.Handler.ServeHTTP(w, r)
		}))
		t.Cleanup(proxy.Close)
		// without the loaded revision the current state is a change, after the failures
		changes, stop := watch(t, newClient([]Option{WithEndpoints(proxy.URL)}), "key", 0)
		wait(t, changes)
		assert.NoError(t, stop())
	})
	t.Run("stream is closed", func(t *testing.T) {
		etcd := newFakeEtcd(map[string]string{"key": "value"})
		watches := 0
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v3/watch" {
				watches++
				if watches == 1 {
					// the stream is closed by the server
					_, _ = w.Write([]byte(`{"result":{"created":true}}`))
					return
				}
				if watches == 2 {
					_, _ = w.Write([]byte(`{"error":{"code":14,"message":"etcdserver: no leader"}}`))
					return
				}
			}
			etcd.Config.Handler.ServeHTTP(w, r)
		}))
		t.Cleanup(proxy.Close)
		t.Cleanup(etcd.Close)
		changes, stop := watch(t, newClient([]Option{WithEndpoints(proxy.URL)}), "key", 1)
		etcd.put("key", str("new"))
		wait(t, changes)
		assert.NoError(t, stop())
	})
	t.Run("watching is denied by the gateway", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusForbidden, 7, "etcdserver: permission denied")
		}))
		t.Cleanup(srv.Close)
		err := newClient([]Option{WithEndpoints(srv.URL)}).watch(context.Background(), "key", false, 1, func() {})
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
	})
	t.Run("canceled while waiting for retry", func(t *testing.T) {
		original := retryInterval
		retryInterval = time.Hour
		defer func() { retryInterval = original }()
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()
		assert.NoError(t, newClient([]Option{WithEndpoints("127.0.0.1:1")}).watch(ctx, "key", false, 1, func() {}))
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
	etcd := newFakeEtcd(map[string]string{"app/config": "name: etcd\nserver:\n  port: 9090\n"})
	t.Cleanup(etcd.Close)

	driver := must(yaml.New("", yaml.WithData("defaults", []byte("name: file\nserver:\n  host: localhost\n  port: 8080\n")),
		yaml.WithSource(Document("/app/config", WithEndpoints(etcd.URL)))))
	manager := newManager(t, driver)
	conf := &config{}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))
	// the document is merged with the data by the yaml tags
	assert.Equal(t, config{Name: "etcd", Server: yamlServer{Host: "localhost", Port: 9090}}, *conf)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan *config)
	assert.NoError(t, tinyconf.Subscribe(ctx, manager, func(prev, next *config) {
		changes <- next
	}))
	etcd.put("app/config", str("name: changed\n"))
	assert.Equal(t, config{Name: "changed", Server: yamlServer{Host: "localhost", Port: 8080}}, *<-changes)

	// absent document is an error, unless the driver has the optional option
	type nameConfig struct {
		Name string `yaml:"name"`
	}
	manager = newManager(t, must(yaml.New("", yaml.WithSource(Document("app/absent", WithEndpoints(etcd.URL))))))
	assert.NoError(t, manager.Register(&nameConfig{}))
	err := manager.Parse(&nameConfig{})
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.ErrorContains(t, err, "failed to load etcd key app/absent")

	manager = newManager(t, must(yaml.New("", yaml.WithOptional(), yaml.WithSource(Document("app/absent", WithEndpoints(etcd.URL))))))
	assert.NoError(t, manager.Register(&nameConfig{}))
	assert.NoError(t, manager.Parse(&nameConfig{}))

	etcd.set(func(f *fakeEtcd) { f.denied = []string{"app/absent"} })
	err = manager.Parse(&nameConfig{})
	assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
}
