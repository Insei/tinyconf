package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcconsul "github.com/testcontainers/testcontainers-go/modules/consul"

	"github.com/insei/tinyconf/v2"
	"github.com/insei/tinyconf/v2/drivers/consul"
	"github.com/insei/tinyconf/v2/drivers/yaml"
)

const consulImage = "hashicorp/consul:1.20"

// consulServer is a Consul agent in Docker.
type consulServer struct {
	container *tcconsul.ConsulContainer
	address   string
	token     string
}

func startConsul(t *testing.T, opts ...testcontainers.ContainerCustomizer) *consulServer {
	t.Helper()
	ctx := context.Background()
	c, err := tcconsul.Run(ctx, consulImage, opts...)
	testcontainers.CleanupContainer(t, c)
	require.NoError(t, err)
	address, err := c.ApiEndpoint(ctx)
	require.NoError(t, err)
	return &consulServer{container: c, address: address}
}

func (s *consulServer) options() []consul.Option {
	return []consul.Option{consul.WithAddress(s.address), consul.WithToken(s.token), consul.WithWaitTime(10 * time.Second)}
}

// request calls the HTTP API of Consul and returns the body of the response.
func (s *consulServer) request(t *testing.T, method, path, body string) []byte {
	t.Helper()
	req, err := http.NewRequest(method, "http://"+s.address+"/v1/"+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-Consul-Token", s.token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s %s: %s", method, path, data)
	return data
}

func (s *consulServer) put(t *testing.T, key, value string) {
	s.request(t, http.MethodPut, "kv/"+key, value)
}

func (s *consulServer) delete(t *testing.T, key string) {
	s.request(t, http.MethodDelete, "kv/"+key, "")
}

// createToken creates the ACL token that can read the keys with the prefix.
func (s *consulServer) createToken(t *testing.T, prefix string) string {
	t.Helper()
	policy := fmt.Sprintf(`{"Name": %q, "Rules": "key_prefix \"%s\" { policy = \"read\" }"}`, "read-"+strings.ReplaceAll(prefix, "/", "-"), prefix)
	s.request(t, http.MethodPut, "acl/policy", policy)
	var token struct{ SecretID string }
	data := s.request(t, http.MethodPut, "acl/token", fmt.Sprintf(`{"Policies": [{"Name": %q}]}`, "read-"+strings.ReplaceAll(prefix, "/", "-")))
	require.NoError(t, json.Unmarshal(data, &token))
	return token.SecretID
}

func newManager(t *testing.T, drivers ...tinyconf.Driver) *tinyconf.Manager {
	t.Helper()
	opts := make([]tinyconf.Option, 0, len(drivers))
	for _, driver := range drivers {
		opts = append(opts, tinyconf.WithDriver(driver))
	}
	manager, err := tinyconf.New(opts...)
	require.NoError(t, err)
	return manager
}

// subscribe subscribes to the changes of the config and returns the channel with the new versions.
// Reloads with errors are logged by the manager and are not reported.
func subscribe[T any](t *testing.T, manager *tinyconf.Manager) <-chan *T {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	changes := make(chan *T, 10)
	require.NoError(t, tinyconf.Subscribe(ctx, manager, func(prev, next *T) {
		changes <- next
	}))
	return changes
}

func next[T any](t *testing.T, changes <-chan *T) *T {
	t.Helper()
	select {
	case c := <-changes:
		return c
	case <-time.After(15 * time.Second):
		t.Fatal("config is not reloaded")
		return nil
	}
}

func noReload[T any](t *testing.T, changes <-chan *T) {
	t.Helper()
	select {
	case c := <-changes:
		t.Fatalf("unexpected reload: %+v", *c)
	case <-time.After(2 * time.Second):
	}
}

type server struct {
	Host    string        `consul:"host" yaml:"host"`
	Port    int           `consul:"port" yaml:"port"`
	Timeout time.Duration `consul:"timeout" yaml:"timeout"`
}

type keysConfig struct {
	Name     string            `consul:"name" doc:"application name"`
	Server   server            `consul:"server"`
	Hosts    []string          `consul:"hosts"`
	Labels   map[string]string `consul:"labels"`
	Password string            `consul:"password" hidden:"true"`
}

func TestConsul(t *testing.T) {
	srv := startConsul(t)

	t.Run("key per field", func(t *testing.T) {
		srv.put(t, "keys/name", "app")
		srv.put(t, "keys/server/host", "example.com")
		srv.put(t, "keys/server/port", "9090")
		srv.put(t, "keys/server/timeout", "1m30s")
		srv.put(t, "keys/hosts", `a,"b,c"`)
		srv.put(t, "keys/labels", "env:prod,team:core")
		srv.put(t, "keys/password", "secret")
		// keys with the same beginning but another prefix are not read
		srv.put(t, "keys-other/name", "other")

		driver, err := consul.New("keys", srv.options()...)
		require.NoError(t, err)
		manager := newManager(t, driver)
		conf := &keysConfig{}
		require.NoError(t, manager.Register(conf))
		require.NoError(t, manager.Parse(context.Background(), conf))
		assert.Equal(t, keysConfig{
			Name:     "app",
			Server:   server{Host: "example.com", Port: 9090, Timeout: 90 * time.Second},
			Hosts:    []string{"a", "b,c"},
			Labels:   map[string]string{"env": "prod", "team": "core"},
			Password: "secret",
		}, *conf)
		assert.Equal(t, tinyconf.FieldSource{Field: "Server.Port", Driver: "consul", Source: "keys/server/port"}, manager.Sources(conf)[2])
	})

	t.Run("gendoc commands put the values to consul", func(t *testing.T) {
		driver, err := consul.New("gendoc", srv.options()...)
		require.NoError(t, err)
		manager := newManager(t, driver)
		conf := &keysConfig{
			Name:   "it's my app",
			Server: server{Host: "example.com", Port: 9090, Timeout: time.Minute},
			Hosts:  []string{"a b", "c"},
			Labels: map[string]string{"env": "prod"},
		}
		require.NoError(t, manager.Register(conf))
		commands := manager.GenDoc("consul")

		// the commands are run by the Consul CLI in the container
		code, output, err := srv.container.Exec(context.Background(), []string{"sh", "-c", commands})
		require.NoError(t, err)
		out, _ := io.ReadAll(output)
		require.Equal(t, 0, code, out)

		parsed := &keysConfig{}
		manager = newManager(t, driver)
		require.NoError(t, manager.Register(parsed))
		require.NoError(t, manager.Parse(context.Background(), parsed))
		assert.Equal(t, *conf, *parsed)
	})

	t.Run("document merged with file", func(t *testing.T) {
		type config struct {
			Name   string `yaml:"name"`
			Server server `yaml:"server"`
		}
		srv.put(t, "document/config", "name: consul\nserver:\n  port: 9090\n")
		file := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(file, []byte("name: file\nserver:\n  host: localhost\n  port: 8080\n"), 0o600))

		driver, err := yaml.New(file, yaml.WithSource(consul.Document("document/config", srv.options()...)))
		require.NoError(t, err)
		manager := newManager(t, driver)
		conf := &config{}
		require.NoError(t, manager.Register(conf))
		require.NoError(t, manager.Parse(context.Background(), conf))
		assert.Equal(t, config{Name: "consul", Server: server{Host: "localhost", Port: 9090}}, *conf)

		// absent document is an error, unless the driver has the optional option
		driver, err = yaml.New("", yaml.WithSource(consul.Document("document/absent", srv.options()...)))
		require.NoError(t, err)
		manager = newManager(t, driver)
		require.NoError(t, manager.Register(conf))
		assert.ErrorContains(t, manager.Parse(context.Background(), conf), "failed to load consul key document/absent")
	})

	t.Run("watch", func(t *testing.T) {
		type config struct {
			Name  string `yaml:"name"`
			Level string `consul:"level"`
			Port  int    `consul:"port"`
		}
		srv.put(t, "watch/document", "name: first\n")
		srv.put(t, "watch/keys/level", "info")
		yamlDriver, err := yaml.New("", yaml.WithSource(consul.Document("watch/document", srv.options()...)))
		require.NoError(t, err)
		consulDriver, err := consul.New("watch/keys", srv.options()...)
		require.NoError(t, err)
		manager := newManager(t, yamlDriver, consulDriver)
		conf := &config{}
		require.NoError(t, manager.Register(conf))
		require.NoError(t, manager.Parse(context.Background(), conf))
		changes := subscribe[config](t, manager)

		srv.put(t, "watch/keys/level", "debug")
		assert.Equal(t, config{Name: "first", Level: "debug"}, *next(t, changes))

		srv.put(t, "watch/keys/port", "8080")
		assert.Equal(t, config{Name: "first", Level: "debug", Port: 8080}, *next(t, changes))

		srv.put(t, "watch/document", "name: second\n")
		assert.Equal(t, config{Name: "second", Level: "debug", Port: 8080}, *next(t, changes))

		// the deleted key gets the initial value
		srv.delete(t, "watch/keys/port")
		assert.Equal(t, config{Name: "second", Level: "debug"}, *next(t, changes))

		// keys out of the prefix and the document do not cause reloads
		srv.put(t, "watch/other", "value")
		srv.put(t, "other/level", "error")
		noReload(t, changes)
	})
}

func TestConsul_ACL(t *testing.T) {
	srv := startConsul(t, tcconsul.WithConfigString(`{
		"acl": {"enabled": true, "default_policy": "deny", "tokens": {"initial_management": "root"}}
	}`))
	srv.token = "root"
	srv.put(t, "acl/name", "secured")
	srv.put(t, "acl/secret/password", "secret")

	type config struct {
		Name   string `consul:"name"`
		Secret struct {
			Password string `consul:"password"`
		} `consul:"secret"`
	}
	parse := func(opts ...consul.Option) (*config, error) {
		driver, err := consul.New("acl", append([]consul.Option{consul.WithAddress(srv.address)}, opts...)...)
		require.NoError(t, err)
		manager := newManager(t, driver)
		conf := &config{}
		require.NoError(t, manager.Register(conf))
		return conf, manager.Parse(context.Background(), conf)
	}

	// Consul answers as if there are no keys, the driver checks the permission
	_, err := parse()
	assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
	assert.ErrorContains(t, err, "403 Forbidden: Permission denied")

	// some keys are hidden, the config would be incomplete
	_, err = parse(consul.WithToken(srv.createToken(t, "acl/name")))
	assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
	assert.ErrorContains(t, err, "some keys are hidden by ACL")

	conf, err := parse(consul.WithToken(srv.createToken(t, "acl/")))
	require.NoError(t, err)
	assert.Equal(t, "secured", conf.Name)
	assert.Equal(t, "secret", conf.Secret.Password)

	// the document is read by its own key
	driver, err := yaml.New("", yaml.WithSource(consul.Document("acl/document", consul.WithAddress(srv.address))))
	require.NoError(t, err)
	manager := newManager(t, driver)
	require.NoError(t, manager.Register(conf))
	err = manager.Parse(context.Background(), conf)
	assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
	assert.ErrorContains(t, err, "403 Forbidden")
}

// freePort returns a free TCP port of the host.
func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	return fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
}

func TestConsul_RestartWhileWatching(t *testing.T) {
	// the host port is fixed, so the address is the same after the restart
	port := freePort(t)
	srv := startConsul(t, testcontainers.WithHostConfigModifier(func(hostConfig *container.HostConfig) {
		hostConfig.PortBindings = network.PortMap{
			network.MustParsePort("8500/tcp"): {{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: port}},
		}
	}))
	srv.put(t, "restart/name", "before")

	type config struct {
		Name string `consul:"name"`
	}
	driver, err := consul.New("restart", srv.options()...)
	require.NoError(t, err)
	manager := newManager(t, driver)
	conf := &config{}
	require.NoError(t, manager.Register(conf))
	require.NoError(t, manager.Parse(context.Background(), conf))
	changes := subscribe[config](t, manager)

	// the dev agent keeps the data in memory, after the restart the keys and the indexes start from scratch,
	// so the new indexes can be the same as the known ones: the driver reloads the config after the failure anyway
	ctx := context.Background()
	timeout := 10 * time.Second
	require.NoError(t, srv.container.Stop(ctx, &timeout))
	require.NoError(t, srv.container.Start(ctx))
	waitLeader := func() {
		require.Eventually(t, func() bool {
			resp, err := http.Get("http://" + srv.address + "/v1/status/leader")
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			return resp.StatusCode == http.StatusOK && string(body) != `""`
		}, 30*time.Second, 200*time.Millisecond)
	}
	waitLeader()

	srv.put(t, "restart/name", "after")
	for {
		if c := next(t, changes); c.Name == "after" {
			break
		}
	}

	// Consul is not available at the start of the application, the config is loaded when it is available
	require.NoError(t, srv.container.Stop(ctx, &timeout))
	manager = newManager(t, driver)
	conf = &config{}
	require.NoError(t, manager.Register(conf))
	require.Error(t, manager.Parse(context.Background(), conf))
	changes = subscribe[config](t, manager)
	require.NoError(t, srv.container.Start(ctx))
	waitLeader()
	srv.put(t, "restart/name", "started")
	for {
		if c := next(t, changes); c.Name == "started" {
			break
		}
	}
}
