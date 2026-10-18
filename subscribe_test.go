package tinyconf

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

type update[T any] struct {
	prev, next *T
}

// subscribe subscribes to the changes of T and returns the channel with the updates.
func subscribe[T any](t *testing.T, m *Manager) (<-chan update[T], context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	updates := make(chan update[T], 10)
	assert.NoError(t, Subscribe(ctx, m, func(prev, next *T) {
		updates <- update[T]{prev: prev, next: next}
	}))
	return updates, cancel
}

func receive[T any](t *testing.T, updates <-chan update[T]) update[T] {
	t.Helper()
	select {
	case u := <-updates:
		return u
	case <-time.After(5 * time.Second):
		t.Fatal("update is not received")
		return update[T]{}
	}
}

func nothing[T any](t *testing.T, updates <-chan update[T]) {
	t.Helper()
	select {
	case u := <-updates:
		t.Fatalf("unexpected update: %+v -> %+v", u.prev, u.next)
	case <-time.After(50 * time.Millisecond):
	}
}

type subHTTP struct {
	Host string
}

type subInner struct {
	Value string
}

type subSection struct {
	Level string
	Inner subInner
}

type subConfig struct {
	Name    string
	Default string
	HTTP    subHTTP
	Section *subSection
}

func TestSubscribe(t *testing.T) {
	file := newMapMockDriver("file", map[string]any{"Name": "first", "HTTP.Host": "a"})
	c, err := New(WithDriver(file))
	assert.NoError(t, err)
	conf := &subConfig{Default: "default"}
	assert.NoError(t, c.Register(conf))
	assert.NoError(t, c.Parse(context.Background(), conf))

	configs, _ := subscribe[subConfig](t, c)
	https, _ := subscribe[subHTTP](t, c)
	sections, _ := subscribe[subSection](t, c)

	// only the subscribers of the changed parts are called
	file.set(map[string]any{"Name": "second", "HTTP.Host": "a"})
	u := receive(t, configs)
	assert.Equal(t, "first", u.prev.Name)
	assert.Equal(t, subConfig{Name: "second", Default: "default", HTTP: subHTTP{Host: "a"}}, *u.next)
	nothing(t, https)
	nothing(t, sections)
	// the current version is replaced, the registered config is not changed
	assert.Same(t, u.next, Current[subConfig](c))
	assert.Equal(t, "first", conf.Name)

	file.set(map[string]any{"Name": "second", "HTTP.Host": "b"})
	h := receive(t, https)
	assert.Equal(t, subHTTP{Host: "a"}, *h.prev)
	assert.Equal(t, subHTTP{Host: "b"}, *h.next)
	assert.Same(t, h.next, &receive(t, configs).next.HTTP)
	assert.Equal(t, subHTTP{Host: "b"}, *Current[subHTTP](c))

	// the struct stored by pointer is created by the new value
	assert.Nil(t, Current[subSection](c))
	assert.Nil(t, Current[subInner](c))
	file.set(map[string]any{"Name": "second", "HTTP.Host": "b", "Section.Level": "debug"})
	s := receive(t, sections)
	assert.Nil(t, s.prev)
	assert.Equal(t, subSection{Level: "debug"}, *s.next)
	receive(t, configs)

	// the config with errors is not applied
	file.set(map[string]any{"Name": errors.New("broken")})
	nothing(t, configs)
	assert.Equal(t, "second", Current[subConfig](c).Name)

	// the value removed from the source gets the initial value, the reload starts from the values before drivers
	file.set(map[string]any{"HTTP.Host": "b", "Section.Level": "debug"})
	u = receive(t, configs)
	assert.Equal(t, subConfig{Default: "default", HTTP: subHTTP{Host: "b"}, Section: &subSection{Level: "debug"}}, *u.next)
	nothing(t, https)

	// the documentation has the current values
	assert.Contains(t, c.GenDoc("file"), "Host:b")
}

func TestSubscribe_BeforeParse(t *testing.T) {
	file := newMapMockDriver("file", map[string]any{"Name": "first"})
	c, err := New(WithDriver(file))
	assert.NoError(t, err)
	conf := &subConfig{Default: "default"}
	assert.NoError(t, c.Register(conf))
	assert.Nil(t, Current[subConfig](c))

	// the values of the registered config are the initial values
	configs, _ := subscribe[subConfig](t, c)
	file.set(map[string]any{"Name": "second"})
	u := receive(t, configs)
	assert.Equal(t, subConfig{Default: "default"}, *u.prev)
	assert.Equal(t, subConfig{Name: "second", Default: "default"}, *u.next)
}

func TestSubscribe_Parse(t *testing.T) {
	file := newMapMockDriver("file", map[string]any{"Name": "first", "HTTP.Host": "a"})
	c, err := New(WithDriver(file))
	assert.NoError(t, err)
	conf := &subConfig{}
	assert.NoError(t, c.Register(conf))
	configs, _ := subscribe[subConfig](t, c)

	// Parse updates the current version without the handlers
	assert.NoError(t, c.Parse(context.Background(), conf))
	nothing(t, configs)
	current := Current[subConfig](c)
	assert.Equal(t, "first", current.Name)
	// the current version is a copy, the application can change its config
	assert.NotSame(t, conf, current)
	conf.Name = "changed"
	assert.Equal(t, "first", current.Name)

	// Parse of the sub config does not change the current version
	file.values = map[string]any{"HTTP.Host": "b"}
	http := &subHTTP{}
	assert.NoError(t, c.Parse(context.Background(), http))
	assert.Equal(t, "b", http.Host)
	assert.Equal(t, "a", Current[subHTTP](c).Host)
}

func TestSubscribe_Errors(t *testing.T) {
	c, err := New(WithDriver(&staticMockDriver{}))
	assert.NoError(t, err)
	ctx := context.Background()
	assert.ErrorIs(t, Subscribe(ctx, c, func(prev, next *subConfig) {}), ErrNotRegisteredConfig)
	assert.Nil(t, Current[subConfig](c))
	assert.NoError(t, c.Register(&subConfig{}))
	assert.ErrorIs(t, Subscribe(ctx, c, func(prev, next *subConfig) {}), ErrNoWatchers)
}

// lifecycleMockDriver reports when watching is started and stopped.
type lifecycleMockDriver struct {
	started, stopped chan struct{}
}

func (md *lifecycleMockDriver) GenDoc(...*Registered) string        { return "" }
func (md *lifecycleMockDriver) GetName() string                     { return "lifecycle" }
func (md *lifecycleMockDriver) GetValue(fmap.Field) (*Value, error) { return nil, ErrValueNotFound }
func (md *lifecycleMockDriver) Watch(ctx context.Context, _ func()) error {
	md.started <- struct{}{}
	<-ctx.Done()
	md.stopped <- struct{}{}
	return nil
}

func TestSubscribe_Watching(t *testing.T) {
	driver := &lifecycleMockDriver{started: make(chan struct{}, 10), stopped: make(chan struct{}, 10)}
	// drivers without watching are skipped
	c, err := New(WithDriver(&staticMockDriver{}), WithDriver(driver))
	assert.NoError(t, err)
	assert.NoError(t, c.Register(&subConfig{}))
	wait := func(ch chan struct{}) {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("watching is not started or stopped")
		}
	}

	// one watching for all subscribers, it is stopped after the last subscription
	_, cancelFirst := subscribe[subConfig](t, c)
	_, cancelSecond := subscribe[subHTTP](t, c)
	wait(driver.started)
	cancelFirst()
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, driver.stopped, 0)
	assert.Len(t, driver.started, 0)
	cancelSecond()
	wait(driver.stopped)

	// the new subscription starts watching again
	_, cancel := subscribe[subConfig](t, c)
	wait(driver.started)
	cancel()
	wait(driver.stopped)
}

// errorsLogger records error messages with the details.
type errorsLogger struct {
	noopLogger
	mu     sync.Mutex
	errors []string
}

func (l *errorsLogger) Error(msg string, fields ...Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, field := range fields {
		if field.Key == "details" {
			msg += ": " + field.Value.(string)
		}
	}
	l.errors = append(l.errors, msg)
}

func (l *errorsLogger) With(...Field) Logger { return l }

func (l *errorsLogger) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	errs := l.errors
	l.errors = nil
	return errs
}

// failedWatchMockDriver can not watch.
type failedWatchMockDriver struct {
	started chan struct{}
}

func (md *failedWatchMockDriver) GenDoc(...*Registered) string        { return "" }
func (md *failedWatchMockDriver) GetName() string                     { return "failed" }
func (md *failedWatchMockDriver) GetValue(fmap.Field) (*Value, error) { return nil, ErrValueNotFound }
func (md *failedWatchMockDriver) Watch(context.Context, func()) error {
	md.started <- struct{}{}
	return errors.New("connection lost")
}

func TestSubscribe_LoggedErrors(t *testing.T) {
	log := &errorsLogger{}
	file := newMapMockDriver("file", map[string]any{"Name": "first"})
	failed := &failedWatchMockDriver{started: make(chan struct{}, 1)}
	c, err := New(WithLogger(log), WithDriver(file), WithDriver(failed))
	assert.NoError(t, err)
	conf := &subConfig{}
	assert.NoError(t, c.Register(conf))
	assert.NoError(t, c.Parse(context.Background(), conf))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, Subscribe(ctx, c, func(prev, next *subConfig) {
		panic("broken handler")
	}))
	configs, _ := subscribe[subConfig](t, c)
	<-failed.started

	// the failed watcher is logged, other watchers continue
	file.set(map[string]any{"Name": "second"})
	// the panic is logged, the next subscribers are called
	assert.Equal(t, "second", receive(t, configs).next.Name)
	assert.ElementsMatch(t, []string{
		"failed to watch: connection lost",
		"subscriber panicked: broken handler",
	}, log.take())

	file.set(map[string]any{"Name": errors.New("broken")})
	nothing(t, configs)
	// the error of the field and the result of the reload
	assert.Equal(t, []string{"failed: broken", "failed to reload: file driver: field Name: broken"}, log.take())
}

func TestManager_reload(t *testing.T) {
	file := newMapMockDriver("file", map[string]any{"Name": "first"})
	c, err := New(WithDriver(file))
	assert.NoError(t, err)
	conf := &subConfig{}
	assert.NoError(t, c.Register(conf))
	assert.NoError(t, c.Parse(context.Background(), conf))
	called := false
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.subscribers = []*subscriber{{ctx: ctx, root: reflect.TypeOf(conf), notify: func(any, any) { called = true }}}
	file.values = map[string]any{"Name": "second"}

	// the stopped watching does not reload
	c.reload(ctx)
	assert.Equal(t, "first", Current[subConfig](c).Name)
	// the canceled subscription is not called
	c.reload(context.Background())
	assert.Equal(t, "second", Current[subConfig](c).Name)
	assert.False(t, called)
}
