package json

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
)

func TestJSONDriver_Watch(t *testing.T) {
	type config struct {
		Name    string `json:"name"`
		Default string
	}
	filePath := writeFile(t, "config.json", `{"name": "first"}`)
	driver, err := New(filePath, WithWatchInterval(5*time.Millisecond))
	assert.NoError(t, err)
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)

	conf := &config{Default: "default"}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))

	ctx, cancel := context.WithCancel(context.Background())
	changes, done := make(chan *config, 10), make(chan error, 1)
	go func() {
		done <- manager.Watch(ctx, conf, func(newConf any, err error) {
			assert.NoError(t, err)
			changes <- newConf.(*config)
		})
	}()

	assert.NoError(t, os.WriteFile(filePath, []byte(`{"name": "second value"}`), 0o600))
	select {
	case newConf := <-changes:
		assert.Equal(t, &config{Name: "second value", Default: "default"}, newConf)
		assert.Equal(t, "first", conf.Name)
	case <-time.After(5 * time.Second):
		t.Fatal("change is not detected")
	}
	cancel()
	assert.NoError(t, <-done)
}
