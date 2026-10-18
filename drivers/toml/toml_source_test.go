package toml

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf/v2"
	"github.com/insei/tinyconf/v2/drivers/internal/tree"
)

func TestTomlDriver_WithSource(t *testing.T) {
	type config struct {
		Name string `toml:"name"`
	}
	driver, err := New("", WithSource(tree.DataSource("source", []byte("name = \"source\""))))
	assert.NoError(t, err)
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)
	conf := &config{}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(context.Background(), conf))
	assert.Equal(t, "source", conf.Name)
}
