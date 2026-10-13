package drivers_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/env"
	"github.com/insei/tinyconf/drivers/tag"
	"github.com/insei/tinyconf/drivers/yaml"
)

type pointersTLS struct {
	Cert string `env:"POINTERS_TLS_CERT" yaml:"cert" doc:"certificate path"`
	Key  string `env:"POINTERS_TLS_KEY" yaml:"key"`
}

type pointersServer struct {
	Host    string        `env:"POINTERS_HOST" yaml:"host"`
	Timeout time.Duration `env:"POINTERS_TIMEOUT" yaml:"timeout" initial:"5s"`
	TLS     *pointersTLS  `yaml:"tls"`
}

type pointersConfig struct {
	Name   string          `env:"POINTERS_NAME" yaml:"name"`
	Server *pointersServer `yaml:"server"`
}

func newPointersManager(t *testing.T, drivers ...tinyconf.Driver) *tinyconf.Manager {
	opts := make([]tinyconf.Option, 0, len(drivers))
	for _, driver := range drivers {
		opts = append(opts, tinyconf.WithDriver(driver))
	}
	manager, err := tinyconf.New(opts...)
	assert.NoError(t, err)
	return manager
}

func TestStructPointers_NoValues(t *testing.T) {
	envDriver, err := env.New()
	assert.NoError(t, err)
	manager := newPointersManager(t, envDriver)

	conf := &pointersConfig{}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))
	// structs are not created without values
	assert.Nil(t, conf.Server)
}

func TestStructPointers_Parse(t *testing.T) {
	yamlPath := filepath.Join(t.TempDir(), "config.yaml")
	assert.NoError(t, os.WriteFile(yamlPath, []byte("name: app\nserver:\n  host: yaml\n  tls:\n    cert: yaml.pem\n"), 0o600))
	t.Setenv("POINTERS_HOST", "env")
	t.Setenv("POINTERS_TLS_KEY", "env.key")

	tagDriver, err := tag.New("initial")
	assert.NoError(t, err)
	yamlDriver, err := yaml.New(yamlPath)
	assert.NoError(t, err)
	envDriver, err := env.New()
	assert.NoError(t, err)
	manager := newPointersManager(t, tagDriver, yamlDriver, envDriver)

	conf := &pointersConfig{}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))
	assert.Equal(t, pointersConfig{
		Name: "app",
		Server: &pointersServer{
			Host:    "env",
			Timeout: 5 * time.Second,
			TLS:     &pointersTLS{Cert: "yaml.pem", Key: "env.key"},
		},
	}, *conf)

	sources := map[string]string{}
	for _, source := range manager.Sources(conf) {
		sources[source.Field] = source.Driver
	}
	assert.Equal(t, map[string]string{
		"Name":            "yaml",
		"Server.Host":     "env",
		"Server.Timeout":  "tag",
		"Server.TLS.Cert": "yaml",
		"Server.TLS.Key":  "env",
	}, sources)

	// existing structs are filled, not replaced
	existing := &pointersConfig{Server: &pointersServer{TLS: &pointersTLS{}}}
	server, tls := existing.Server, existing.Server.TLS
	another := newPointersManager(t, tagDriver, yamlDriver, envDriver)
	assert.NoError(t, another.Register(existing))
	assert.NoError(t, another.Parse(existing))
	assert.Same(t, server, existing.Server)
	assert.Same(t, tls, existing.Server.TLS)
	assert.Equal(t, *conf, *existing)

	t.Run("sub config", func(t *testing.T) {
		sub := &pointersTLS{}
		assert.NoError(t, manager.Parse(sub))
		assert.Equal(t, pointersTLS{Cert: "yaml.pem", Key: "env.key"}, *sub)
	})
}

func TestStructPointers_Required(t *testing.T) {
	type db struct {
		DSN  string `env:"POINTERS_REQUIRED_DSN" required:"true"`
		Pool int    `env:"POINTERS_REQUIRED_POOL"`
	}
	type config struct {
		DB *db
	}
	envDriver, err := env.New()
	assert.NoError(t, err)
	manager := newPointersManager(t, envDriver)

	conf := &config{}
	assert.NoError(t, manager.Register(conf))
	err = manager.Parse(conf)
	assert.ErrorIs(t, err, tinyconf.ErrRequired)
	assert.ErrorContains(t, err, "DB.DSN")
	assert.Nil(t, conf.DB)

	t.Setenv("POINTERS_REQUIRED_DSN", "dsn")
	assert.NoError(t, manager.Parse(conf))
	assert.Equal(t, &db{DSN: "dsn"}, conf.DB)
}

func TestStructPointers_GenDoc(t *testing.T) {
	yamlDriver, err := yaml.New(filepath.Join(t.TempDir(), "missing.yaml"), yaml.WithOptional())
	assert.NoError(t, err)
	envDriver, err := env.New()
	assert.NoError(t, err)
	manager := newPointersManager(t, yamlDriver, envDriver)

	conf := &pointersConfig{Name: "app", Server: &pointersServer{Host: "localhost"}}
	assert.NoError(t, manager.Register(conf))

	yamlDoc := manager.GenDoc("yaml")
	for _, line := range []string{"name: app", "server:", "host: localhost", "tls:", "#certificate path", "cert:"} {
		assert.Contains(t, yamlDoc, line)
	}
	// struct is a section, it has no value
	assert.NotContains(t, yamlDoc, "server: {")

	envDoc := manager.GenDoc("env")
	for _, line := range []string{"POINTERS_NAME=app", "POINTERS_HOST=localhost", "POINTERS_TLS_CERT", "POINTERS_TLS_KEY"} {
		assert.Contains(t, envDoc, line)
	}
}

func TestStructPointers_Watch(t *testing.T) {
	yamlPath := filepath.Join(t.TempDir(), "config.yaml")
	assert.NoError(t, os.WriteFile(yamlPath, []byte("server:\n  host: first\n  tls:\n    cert: first.pem\n"), 0o600))
	yamlDriver, err := yaml.New(yamlPath, yaml.WithWatchInterval(10*time.Millisecond))
	assert.NoError(t, err)
	manager := newPointersManager(t, yamlDriver)

	conf := &pointersConfig{}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))
	server, tls := conf.Server, conf.Server.TLS

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan *pointersConfig, 1)
	done := make(chan error, 1)
	go func() {
		done <- manager.Watch(ctx, conf, func(fresh any, err error) {
			assert.NoError(t, err)
			select {
			case changed <- fresh.(*pointersConfig):
			default:
			}
		})
	}()

	time.Sleep(50 * time.Millisecond)
	assert.NoError(t, os.WriteFile(yamlPath, []byte("server:\n  host: second\n  tls:\n    cert: second.pem\n"), 0o600))
	select {
	case fresh := <-changed:
		assert.Equal(t, "second", fresh.Server.Host)
		assert.Equal(t, "second.pem", fresh.Server.TLS.Cert)
		assert.NotSame(t, server, fresh.Server)
		assert.NotSame(t, tls, fresh.Server.TLS)
	case <-time.After(5 * time.Second):
		t.Fatal("config change is not reported")
	}
	cancel()
	<-done

	// the config passed to Watch is not changed
	assert.Same(t, server, conf.Server)
	assert.Same(t, tls, conf.Server.TLS)
	assert.Equal(t, "first", conf.Server.Host)
	assert.Equal(t, "first.pem", conf.Server.TLS.Cert)
}
