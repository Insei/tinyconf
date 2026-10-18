package format

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/internal/tree"
)

// decodeLines decodes the format of the tests: key.path=value lines, the parts of the key are nested objects.
func decodeLines(data []byte) (map[string]any, error) {
	doc := make(map[string]any)
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, errors.New("line without =")
		}
		keys := strings.Split(key, ".")
		node := doc
		for _, k := range keys[:len(keys)-1] {
			nested, ok := node[k].(map[string]any)
			if !ok {
				nested = make(map[string]any)
				node[k] = nested
			}
			node = nested
		}
		node[keys[len(keys)-1]] = value
	}
	return doc, nil
}

func encodeJSON(doc map[string]any) ([]byte, error) {
	return json.Marshal(doc)
}

type testConfig struct {
	Name string `lines:"name" doc:"application name"`
	HTTP struct {
		Host  string        `lines:"host"`
		Port  int           `lines:"port"`
		Hosts []string      `lines:"hosts"`
		Wait  time.Duration `lines:"wait"`
	} `lines:"http"`
	Password string `lines:"password" hidden:"true"`
}

func parse(t *testing.T, driver tinyconf.Driver) (*testConfig, *tinyconf.Manager, error) {
	t.Helper()
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	conf := &testConfig{}
	assert.NoError(t, manager.Register(conf))
	return conf, manager, manager.Parse(context.Background(), conf)
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	assert.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestNew(t *testing.T) {
	t.Setenv("FORMAT_PORT", "9090")
	file := writeFile(t, "config", "name=file\nhttp.host=localhost\nhttp.port=${FORMAT_PORT}\n")
	driver, err := New("lines", decodeLines, file,
		WithFiles(writeFile(t, "local", "http.hosts=a,b\n"), filepath.Join(t.TempDir(), "absent")),
		WithFS(fstest.MapFS{"embedded": {Data: []byte("http.wait=1m\n")}}, "embedded"),
		WithData("data", []byte("password=secret\n")),
		WithSource(tree.DataSource("source", []byte("name=source\n"))),
		WithOptional(),
		WithEnvExpansion(),
	)
	assert.NoError(t, err)
	assert.Equal(t, "lines", driver.GetName())
	conf, manager, err := parse(t, driver)
	assert.NoError(t, err)
	// sources are merged in the order of addition, nested objects by keys
	assert.Equal(t, "source", conf.Name)
	assert.Equal(t, "localhost", conf.HTTP.Host)
	assert.Equal(t, 9090, conf.HTTP.Port)
	assert.Equal(t, []string{"a", "b"}, conf.HTTP.Hosts)
	assert.Equal(t, time.Minute, conf.HTTP.Wait)
	assert.Equal(t, "secret", conf.Password)
	assert.Contains(t, manager.Sources(conf), tinyconf.FieldSource{Field: "HTTP.Port", Driver: "lines", Source: "http.port"})
}

func TestNew_Errors(t *testing.T) {
	_, err := New("", decodeLines, "")
	assert.EqualError(t, err, "name of the format driver is empty")
	_, err = New("lines", nil, "")
	assert.EqualError(t, err, "decoder of the format driver is nil")

	// absent file is an error without WithOptional
	driver, err := New("lines", decodeLines, filepath.Join(t.TempDir(), "absent"))
	assert.NoError(t, err)
	_, _, err = parse(t, driver)
	assert.Error(t, err)

	driver, err = New("lines", decodeLines, "", WithData("broken", []byte("no equal sign")))
	assert.NoError(t, err)
	_, _, err = parse(t, driver)
	assert.ErrorContains(t, err, "failed to load broken: line without =")
}

func TestGenDoc(t *testing.T) {
	conf := &testConfig{Name: "app", Password: "secret"}
	conf.HTTP.Port = 8080
	genDoc := func(opts ...Option) string {
		driver, err := New("lines", decodeLines, "", opts...)
		assert.NoError(t, err)
		manager, err := tinyconf.New(tinyconf.WithDriver(driver))
		assert.NoError(t, err)
		assert.NoError(t, manager.Register(conf))
		return manager.GenDoc("lines")
	}
	assert.Equal(t, `{"http":{"host":"","hosts":[],"port":8080,"wait":"0s"},"name":"app","password":""}`, genDoc(WithEncoder(encodeJSON)))
	// without the encoder or with its error
	assert.Equal(t, "", genDoc())
	assert.Equal(t, "", genDoc(WithEncoder(func(map[string]any) ([]byte, error) { return nil, errors.New("broken") })))

	// without fields with the tags
	driver, err := New("other", decodeLines, "", WithEncoder(encodeJSON))
	assert.NoError(t, err)
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	assert.NoError(t, manager.Register(conf))
	assert.Equal(t, "", manager.GenDoc("other"))
}

func TestWatch(t *testing.T) {
	file := writeFile(t, "config", "name=first\n")
	driver, err := New("lines", decodeLines, file, WithWatchInterval(time.Hour))
	assert.NoError(t, err)
	_, manager, err := parse(t, driver)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan *testConfig, 10)
	assert.NoError(t, tinyconf.Subscribe(ctx, manager, func(prev, next *testConfig) {
		changes <- next
	}))
	time.Sleep(50 * time.Millisecond)
	assert.NoError(t, os.WriteFile(file, []byte("name=second\n"), 0o600))
	select {
	case c := <-changes:
		assert.Equal(t, "second", c.Name)
	case <-time.After(5 * time.Second):
		t.Fatal("change is not detected")
	}
}
