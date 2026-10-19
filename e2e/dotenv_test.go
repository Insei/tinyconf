package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insei/tinyconf/v2"
	"github.com/insei/tinyconf/v2/drivers/env"
	"github.com/insei/tinyconf/v2/drivers/tag"
	"github.com/insei/tinyconf/v2/drivers/yaml"
)

type dotenvHTTP struct {
	Host string `yaml:"host" env:"E2E_DOTENV_HTTP_HOST" default:"localhost"`
	Port int    `yaml:"port" env:"E2E_DOTENV_HTTP_PORT" default:"8080"`
}

type dotenvDatabase struct {
	Pool     int    `yaml:"pool" env:"E2E_DOTENV_DB_POOL"`
	Password string `env:"E2E_DOTENV_DB_PASSWORD"`
}

type dotenvConfig struct {
	HTTP     dotenvHTTP     `yaml:"http"`
	Database dotenvDatabase `yaml:"database"`
}

// TestDotenv is the config of a service from the defaults, the yaml file, the .env file and the process environment,
// the changes of the .env file are reloaded without restart.
func TestDotenv(t *testing.T) {
	exported := []string{"E2E_DOTENV_HTTP_HOST", "E2E_DOTENV_HTTP_PORT", "E2E_DOTENV_DB_PASSWORD"}
	for _, key := range exported {
		require.NoError(t, os.Unsetenv(key))
	}
	t.Cleanup(func() {
		for _, key := range exported {
			_ = os.Unsetenv(key)
		}
	})
	// the real environment of the process has priority over the .env file
	t.Setenv("E2E_DOTENV_DB_POOL", "7")

	dir := t.TempDir()
	configFile, envFile := filepath.Join(dir, "config.yaml"), filepath.Join(dir, ".env")
	write := func(file, content string) {
		t.Helper()
		require.NoError(t, os.WriteFile(file, []byte(content), 0o600))
	}
	write(configFile, "http:\n  host: yaml.example.com\ndatabase:\n  pool: 3\n")
	write(envFile, "E2E_DOTENV_HTTP_PORT=9090\nE2E_DOTENV_DB_POOL=5\nE2E_DOTENV_DB_PASSWORD=first\n")

	tagDriver, err := tag.New("default")
	require.NoError(t, err)
	yamlDriver, err := yaml.New(configFile, yaml.WithWatchInterval(100*time.Millisecond))
	require.NoError(t, err)
	envDriver, err := env.New(env.WithFiles(envFile), env.WithWatchInterval(100*time.Millisecond))
	require.NoError(t, err)
	manager := newManager(t, tagDriver, yamlDriver, envDriver)
	conf := &dotenvConfig{}
	require.NoError(t, manager.Register(conf))
	require.NoError(t, manager.Parse(context.Background(), conf))
	assert.Equal(t, dotenvConfig{
		HTTP:     dotenvHTTP{Host: "yaml.example.com", Port: 9090},
		Database: dotenvDatabase{Pool: 7, Password: "first"},
	}, *conf)
	assert.Equal(t, "9090", os.Getenv("E2E_DOTENV_HTTP_PORT"))

	// reloaded checks that the change is reloaded, when the subscribers are not called
	reloaded := func(t *testing.T, since time.Time) {
		t.Helper()
		assert.True(t, manager.ReloadStatus().LastReload.After(since), "change is not reloaded")
	}

	https := subscribe[dotenvHTTP](t, manager)
	databases := subscribe[dotenvDatabase](t, manager)

	t.Run("change of the .env file reloads only its part", func(t *testing.T) {
		write(envFile, "E2E_DOTENV_HTTP_PORT=9191\nE2E_DOTENV_DB_POOL=5\nE2E_DOTENV_DB_PASSWORD=first\n")
		assert.Equal(t, dotenvHTTP{Host: "yaml.example.com", Port: 9191}, *next(t, https))
		noReload(t, databases)
		assert.Equal(t, "9191", os.Getenv("E2E_DOTENV_HTTP_PORT"))
	})

	t.Run("the process environment keeps its values", func(t *testing.T) {
		since := time.Now()
		write(envFile, "E2E_DOTENV_HTTP_PORT=9191\nE2E_DOTENV_DB_POOL=50\nE2E_DOTENV_DB_PASSWORD=first\n")
		noReload(t, databases)
		reloaded(t, since)
		assert.Equal(t, "7", os.Getenv("E2E_DOTENV_DB_POOL"))
		assert.Equal(t, 7, tinyconf.Current[dotenvDatabase](manager).Pool)
	})

	t.Run("variable removed from the .env file is removed from the environment", func(t *testing.T) {
		write(envFile, "E2E_DOTENV_HTTP_PORT=9191\nE2E_DOTENV_DB_POOL=50\n")
		assert.Equal(t, dotenvDatabase{Pool: 7}, *next(t, databases))
		_, exist := os.LookupEnv("E2E_DOTENV_DB_PASSWORD")
		assert.False(t, exist)
	})

	t.Run("variable added to the .env file overrides the yaml file", func(t *testing.T) {
		write(envFile, "E2E_DOTENV_HTTP_PORT=9191\nE2E_DOTENV_DB_POOL=50\nE2E_DOTENV_HTTP_HOST=env.example.com\n")
		assert.Equal(t, dotenvHTTP{Host: "env.example.com", Port: 9191}, *next(t, https))
	})

	t.Run("change of the yaml file under the .env value", func(t *testing.T) {
		since := time.Now()
		write(configFile, "http:\n  host: new.example.com\ndatabase:\n  pool: 4\n")
		noReload(t, https)
		noReload(t, databases)
		reloaded(t, since)
	})

	t.Run("broken .env file keeps the current version", func(t *testing.T) {
		require.NoError(t, os.Remove(envFile))
		noReload(t, https)
		assert.Error(t, manager.ReloadStatus().LastError)

		write(envFile, "E2E_DOTENV_HTTP_PORT=9292\n")
		assert.Equal(t, dotenvHTTP{Host: "new.example.com", Port: 9292}, *next(t, https))
	})

	// the config of the application is never changed by reloads
	assert.Equal(t, 9090, conf.HTTP.Port)
}
