package tinyconf

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

// burstWatchMockDriver reports several changes while the first one is still handled,
// its value is changed after every reading.
type burstWatchMockDriver struct {
	handling chan struct{}
	reported chan struct{}
	reads    atomic.Int32
}

func (md *burstWatchMockDriver) GenDoc(...*Registered) string { return "" }
func (md *burstWatchMockDriver) GetName() string              { return "burst" }
func (md *burstWatchMockDriver) GetValue(fmap.Field) (*Value, error) {
	return &Value{Source: "burst", Value: fmt.Sprint(md.reads.Add(1))}, nil
}

func (md *burstWatchMockDriver) Watch(ctx context.Context, onChange func()) error {
	onChange()
	// the first change is taken for handling, the next is waiting, the others are dropped
	<-md.handling
	onChange()
	onChange()
	onChange()
	close(md.reported)
	<-ctx.Done()
	return nil
}

func TestSubscribe_ChangesBurst(t *testing.T) {
	type Config struct {
		Name string
	}
	driver := &burstWatchMockDriver{handling: make(chan struct{}), reported: make(chan struct{})}
	c, err := New(WithDriver(driver))
	assert.NoError(t, err)
	assert.NoError(t, c.Register(&Config{}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handled := make(chan struct{}, 10)
	first := true
	assert.NoError(t, Subscribe(ctx, c, func(prev, next *Config) {
		if first {
			first = false
			close(driver.handling)
			<-driver.reported
		}
		handled <- struct{}{}
	}))

	// four changes are reported, but the config is reloaded only twice
	for i := 0; i < 2; i++ {
		select {
		case <-handled:
		case <-time.After(5 * time.Second):
			t.Fatal("change is not handled")
		}
	}
	select {
	case <-handled:
		t.Fatal("dropped change is handled")
	case <-time.After(50 * time.Millisecond):
	}
	assert.Equal(t, int32(2), driver.reads.Load())
}
