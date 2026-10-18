package tinyconf

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestManager_ReloadStatus(t *testing.T) {
	file := newMapMockDriver("file", map[string]any{"Name": "first"})
	failed := &failedWatchMockDriver{started: make(chan struct{}, 10)}
	c, err := New(WithDriver(file), WithDriver(failed))
	assert.NoError(t, err)
	conf := &subConfig{}
	assert.NoError(t, c.Register(conf))
	assert.NoError(t, c.Parse(context.Background(), conf))
	assert.Equal(t, ReloadStatus{}, c.ReloadStatus())

	ctx, cancel := context.WithCancel(context.Background())
	configs := make(chan *subConfig, 10)
	assert.NoError(t, Subscribe(ctx, c, func(prev, next *subConfig) { configs <- next }))
	<-failed.started
	// the driver that can not watch is in the status, other drivers continue
	assert.Eventually(t, func() bool { return len(c.ReloadStatus().StoppedDrivers) == 1 }, 5*time.Second, time.Millisecond)
	status := c.ReloadStatus()
	assert.True(t, status.Watching)
	assert.EqualError(t, status.StoppedDrivers["failed"], "connection lost")
	// the status has the copy of the drivers
	delete(status.StoppedDrivers, "failed")
	assert.Len(t, c.ReloadStatus().StoppedDrivers, 1)

	// the applied reload
	file.set(map[string]any{"Name": "second"})
	<-configs
	status = c.ReloadStatus()
	assert.NoError(t, status.LastError)
	assert.False(t, status.LastReload.IsZero())
	assert.Equal(t, status.LastReload, status.LastApplied)
	applied := status.LastApplied

	// the reload with the error is not applied
	file.set(map[string]any{"Name": errors.New("broken")})
	assert.Eventually(t, func() bool { return c.ReloadStatus().LastError != nil }, 5*time.Second, time.Millisecond)
	status = c.ReloadStatus()
	assert.ErrorContains(t, status.LastError, "config *tinyconf.subConfig: file driver: field Name: broken")
	assert.Equal(t, applied, status.LastApplied)
	assert.True(t, status.LastReload.After(applied))

	// watching is stopped with the last subscription
	cancel()
	assert.Eventually(t, func() bool { return !c.ReloadStatus().Watching }, 5*time.Second, time.Millisecond)

	// watching starts again with the new subscription
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, Subscribe(ctx, c, func(prev, next *subConfig) {}))
	assert.True(t, c.ReloadStatus().Watching)
	<-failed.started
}
