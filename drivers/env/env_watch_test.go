package env

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf/v2"
)

func TestEnvDriver_Watch(t *testing.T) {
	type config struct {
		Name    string `env:"TINYCONF_WATCH_NAME"`
		Default string
	}
	os.Unsetenv("TINYCONF_WATCH_NAME")
	t.Cleanup(func() { os.Unsetenv("TINYCONF_WATCH_NAME") })
	filePath := path.Join(t.TempDir(), ".env")
	assert.NoError(t, os.WriteFile(filePath, []byte("TINYCONF_WATCH_NAME=first\n"), 0o600))
	driver, err := New(WithFiles(filePath), WithWatchInterval(5*time.Millisecond))
	assert.NoError(t, err)
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)

	conf := &config{Default: "default"}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(context.Background(), conf))
	assert.Equal(t, "first", conf.Name)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan *config, 10)
	assert.NoError(t, tinyconf.Subscribe(ctx, manager, func(prev, next *config) {
		changes <- next
	}))

	assert.NoError(t, os.WriteFile(filePath, []byte("TINYCONF_WATCH_NAME=second value\n"), 0o600))
	select {
	case newConf := <-changes:
		assert.Equal(t, &config{Name: "second value", Default: "default"}, newConf)
		assert.Equal(t, "second value", os.Getenv("TINYCONF_WATCH_NAME"))
	case <-time.After(5 * time.Second):
		t.Fatal("change is not detected")
	}
}

func TestEnvDriver_Load(t *testing.T) {
	keys := []string{"TINYCONF_LOAD_OWN", "TINYCONF_LOAD_REMOVED", "TINYCONF_LOAD_REAL", "TINYCONF_LOAD_CHANGED", "TINYCONF_LOAD_KEPT"}
	unset := func() {
		for _, key := range keys {
			os.Unsetenv(key)
		}
	}
	unset()
	t.Cleanup(unset)
	dir := t.TempDir()
	filePath := path.Join(dir, ".env")
	write := func(content string) {
		assert.NoError(t, os.WriteFile(filePath, []byte(content), 0o600))
	}
	lookup := func(key string) string {
		val, ok := os.LookupEnv(key)
		if !ok {
			return "<unset>"
		}
		return val
	}

	t.Run("export to the process environment", func(t *testing.T) {
		t.Cleanup(unset)
		write("TINYCONF_LOAD_OWN=1\nTINYCONF_LOAD_REMOVED=1\nTINYCONF_LOAD_REAL=1\nTINYCONF_LOAD_CHANGED=1\nTINYCONF_LOAD_KEPT=1\n")
		os.Setenv("TINYCONF_LOAD_REAL", "real")
		driver, err := New(WithFiles(filePath))
		assert.NoError(t, err)
		// changed by the application after the export
		os.Setenv("TINYCONF_LOAD_CHANGED", "app")
		os.Setenv("TINYCONF_LOAD_KEPT", "app")

		write("TINYCONF_LOAD_OWN=2\nTINYCONF_LOAD_REAL=2\nTINYCONF_LOAD_CHANGED=2\n")
		assert.NoError(t, driver.(tinyconf.Loader).Load())
		assert.Equal(t, "2", lookup("TINYCONF_LOAD_OWN"))
		assert.Equal(t, "<unset>", lookup("TINYCONF_LOAD_REMOVED"))
		assert.Equal(t, "real", lookup("TINYCONF_LOAD_REAL"))
		assert.Equal(t, "app", lookup("TINYCONF_LOAD_CHANGED"))
		assert.Equal(t, "app", lookup("TINYCONF_LOAD_KEPT"))
	})

	t.Run("isolation", func(t *testing.T) {
		t.Cleanup(unset)
		write("TINYCONF_LOAD_OWN=1\nTINYCONF_LOAD_REMOVED=1\n")
		driver, err := New(WithFiles(filePath), WithFilesIsolation())
		assert.NoError(t, err)
		d := driver.(*envDriver)
		assert.Equal(t, map[string]string{"TINYCONF_LOAD_OWN": "1", "TINYCONF_LOAD_REMOVED": "1"}, d.values)

		write("TINYCONF_LOAD_OWN=2\n")
		assert.NoError(t, d.Load())
		assert.Equal(t, map[string]string{"TINYCONF_LOAD_OWN": "2"}, d.values)
		assert.Equal(t, "<unset>", lookup("TINYCONF_LOAD_OWN"))
	})

	t.Run("removed file", func(t *testing.T) {
		t.Cleanup(unset)
		write("TINYCONF_LOAD_OWN=1\n")
		driver, err := New(WithFiles(filePath))
		assert.NoError(t, err)
		assert.NoError(t, os.Remove(filePath))
		assert.Error(t, driver.(tinyconf.Loader).Load())
		assert.Equal(t, "1", lookup("TINYCONF_LOAD_OWN"))
	})

	t.Run("default files are optional", func(t *testing.T) {
		d := &envDriver{files: []envFile{{path: path.Join(dir, "absent", ".env"), optional: true}}, exported: map[string]string{}}
		assert.NoError(t, d.Load())
		assert.Equal(t, ";", d.stat())
	})
}
