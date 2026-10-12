package json

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
)

type testConfig struct {
	HTTP struct {
		Host    string        `json:"host"`
		Port    int           `json:"port,omitempty"`
		Timeout time.Duration `json:"timeout"`
	} `json:"http"`
	Hosts    []string       `json:"hosts"`
	Labels   map[string]int `json:"labels"`
	Password string         `json:"password" hidden:"true"`
	At       time.Time      `json:"at"`
	Skipped  string
}

func writeFile(t *testing.T, name, content string) string {
	filePath := filepath.Join(t.TempDir(), name)
	assert.NoError(t, os.WriteFile(filePath, []byte(content), 0o600))
	return filePath
}

func TestJSONDriver(t *testing.T) {
	base := `{"http": {"host": "localhost", "port": 80, "timeout": "5s"}, "hosts": ["a", "b"], "labels": {"a": 1}}`
	override := `{"http": {"port": 8080}, "hosts": ["c"]}`
	expectedBase := func(c *testConfig) {
		c.HTTP.Host, c.HTTP.Port, c.HTTP.Timeout = "localhost", 80, 5*time.Second
		c.Hosts, c.Labels = []string{"a", "b"}, map[string]int{"a": 1}
	}
	expectedOverride := func(c *testConfig) {
		expectedBase(c)
		c.HTTP.Port, c.Hosts = 8080, []string{"c"}
	}
	tests := []struct {
		name    string
		driver  func(t *testing.T) (tinyconf.Driver, error)
		want    func(c *testConfig)
		wantErr bool
	}{
		{
			name: "single file",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "base.json", base))
			},
			want: expectedBase,
		},
		{
			name: "next file overrides previous, nested objects are merged",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "base.json", base), WithFiles(writeFile(t, "override.json", override)))
			},
			want: expectedOverride,
		},
		{
			name: "fs and data",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				fsys := fstest.MapFS{"base.json": {Data: []byte(base)}}
				return New("", WithFS(fsys, "base.json"), WithData("override", []byte(override)))
			},
			want: expectedOverride,
		},
		{
			name: "absent file",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(filepath.Join(t.TempDir(), "absent.json"))
			},
			want:    func(c *testConfig) {},
			wantErr: true,
		},
		{
			name: "absent optional file",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(filepath.Join(t.TempDir(), "absent.json"), WithOptional())
			},
			want: func(c *testConfig) {},
		},
		{
			name: "empty file",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "empty.json", " \n"))
			},
			want: func(c *testConfig) {},
		},
		{
			name: "null",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "null.json", "null"))
			},
			want: func(c *testConfig) {},
		},
		{
			name: "incorrect json",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "bad.json", `{"http": `))
			},
			want:    func(c *testConfig) {},
			wantErr: true,
		},
		{
			name: "not an object",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "list.json", `[1]`))
			},
			want:    func(c *testConfig) {},
			wantErr: true,
		},
		{
			name: "env expansion",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				t.Setenv("TINYCONF_JSON_HOST", "example.com")
				content := `{"http": {"host": "${TINYCONF_JSON_HOST}", "port": ${TINYCONF_JSON_PORT:-443}}}`
				return New("", WithData("config", []byte(content)), WithEnvExpansion())
			},
			want: func(c *testConfig) {
				c.HTTP.Host, c.HTTP.Port = "example.com", 443
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver, err := tt.driver(t)
			assert.NoError(t, err)
			assert.Equal(t, "json", driver.GetName())
			manager, err := tinyconf.New(tinyconf.WithDriver(driver))
			assert.NoError(t, err)

			conf, want := &testConfig{}, &testConfig{}
			tt.want(want)
			assert.NoError(t, manager.Register(conf))
			err = manager.Parse(conf)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, want, conf)
		})
	}
}

func TestJSONDriver_GenDoc(t *testing.T) {
	conf := &testConfig{
		Hosts:    []string{"a", "b"},
		Labels:   map[string]int{"a": 1},
		Password: "secret",
		At:       time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		Skipped:  "skipped",
	}
	conf.HTTP.Host, conf.HTTP.Port, conf.HTTP.Timeout = "localhost", 80, 5*time.Second
	storage, err := fmap.GetFrom(conf)
	assert.NoError(t, err)

	driver, err := New("")
	assert.NoError(t, err)
	doc := driver.GenDoc(&tinyconf.Registered{Storage: storage, Config: conf})
	assert.Equal(t, `{
  "at": "2024-01-02T03:04:05Z",
  "hosts": [
    "a",
    "b"
  ],
  "http": {
    "host": "localhost",
    "port": 80,
    "timeout": "5s"
  },
  "labels": {
    "a": 1
  },
  "password": ""
}
`, doc)

	t.Run("doc can be parsed back", func(t *testing.T) {
		driver, err := New("", WithData("doc", []byte(doc)))
		assert.NoError(t, err)
		manager, err := tinyconf.New(tinyconf.WithDriver(driver))
		assert.NoError(t, err)
		parsed := &testConfig{}
		assert.NoError(t, manager.Register(parsed))
		assert.NoError(t, manager.Parse(parsed))
		conf.Password, conf.Skipped = "", ""
		assert.Equal(t, conf, parsed)
	})

	t.Run("no fields", func(t *testing.T) {
		empty := &struct{ Skipped string }{}
		storage, err := fmap.GetFrom(empty)
		assert.NoError(t, err)
		assert.Equal(t, "", driver.GenDoc(&tinyconf.Registered{Storage: storage, Config: empty}))
	})
}
