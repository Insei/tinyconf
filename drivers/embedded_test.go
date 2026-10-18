package drivers_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/env"
	"github.com/insei/tinyconf/drivers/json"
	"github.com/insei/tinyconf/drivers/toml"
	"github.com/insei/tinyconf/drivers/yaml"
)

type embeddedCommon struct {
	Port int `yaml:"port" json:"port" toml:"port"`
}

type embeddedConfig struct {
	embeddedCommon
	Name string `yaml:"name" json:"name" toml:"name"`
	HTTP struct {
		*embeddedCommon
		Host string `yaml:"host" json:"host" toml:"host"`
	} `yaml:"http" json:"http" toml:"http"`
}

// warningLogger counts warnings of the manager.
type warningLogger struct {
	warnings *int
}

func (l warningLogger) Debug(string, ...tinyconf.Field)        {}
func (l warningLogger) Error(string, ...tinyconf.Field)        {}
func (l warningLogger) Info(string, ...tinyconf.Field)         {}
func (l warningLogger) Warn(string, ...tinyconf.Field)         { *l.warnings++ }
func (l warningLogger) With(...tinyconf.Field) tinyconf.Logger { return l }

// Fields of the embedded structs are on the level of the embedding struct, like in encoding/json.
func TestEmbeddedStructs(t *testing.T) {
	newDriver := map[string]func() (tinyconf.Driver, error){
		"yaml": func() (tinyconf.Driver, error) {
			return yaml.New("", yaml.WithData("data", []byte("port: 1\nname: app\nhttp:\n  port: 2\n  host: example.com\n")))
		},
		"json": func() (tinyconf.Driver, error) {
			return json.New("", json.WithData("data", []byte(`{"port": 1, "name": "app", "http": {"port": 2, "host": "example.com"}}`)))
		},
		"toml": func() (tinyconf.Driver, error) {
			return toml.New("", toml.WithData("data", []byte("port = 1\nname = \"app\"\n[http]\nport = 2\nhost = \"example.com\"\n")))
		},
	}
	for name, newDriver := range newDriver {
		t.Run(name, func(t *testing.T) {
			driver, err := newDriver()
			assert.NoError(t, err)
			warnings := 0
			manager, err := tinyconf.New(
				tinyconf.WithDriver(driver),
				tinyconf.WithLogger(warningLogger{warnings: &warnings}),
			)
			assert.NoError(t, err)
			conf := &embeddedConfig{}
			assert.NoError(t, manager.Register(conf))
			assert.NoError(t, manager.Parse(context.Background(), conf))

			assert.Equal(t, 1, conf.Port)
			assert.Equal(t, "app", conf.Name)
			assert.Equal(t, 2, conf.HTTP.Port)
			assert.Equal(t, "example.com", conf.HTTP.Host)
			// embedded structs without the tag are expected
			assert.Equal(t, 0, warnings)
			// fields of the embedded structs are in the documentation
			assert.Contains(t, manager.GenDoc(name), "example.com")
			assert.Equal(t, 2, strings.Count(manager.GenDoc(name), "port"))
		})
	}
}

func TestEmbeddedStructs_Env(t *testing.T) {
	type server struct {
		embeddedCommon
		Name string
	}
	type config struct {
		embeddedCommon
		HTTP struct {
			embeddedCommon
		}
		Servers []server `env:"EMBEDDED_SERVERS"`
	}
	t.Setenv("PORT", "1")
	t.Setenv("HTTP_PORT", "2")
	t.Setenv("EMBEDDED_SERVERS_0_PORT", "3")
	t.Setenv("EMBEDDED_SERVERS_0_NAME", "first")
	driver, err := env.New(env.WithFiles(), env.WithAutoKeys(), env.WithIndexedKeys())
	assert.NoError(t, err)
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	conf := &config{}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(context.Background(), conf))

	// auto keys are built without the names of the embedded structs
	assert.Equal(t, 1, conf.Port)
	assert.Equal(t, 2, conf.HTTP.Port)
	// indexed keys of the embedded structs are on the level of the embedding struct
	assert.Equal(t, []server{{embeddedCommon: embeddedCommon{Port: 3}, Name: "first"}}, conf.Servers)
	assert.Contains(t, manager.GenDoc("env"), "#HTTP_PORT=2\n")
}
