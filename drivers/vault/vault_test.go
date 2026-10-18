package vault

import (
	"context"
	"encoding/json"
	"io/fs"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf/v2"
)

// fakeVault is a minimal HTTP API of the KV secrets engine of Vault.
type fakeVault struct {
	*httptest.Server

	mu       sync.Mutex
	kv1      bool
	secrets  map[string]map[string]any
	versions map[string]int
	// denied are the paths that the token can not read
	denied   map[string]bool
	token    string
	requests []*http.Request
}

func newFakeVault(secrets map[string]map[string]any) *fakeVault {
	f := &fakeVault{secrets: map[string]map[string]any{}, versions: map[string]int{}, denied: map[string]bool{}}
	for path, data := range secrets {
		f.put(path, data)
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

// put writes the new version of the secret, nil deletes it.
func (f *fakeVault) put(path string, data map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if data == nil {
		delete(f.secrets, path)
		return
	}
	f.secrets[path] = data
	f.versions[path]++
}

func (f *fakeVault) lastRequest() *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func (f *fakeVault) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r)
	writeErrors := func(status int, errs ...string) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"errors": errs})
	}
	if f.token != "" && r.Header.Get("X-Vault-Token") != f.token {
		writeErrors(http.StatusForbidden, "permission denied")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/secret/")
	if !f.kv1 {
		path = strings.TrimPrefix(path, "data/")
	}
	if f.denied[path] {
		writeErrors(http.StatusForbidden, "1 error occurred:", "permission denied")
		return
	}
	data, ok := f.secrets[path]
	if !ok {
		writeErrors(http.StatusNotFound)
		return
	}
	resp := map[string]any{"request_id": time.Now().String(), "data": data}
	if !f.kv1 {
		resp["data"] = map[string]any{"data": data, "metadata": map[string]any{"version": f.versions[path]}}
	}
	_ = json.NewEncoder(w).Encode(resp)
}

type server struct {
	Host string `vault:"host"`
	Port int    `vault:"port"`
}

type testConfig struct {
	Name     string            `vault:"name" doc:"application name"`
	Server   server            `vault:"server"`
	Hosts    []string          `vault:"hosts"`
	Labels   map[string]string `vault:"labels"`
	Password string            `vault:"password" hidden:"true"`
	NoTag    string
}

func parse(t *testing.T, driver tinyconf.Driver) (*testConfig, *tinyconf.Manager, error) {
	t.Helper()
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	conf := &testConfig{}
	assert.NoError(t, manager.Register(conf))
	return conf, manager, manager.Parse(context.Background(), conf)
}

func TestDriver_Parse(t *testing.T) {
	vault := newFakeVault(map[string]map[string]any{
		"app/config": {
			"name":     "app",
			"server":   map[string]any{"host": "localhost", "port": 8080},
			"hosts":    []any{"a", "b"},
			"labels":   map[string]any{"env": "prod"},
			"password": "first",
		},
		"app/secrets": {"password": "secret", "server": map[string]any{"port": "9090"}},
	})
	defer vault.Close()

	driver, err := New("/app/config/", WithAddress(vault.URL+"/"), WithToken("token"), WithNamespace("team"),
		WithPaths("app/secrets", ""))
	assert.NoError(t, err)
	assert.Equal(t, "vault", driver.GetName())
	conf, manager, err := parse(t, driver)
	assert.NoError(t, err)
	// secrets are merged in the order of the paths
	assert.Equal(t, testConfig{
		Name:     "app",
		Server:   server{Host: "localhost", Port: 9090},
		Hosts:    []string{"a", "b"},
		Labels:   map[string]string{"env": "prod"},
		Password: "secret",
	}, *conf)
	assert.Contains(t, manager.Sources(conf), tinyconf.FieldSource{Field: "Server.Port", Driver: "vault", Source: "server.port"})

	req := vault.lastRequest()
	assert.Equal(t, "/v1/secret/data/app/secrets", req.URL.Path)
	assert.Equal(t, "token", req.Header.Get("X-Vault-Token"))
	assert.Equal(t, "team", req.Header.Get("X-Vault-Namespace"))
}

func TestDriver_KVv1(t *testing.T) {
	vault := newFakeVault(map[string]map[string]any{"app": {"name": "v1"}})
	defer vault.Close()
	vault.kv1 = true
	driver, err := New("app", WithAddress(vault.URL), WithKVv1())
	assert.NoError(t, err)
	conf, _, err := parse(t, driver)
	assert.NoError(t, err)
	assert.Equal(t, "v1", conf.Name)
	assert.Equal(t, "/v1/secret/app", vault.lastRequest().URL.Path)

	// the version is the hash of the data, the id of the request is not a change
	c := newClient(options{address: vault.URL, kv1: true})
	_, first, err := c.read(context.Background(), "app")
	assert.NoError(t, err)
	_, second, err := c.read(context.Background(), "app")
	assert.NoError(t, err)
	assert.Equal(t, first, second)
	vault.put("app", map[string]any{"name": "changed"})
	_, third, err := c.read(context.Background(), "app")
	assert.NoError(t, err)
	assert.NotEqual(t, first, third)
}

func TestDriver_Errors(t *testing.T) {
	vault := newFakeVault(map[string]map[string]any{"app": {"name": "app"}, "denied": {"name": "denied"}})
	defer vault.Close()
	vault.denied["denied"] = true

	t.Run("absent secret", func(t *testing.T) {
		driver, err := New("absent", WithAddress(vault.URL))
		assert.NoError(t, err)
		_, _, err = parse(t, driver)
		assert.ErrorIs(t, err, fs.ErrNotExist)
		assert.ErrorContains(t, err, "failed to load vault secret secret/absent")

		driver, err = New("absent", WithAddress(vault.URL), WithOptional())
		assert.NoError(t, err)
		_, _, err = parse(t, driver)
		assert.NoError(t, err)
	})
	t.Run("access denied", func(t *testing.T) {
		driver, err := New("denied", WithAddress(vault.URL))
		assert.NoError(t, err)
		_, _, err = parse(t, driver)
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
		assert.ErrorContains(t, err, "failed to read secret denied: 1 error occurred:, permission denied")
	})
	t.Run("other statuses", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "sealed", http.StatusServiceUnavailable)
		}))
		defer srv.Close()
		driver, err := New("app", WithAddress(srv.URL))
		assert.NoError(t, err)
		_, _, err = parse(t, driver)
		assert.ErrorContains(t, err, "failed to read secret app: 503 Service Unavailable: sealed")
	})
	t.Run("invalid response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}))
		defer srv.Close()
		for _, opts := range [][]Option{{WithAddress(srv.URL)}, {WithAddress(srv.URL), WithKVv1()}} {
			driver, err := New("app", opts...)
			assert.NoError(t, err)
			_, _, err = parse(t, driver)
			assert.ErrorContains(t, err, "failed to decode secret app")
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		driver, err := New("app", WithAddress("http://127.0.0.1:1"))
		assert.NoError(t, err)
		_, _, err = parse(t, driver)
		assert.Error(t, err)
	})
	t.Run("invalid address", func(t *testing.T) {
		driver, err := New("app", WithAddress("http://vault\n"))
		assert.NoError(t, err)
		_, _, err = parse(t, driver)
		assert.Error(t, err)
	})
	t.Run("long response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("{"))
		}))
		defer srv.Close()
		driver, err := New("app", WithAddress(srv.URL))
		assert.NoError(t, err)
		_, _, err = parse(t, driver)
		assert.Error(t, err)
	})
}

func TestDriver_TokenFile(t *testing.T) {
	vault := newFakeVault(map[string]map[string]any{"app": {"name": "app"}})
	defer vault.Close()
	vault.token = "first"
	tokenFile := filepath.Join(t.TempDir(), "token")
	assert.NoError(t, os.WriteFile(tokenFile, []byte("first\n"), 0o600))

	driver, err := New("app", WithAddress(vault.URL), WithTokenFile(tokenFile))
	assert.NoError(t, err)
	_, manager, err := parse(t, driver)
	assert.NoError(t, err)

	// the renewed token is read from the file before every request
	vault.token = "second"
	assert.NoError(t, os.WriteFile(tokenFile, []byte("second"), 0o600))
	assert.NoError(t, manager.Parse(context.Background(), &testConfig{}))

	assert.NoError(t, os.Remove(tokenFile))
	assert.ErrorContains(t, manager.Parse(context.Background(), &testConfig{}), "failed to read the token")
}

func TestNewClient(t *testing.T) {
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "")
	t.Setenv("VAULT_NAMESPACE", "")
	c := newClient(options{})
	assert.Equal(t, DefaultAddress, c.address)
	assert.Equal(t, DefaultMount, c.mount)
	assert.Equal(t, http.DefaultClient, c.httpClient)

	t.Setenv("VAULT_ADDR", "http://vault:8200")
	t.Setenv("VAULT_TOKEN", "env-token")
	t.Setenv("VAULT_NAMESPACE", "env-namespace")
	c = newClient(options{})
	assert.Equal(t, "http://vault:8200", c.address)
	assert.Equal(t, "env-token", c.token)
	assert.Equal(t, "env-namespace", c.namespace)

	// the token from the file has priority over the variable
	c = newClient(options{tokenFile: "token"})
	assert.Equal(t, "", c.token)

	httpClient := &http.Client{}
	o := options{}
	for _, opt := range []Option{WithMount("/kv/"), WithHTTPClient(httpClient)} {
		opt(&o)
	}
	c = newClient(o)
	assert.Equal(t, "kv", c.mount)
	assert.Equal(t, httpClient, c.httpClient)
	assert.Equal(t, "http://vault:8200/v1/kv/data/app/my%20config", c.secretURL("app/my config"))
}

func TestDriver_GenDoc(t *testing.T) {
	driver, err := New("app/config", WithMount("kv"), WithPaths("app/secrets"))
	assert.NoError(t, err)
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	assert.NoError(t, manager.Register(&testConfig{
		Name:     "it's app",
		Server:   server{Host: "localhost", Port: 8080},
		Password: "secret",
	}))
	assert.Equal(t, `vault kv put -mount=kv app/config - <<'EOF'
{
  "hosts": [],
  "labels": {},
  "name": "it's app",
  "password": "",
  "server": {
    "host": "localhost",
    "port": 8080
  }
}
EOF
`, manager.GenDoc("vault"))
	assert.Equal(t, `'it'\''s'`, shellQuote("it's"))

	// NaN can not be encoded to JSON
	type ratioConfig struct {
		Ratio float64 `vault:"ratio"`
	}
	manager, err = tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	assert.NoError(t, manager.Register(&ratioConfig{Ratio: math.NaN()}))
	assert.Equal(t, "", manager.GenDoc("vault"))

	// without the paths and without the fields
	driver, err = New("")
	assert.NoError(t, err)
	manager, err = tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	assert.NoError(t, manager.Register(&testConfig{}))
	assert.Equal(t, "", manager.GenDoc("vault"))
}

func TestDriver_Watch(t *testing.T) {
	vault := newFakeVault(map[string]map[string]any{"app": {"name": "first"}})
	defer vault.Close()
	driver, err := New("app", WithAddress(vault.URL), WithWatchInterval(5*time.Millisecond))
	assert.NoError(t, err)
	_, manager, err := parse(t, driver)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan *testConfig, 10)
	assert.NoError(t, tinyconf.Subscribe(ctx, manager, func(prev, next *testConfig) {
		changes <- next
	}))
	receive := func() *testConfig {
		select {
		case c := <-changes:
			return c
		case <-time.After(5 * time.Second):
			t.Fatal("change is not detected")
			return nil
		}
	}

	vault.put("app", map[string]any{"name": "second"})
	assert.Equal(t, "second", receive().Name)

	// the secret is not available, then it is available again with the new value
	vault.mu.Lock()
	vault.denied["app"] = true
	vault.mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	vault.mu.Lock()
	vault.denied["app"] = false
	vault.mu.Unlock()
	vault.put("app", map[string]any{"name": "third"})
	assert.Equal(t, "third", receive().Name)

	// the deleted secret is a change too, the driver without the secret returns the error
	vault.put("app", nil)
	select {
	case c := <-changes:
		t.Fatalf("the config with the error is applied: %+v", *c)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestDriver_ParseCancel(t *testing.T) {
	vault := newFakeVault(map[string]map[string]any{"app": {"name": "app"}})
	defer vault.Close()
	driver, err := New("app", WithAddress(vault.URL))
	assert.NoError(t, err)
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	conf := &testConfig{}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(context.Background(), conf))
	assert.Equal(t, "app", conf.Name)

	// the requests are stopped by the context of Parse
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, manager.Parse(ctx, &testConfig{}), context.Canceled)
}
