package properties

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
	Host string `properties:"host"`
	Port int    `properties:"port"`
}

type testConfig struct {
	Name    string            `properties:"name"`
	HTTP    server            `properties:"http"`
	Hosts   []string          `properties:"hosts"`
	Labels  map[string]string `properties:"labels"`
	Servers []server          `properties:"servers"`
	Wait    time.Duration     `properties:"wait"`
	Secret  string            `properties:"secret" hidden:"true"`
	Limit   *int              `properties:"limit"`
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
	doc, err := decode([]byte("# comment\r\n" +
		"! comment too\n" +
		"   name = my app  \n" +
		"http.host:localhost\n" +
		"http.port 8080\n" +
		"hosts = a,\\\n" +
		"        b\n" +
		"path = C:\\\\Program Files\\\\app\n" +
		"key\\ with\\=separators = value\n" +
		"unicode = \\u043f\\u0440\\u0438\\u0432\\u0435\\u0442\n" +
		"escapes = tab\\tnew\\nline\\rcr\\fff \\q\n" +
		"empty\n" +
		"        \n" +
		"tail = end\\"))
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{
		"name":                "my app  ",
		"http":                map[string]any{"host": "localhost", "port": "8080"},
		"hosts":               "a,b",
		"path":                `C:\Program Files\app`,
		"key with=separators": "value",
		"unicode":             "привет",
		"escapes":             "tab\tnew\nline\rcr\fff q",
		"empty":               "",
		"tail":                "end",
	}, doc)

	for content, message := range map[string]string{
		"a..b = 1":                `line 1: incorrect key "a..b"`,
		"\n\nkey = \\u12":         `line 3: incorrect escape sequence "\\u12"`,
		"key = \\u12zz":           `line 1: incorrect escape sequence "\\u12zz"`,
		"bad\\u1 = 1":             `line 1: incorrect escape sequence "\\u1"`,
		"http = 1\nhttp.port = 2": "line 2: http has a value, it can not be an object",
	} {
		_, err = decode([]byte(content))
		assert.EqualError(t, err, message, content)
	}
}

func TestDriver(t *testing.T) {
	t.Setenv("PROPERTIES_PORT", "9090")
	file := filepath.Join(t.TempDir(), "application.properties")
	assert.NoError(t, os.WriteFile(file, []byte("name=file\nhttp.host=localhost\nhttp.port=${PROPERTIES_PORT}\n"), 0o600))
	driver, err := New(file,
		WithFiles(filepath.Join(t.TempDir(), "absent.properties")),
		WithFS(fstest.MapFS{"embedded.properties": {Data: []byte("hosts=a,b\nwait=1m\n")}}, "embedded.properties"),
		WithData("data", []byte("labels.env=prod\n")),
		WithSource(tree.DataSource("source", []byte("name=source\n"))),
		WithOptional(),
		WithEnvExpansion(),
		WithWatchInterval(time.Hour),
	)
	assert.NoError(t, err)
	assert.Equal(t, "properties", driver.GetName())
	conf, manager, err := parse(t, driver)
	assert.NoError(t, err)
	assert.Equal(t, "source", conf.Name)
	assert.Equal(t, server{Host: "localhost", Port: 9090}, conf.HTTP)
	assert.Equal(t, []string{"a", "b"}, conf.Hosts)
	assert.Equal(t, map[string]string{"env": "prod"}, conf.Labels)
	assert.Equal(t, time.Minute, conf.Wait)
	assert.Contains(t, manager.Sources(conf), tinyconf.FieldSource{Field: "HTTP.Port", Driver: "properties", Source: "http.port"})

	// the file is watched for changes
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan *testConfig, 10)
	assert.NoError(t, tinyconf.Subscribe(ctx, manager, func(prev, next *testConfig) {
		changes <- next
	}))
	time.Sleep(50 * time.Millisecond)
	assert.NoError(t, os.WriteFile(file, []byte("http.port=9091\n"), 0o600))
	select {
	case c := <-changes:
		assert.Equal(t, 9091, c.HTTP.Port)
	case <-time.After(5 * time.Second):
		t.Fatal("change is not detected")
	}

	driver, err = New("", WithData("broken", []byte("a..b=1")))
	assert.NoError(t, err)
	_, _, err = parse(t, driver)
	assert.ErrorContains(t, err, `failed to load broken: line 1: incorrect key "a..b"`)
}

func TestDriver_GenDoc(t *testing.T) {
	limit := 10
	conf := &testConfig{
		Name:    " my app\twith\\special # chars\n",
		HTTP:    server{Host: "localhost", Port: 8080},
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
	doc := manager.GenDoc("properties")
	assert.Equal(t, `hosts=a,b
http.host=localhost
http.port=8080
labels=env:prod
limit=10
name=\ my app\twith\\special # chars\n
secret=
servers=[{"host":"a","port":1}]
wait=1m0s
`, doc)

	// the documentation is read back to the same config
	driver, err = New("", WithData("doc", []byte(doc)))
	assert.NoError(t, err)
	parsed, _, err := parse(t, driver)
	assert.NoError(t, err)
	conf.Secret = ""
	assert.Equal(t, conf, parsed)

	// separators and comments in the keys are escaped
	assert.Equal(t, `key\ with\=\:\#\!=#value`, escape("key with=:#!", true)+"="+escape("#value", false)[1:])
	assert.Equal(t, `\#value\r\f`, escape("#value\r\f", false))

	// nil values are skipped
	type nilConfig struct {
		Limit *int `properties:"limit"`
	}
	driver, err = New("")
	assert.NoError(t, err)
	manager, err = tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	assert.NoError(t, manager.Register(&nilConfig{}))
	assert.Equal(t, "", manager.GenDoc("properties"))
}

// the case found by FuzzDecode: the bytes that are not UTF-8 are written as is
func TestEscape_NotUTF8(t *testing.T) {
	// the byte is kept, the backslash after it is escaped
	assert.Equal(t, "\xfe\\\\", escape("\xfe\\", false))
	doc, err := decode([]byte(escape("\xfe", true) + "=" + escape("\xff", false)))
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"\xfe": "\xff"}, doc)
}
