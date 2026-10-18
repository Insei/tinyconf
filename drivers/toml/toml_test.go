package toml

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf/v2"
)

type testServer struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

type testConfig struct {
	HTTP struct {
		Host    string        `toml:"host"`
		Port    int           `toml:"port,omitempty"`
		Timeout time.Duration `toml:"timeout"`
	} `toml:"http"`
	Hosts    []string       `toml:"hosts"`
	Labels   map[string]int `toml:"labels"`
	Servers  []testServer   `toml:"servers"`
	Password string         `toml:"password" hidden:"true"`
	At       time.Time      `toml:"at"`
	Ptr      *int           `toml:"ptr"`
	Any      any            `toml:"any"`
	Skipped  string
}

func writeFile(t *testing.T, name, content string) string {
	filePath := filepath.Join(t.TempDir(), name)
	assert.NoError(t, os.WriteFile(filePath, []byte(content), 0o600))
	return filePath
}

func TestTOMLDriver(t *testing.T) {
	base := `
hosts = ["a", "b"]
at = 2024-01-02T03:04:05Z
any = [1, 2]

[http]
host = "localhost"
port = 80
timeout = "5s"

[labels]
a = 1

[[servers]]
host = "a"
port = 1

[[servers]]
host = "b"
`
	override := "hosts = [\"c\"]\n[http]\nport = 8080\n"
	expectedBase := func(c *testConfig) {
		c.HTTP.Host, c.HTTP.Port, c.HTTP.Timeout = "localhost", 80, 5*time.Second
		c.Hosts, c.Labels = []string{"a", "b"}, map[string]int{"a": 1}
		c.Servers = []testServer{{Host: "a", Port: 1}, {Host: "b"}}
		c.At = time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
		c.Any = []any{1, 2}
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
				return New(writeFile(t, "base.toml", base))
			},
			want: expectedBase,
		},
		{
			name: "next file overrides previous, nested tables are merged",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "base.toml", base), WithFiles(writeFile(t, "override.toml", override)))
			},
			want: expectedOverride,
		},
		{
			name: "fs and data",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				fsys := fstest.MapFS{"base.toml": {Data: []byte(base)}}
				return New("", WithFS(fsys, "base.toml"), WithData("override", []byte(override)))
			},
			want: expectedOverride,
		},
		{
			name: "absent file",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(filepath.Join(t.TempDir(), "absent.toml"))
			},
			want:    func(c *testConfig) {},
			wantErr: true,
		},
		{
			name: "absent optional file",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(filepath.Join(t.TempDir(), "absent.toml"), WithOptional())
			},
			want: func(c *testConfig) {},
		},
		{
			name: "empty file",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "empty.toml", ""))
			},
			want: func(c *testConfig) {},
		},
		{
			name: "incorrect toml",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "bad.toml", "[http"))
			},
			want:    func(c *testConfig) {},
			wantErr: true,
		},
		{
			name: "env expansion",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				t.Setenv("TINYCONF_TOML_HOST", "example.com")
				content := "[http]\nhost = \"${TINYCONF_TOML_HOST}\"\nport = ${TINYCONF_TOML_PORT:-443}\n"
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
			assert.Equal(t, "toml", driver.GetName())
			manager, err := tinyconf.New(tinyconf.WithDriver(driver))
			assert.NoError(t, err)

			conf, want := &testConfig{}, &testConfig{}
			tt.want(want)
			assert.NoError(t, manager.Register(conf))
			err = manager.Parse(context.Background(), conf)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, want, conf)
		})
	}
}

func TestTOMLDriver_GenDoc(t *testing.T) {
	conf := &testConfig{
		Hosts:    []string{"a", "b"},
		Labels:   map[string]int{"a": 1},
		Servers:  []testServer{{Host: "a", Port: 1}},
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
	assert.Contains(t, doc, "password = \"\"\n")
	assert.Contains(t, doc, "hosts = [\"a\", \"b\"]\n")
	assert.Contains(t, doc, "[http]\n")
	assert.Contains(t, doc, "  timeout = \"5s\"\n")
	assert.NotContains(t, doc, "secret")
	assert.NotContains(t, doc, "skipped")
	assert.NotContains(t, doc, "ptr")

	t.Run("doc can be parsed back", func(t *testing.T) {
		driver, err := New("", WithData("doc", []byte(doc)))
		assert.NoError(t, err)
		manager, err := tinyconf.New(tinyconf.WithDriver(driver))
		assert.NoError(t, err)
		parsed := &testConfig{}
		assert.NoError(t, manager.Register(parsed))
		assert.NoError(t, manager.Parse(context.Background(), parsed))
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
