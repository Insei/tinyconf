package tinyconf

import (
	"context"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

// contextLoaderMockDriver records which loading is called and with which context.
type contextLoaderMockDriver struct {
	loaded  bool
	context context.Context
}

func (md *contextLoaderMockDriver) GenDoc(...*Registered) string        { return "" }
func (md *contextLoaderMockDriver) GetName() string                     { return "context" }
func (md *contextLoaderMockDriver) GetValue(fmap.Field) (*Value, error) { return nil, ErrValueNotFound }
func (md *contextLoaderMockDriver) Load() error {
	md.loaded = true
	return nil
}

func (md *contextLoaderMockDriver) LoadContext(ctx context.Context) error {
	md.context = ctx
	return ctx.Err()
}

type contextKey struct{}

func TestManager_ParseContext(t *testing.T) {
	type Config struct {
		Name string
	}
	driver := &contextLoaderMockDriver{}
	c, err := New(WithDriver(driver))
	assert.NoError(t, err)
	conf := &Config{}
	assert.NoError(t, c.Register(conf))

	// LoadContext is called instead of Load with the context of ParseContext
	ctx := context.WithValue(context.Background(), contextKey{}, "value")
	assert.NoError(t, c.ParseContext(ctx, conf))
	assert.False(t, driver.loaded)
	assert.Equal(t, "value", driver.context.Value(contextKey{}))

	// Parse uses the background context
	assert.NoError(t, c.Parse(conf))
	assert.Nil(t, driver.context.Value(contextKey{}))

	// the canceled context is the error of the driver
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, c.ParseContext(ctx, conf), context.Canceled)
}
