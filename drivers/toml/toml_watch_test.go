package toml

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
)

func TestTOMLDriver_Watch(t *testing.T) {
	type config struct {
		Name    string `toml:"name"`
		Default string
	}
	filePath := writeFile(t, "config.toml", `name = "first"`)
	driver, err := New(filePath, WithWatchInterval(5*time.Millisecond))
	assert.NoError(t, err)
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)

	conf := &config{Default: "default"}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan *config, 10)
	assert.NoError(t, tinyconf.Subscribe(ctx, manager, func(prev, next *config) {
		changes <- next
	}))

	assert.NoError(t, os.WriteFile(filePath, []byte(`name = "second value"`), 0o600))
	select {
	case newConf := <-changes:
		assert.Equal(t, &config{Name: "second value", Default: "default"}, newConf)
		assert.Equal(t, "first", conf.Name)
	case <-time.After(5 * time.Second):
		t.Fatal("change is not detected")
	}
}
