package ini

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/internal/tree"
)

type server struct {
	Host string `ini:"host"`
	Port int    `ini:"port"`
}

type testConfig struct {
	Name  string `ini:"name"`
	Debug bool   `ini:"debug"`
	HTTP  server `ini:"http"`
	TLS   *struct {
		Cert string `ini:"cert"`
	} `ini:"tls"`
	Hosts   []string          `ini:"hosts"`
	Labels  map[string]string `ini:"labels"`
	Servers []server          `ini:"servers"`
	Wait    time.Duration     `ini:"wait"`
	Secret  string            `ini:"secret" hidden:"true"`
	Limit   *int              `ini:"limit"`
}

func parse(t *testing.T, driver tinyconf.Driver) (*testConfig, *tinyconf.Manager, error) {
	t.Helper()
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	conf := &testConfig{}
	assert.NoError(t, manager.Register(conf))
	return conf, manager, manager.Parse(conf)
}

func TestDecode(t *testing.T) {
	doc, err := decode([]byte(`; comment
# comment too
name = app ; inline comment
debug: true
hosts = a,b
servers = [{"host": "a", "port": 1}]
http.host = root-level dotted key

[http]
port = 8080
host = "quoted ; not a comment \"value\""

[tls]
cert = '/etc/tls/cert.pem'

[labels]
env = prod # inline comment

[a.b]
c = deep
empty =
url = http://example.com/#anchor
`))
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{
		"name":    "app",
		"debug":   "true",
		"hosts":   "a,b",
		"servers": `[{"host": "a", "port": 1}]`,
		"http":    map[string]any{"host": `quoted ; not a comment "value"`, "port": "8080"},
		"tls":     map[string]any{"cert": "/etc/tls/cert.pem"},
		"labels":  map[string]any{"env": "prod"},
		"a":       map[string]any{"b": map[string]any{"c": "deep", "empty": "", "url": "http://example.com/#anchor"}},
	}, doc)

	for content, message := range map[string]string{
		"[http":                        "line 1: section without ]",
		"[]":                           `line 1: incorrect key ""`,
		"[a..b]":                       `line 1: incorrect key "a..b"`,
		"key":                          "line 1: key without value",
		" = value":                     `line 1: incorrect key ""`,
		`key = "not closed`:            "line 1: incorrect quoted value: invalid syntax",
		"http = 1\n[http]":             "line 2: http has a value, it can not be an object",
		"[x]\nhttp.port = 1\nhttp = 2": "line 3: x.http is an object, it can not have a value",
		"a.b = 1\na.b.c = 2":           "line 2: a.b has a value, it can not be an object",
	} {
		_, err = decode([]byte(content))
		assert.EqualError(t, err, message, content)
	}
}

func TestDriver(t *testing.T) {
	t.Setenv("INI_PORT", "9090")
	file := filepath.Join(t.TempDir(), "config.ini")
	assert.NoError(t, os.WriteFile(file, []byte("name = file\n[http]\nhost = localhost\nport = ${INI_PORT}\n"), 0o600))
	driver, err := New(file,
		WithFiles(filepath.Join(t.TempDir(), "absent.ini")),
		WithFS(fstest.MapFS{"embedded.ini": {Data: []byte("hosts = a,b\nwait = 1m\n")}}, "embedded.ini"),
		WithData("data", []byte("[labels]\nenv = prod\n[tls]\ncert = cert.pem\n")),
		WithSource(tree.DataSource("source", []byte("name = source\n"))),
		WithOptional(),
		WithEnvExpansion(),
		WithWatchInterval(time.Hour),
	)
	assert.NoError(t, err)
	assert.Equal(t, "ini", driver.GetName())
	conf, manager, err := parse(t, driver)
	assert.NoError(t, err)
	assert.Equal(t, "source", conf.Name)
	assert.Equal(t, server{Host: "localhost", Port: 9090}, conf.HTTP)
	assert.Equal(t, "cert.pem", conf.TLS.Cert)
	assert.Equal(t, []string{"a", "b"}, conf.Hosts)
	assert.Equal(t, map[string]string{"env": "prod"}, conf.Labels)
	assert.Equal(t, time.Minute, conf.Wait)
	assert.Contains(t, manager.Sources(conf), tinyconf.FieldSource{Field: "HTTP.Port", Driver: "ini", Source: "http.port"})

	// the file is watched for changes
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan *testConfig, 10)
	assert.NoError(t, tinyconf.Subscribe(ctx, manager, func(prev, next *testConfig) {
		changes <- next
	}))
	time.Sleep(50 * time.Millisecond)
	assert.NoError(t, os.WriteFile(file, []byte("[http]\nport = 9091\n"), 0o600))
	select {
	case c := <-changes:
		assert.Equal(t, 9091, c.HTTP.Port)
	case <-time.After(5 * time.Second):
		t.Fatal("change is not detected")
	}

	// errors of the content
	driver, err = New("", WithData("broken", []byte("[broken")))
	assert.NoError(t, err)
	_, _, err = parse(t, driver)
	assert.ErrorContains(t, err, "failed to load broken: line 1: section without ]")
}

func TestDriver_GenDoc(t *testing.T) {
	limit := 10
	conf := &testConfig{
		Name:  "my app; with comment chars",
		Debug: true,
		HTTP:  server{Host: "localhost", Port: 8080},
		TLS: &struct {
			Cert string `ini:"cert"`
		}{Cert: "cert.pem"},
		Hosts:   []string{"a", "b"},
		Labels:  map[string]string{"env": "prod"},
		Servers: []server{{Host: "a", Port: 1}},
		Wait:    time.Minute,
		Secret:  "secret",
		Limit:   &limit,
	}
	driver, err := New("")
	assert.NoError(t, err)
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	assert.NoError(t, manager.Register(conf))
	doc := manager.GenDoc("ini")
	assert.Equal(t, `debug = true
hosts = a,b
labels = env:prod
limit = 10
name = 'my app; with comment chars'
secret = ""
servers = '[{"host":"a","port":1}]'
wait = 1m0s

[http]
host = localhost
port = 8080

[tls]
cert = cert.pem
`, doc)

	// the documentation is read back to the same config
	driver, err = New("", WithData("doc", []byte(doc)))
	assert.NoError(t, err)
	parsed, _, err := parse(t, driver)
	assert.NoError(t, err)
	conf.Secret = ""
	assert.Equal(t, conf, parsed)

	// values with single quotes and new lines are in double quotes with escapes
	assert.Equal(t, `"it's"`, quote("it's"))
	assert.Equal(t, `"a\nb"`, quote("a\nb"))
	assert.Equal(t, `""`, quote(""))
	assert.Equal(t, `' padded '`, quote(" padded "))

	// nil values are skipped, sections without values have no headers
	type nested struct {
		Limit *int `ini:"limit"`
		Inner struct {
			Value string `ini:"value"`
		} `ini:"inner"`
	}
	driver, err = New("")
	assert.NoError(t, err)
	manager, err = tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	assert.NoError(t, manager.Register(&nested{}))
	assert.Equal(t, "[inner]\nvalue = \"\"\n", manager.GenDoc("ini"))
}
