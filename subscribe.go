package tinyconf

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/insei/fmap/v3"

	"github.com/insei/tinyconf/convert"
)

// configState is the state of the registered config.
type configState struct {
	// initial values of the config before drivers at the first Parse, every reload starts from them
	initial any
	// current version of the config after the last Parse or reload, it is never changed after storing
	current atomic.Value
}

// subscriber is the subscription to the changes of the registered config or its sub config.
type subscriber struct {
	ctx    context.Context
	root   reflect.Type
	prefix string
	// notify calls the handler with the parts of the versions of the registered config
	notify func(prev, next any)
}

// copyConf returns the copy of the config, structs stored by pointer are copied too.
func copyConf(conf any) any {
	src := reflect.ValueOf(conf)
	dst := reflect.New(src.Type().Elem())
	dst.Elem().Set(src.Elem())
	convert.CopySections(dst.Interface())
	return dst.Interface()
}

// state returns the state of the registered config type, it is created by Register.
func (c *Manager) state(typeOf reflect.Type) *configState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.states[typeOf]
}

// part returns the pointer to the sub config with the path inside of the version of the registered config.
func part[T any](storage fmap.Storage, subPath string, root any) *T {
	if root == nil {
		return nil
	}
	if subPath == "" {
		return root.(*T)
	}
	ptr := reflect.ValueOf(storage.MustFind(subPath).GetPtr(root))
	if ptr.IsNil() {
		// the struct with the sub config stored by pointer is not created
		return nil
	}
	if ptr.Elem().Kind() == reflect.Ptr {
		// the sub config stored by pointer
		val, _ := ptr.Elem().Interface().(*T)
		return val
	}
	return ptr.Interface().(*T)
}

// Current returns the current version of the config or the sub config with type T:
// the result of the last Parse of the registered config or of the last reload after the changes of the sources.
// The returned value is shared by all readers and must not be changed.
// Nil is returned if the config is not registered or not parsed yet,
// and if the sub config is stored by pointer and is not created.
func Current[T any](m *Manager) *T {
	register, registerTypeOf, subPath := m.findRegister(reflect.TypeOf((*T)(nil)))
	if register == nil {
		return nil
	}
	return part[T](register.Storage, subPath, m.state(registerTypeOf).current.Load())
}

// Subscribe subscribes to the changes of the config or the sub config with type T, it does not block.
// The subscription is active until ctx is done.
//
// The manager watches for changes of the sources while there are active subscriptions,
// by all drivers that implement Watcher, ErrNoWatchers is returned if there are no such drivers.
// After every change the registered configs with subscribers are parsed again, starting from their values
// before drivers at the first Parse, so the result is the same as Parse would give at the start.
// onChange is called only if some fields of the config or the sub config are changed,
// with the previous and the new versions, they are shared with other readers and must not be changed.
// The config with errors is not applied: the errors are logged and onChange is not called.
//
// Handlers are called one by one in the order of subscription, the next reload starts after all handlers return.
// Parse calls of the application update the current version, but do not call the handlers.
func Subscribe[T any](ctx context.Context, m *Manager, onChange func(prev, next *T)) error {
	register, registerTypeOf, subPath := m.findRegister(reflect.TypeOf((*T)(nil)))
	if register == nil {
		return ErrNotRegisteredConfig
	}
	if !m.hasWatchers() {
		return ErrNoWatchers
	}
	prefix := ""
	if subPath != "" {
		prefix = subPath + "."
	}
	sub := &subscriber{
		ctx:    ctx,
		root:   registerTypeOf,
		prefix: prefix,
		notify: func(prev, next any) {
			onChange(part[T](register.Storage, subPath, prev), part[T](register.Storage, subPath, next))
		},
	}
	m.subscribe(ctx, sub)
	return nil
}

func (c *Manager) hasWatchers() bool {
	for _, d := range c.drivers {
		if _, ok := d.(Watcher); ok {
			return true
		}
	}
	return false
}

// subscribe adds the subscriber and starts watching for changes if it is the first one,
// the subscriber is removed when ctx is done, and watching is stopped after the last one.
func (c *Manager) subscribe(ctx context.Context, sub *subscriber) {
	c.subMu.Lock()
	c.subscribers = append(c.subscribers, sub)
	if c.stopWatching == nil {
		var watchCtx context.Context
		watchCtx, c.stopWatching = context.WithCancel(context.Background())
		go c.watch(watchCtx)
	}
	c.subMu.Unlock()

	go func() {
		<-ctx.Done()
		c.subMu.Lock()
		defer c.subMu.Unlock()
		for i, s := range c.subscribers {
			if s == sub {
				c.subscribers = append(c.subscribers[:i:i], c.subscribers[i+1:]...)
				break
			}
		}
		if len(c.subscribers) == 0 {
			c.stopWatching()
			c.stopWatching = nil
		}
	}()
}

// watch runs watching of all drivers that implement Watcher until ctx is done and reloads configs after the changes.
func (c *Manager) watch(ctx context.Context) {
	changes := make(chan struct{}, 1)
	notify := func() {
		select {
		case changes <- struct{}{}:
		default:
			// change is already waiting for reloading
		}
	}
	var wg sync.WaitGroup
	defer wg.Wait()
	for _, d := range c.drivers {
		watcher, ok := d.(Watcher)
		if !ok {
			continue
		}
		wg.Add(1)
		go func(d Driver, watcher Watcher) {
			defer wg.Done()
			// the driver can not watch anymore, other drivers continue
			if err := watcher.Watch(ctx, notify); err != nil && ctx.Err() == nil {
				c.log.Error(errorMessage(err, "failed to watch"), LogField("driver", d.GetName()), LogField("details", err.Error()))
			}
		}(d, watcher)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
			c.reload(ctx)
		}
	}
}

// reload parses again the registered configs with subscribers and notifies the subscribers about the changes.
func (c *Manager) reload(ctx context.Context) {
	// the stopped watching and the new one do not reload at the same time
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	c.subMu.Lock()
	subscribers := append([]*subscriber{}, c.subscribers...)
	c.subMu.Unlock()

	reloaded := make(map[reflect.Type]bool)
	for _, sub := range subscribers {
		if reloaded[sub.root] {
			continue
		}
		reloaded[sub.root] = true
		c.reloadConfig(ctx, sub.root, subscribers)
	}
}

// reloadConfig parses again the registered config and notifies its subscribers about the changes.
func (c *Manager) reloadConfig(ctx context.Context, typeOf reflect.Type, subscribers []*subscriber) {
	c.mu.RLock()
	register := c.registered[typeOf]
	c.mu.RUnlock()
	state := c.state(typeOf)
	c.mu.RLock()
	initial := state.initial
	c.mu.RUnlock()
	if initial == nil {
		// the config is not parsed yet, its values are the initial values
		initial = copyConf(register.Config)
	}
	fresh := copyConf(initial)
	// the requests of the reload are stopped when watching is stopped
	sources, err := c.parse(ctx, fresh, true)
	if err != nil {
		c.log.Error(errorMessage(err, "failed to reload"), LogField("config", typeOf.String()), LogField("details", err.Error()))
		return
	}
	prev := state.current.Load()
	if prev == nil {
		prev = initial
	}
	c.logChanges(register.Storage, prev, fresh, "", sources)
	state.current.Store(fresh)

	for _, sub := range subscribers {
		// the subscription can be canceled during the reload
		if sub.root == typeOf && sub.ctx.Err() == nil && changed(register.Storage, sub.prefix, prev, fresh) {
			c.callSubscriber(sub, prev, fresh)
		}
	}
}

// callSubscriber calls the handler of the subscriber, the panic of the handler is logged.
func (c *Manager) callSubscriber(sub *subscriber, prev, next any) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error("subscriber panicked", LogField("config", sub.root.String()), LogField("details", fmt.Sprint(r)))
		}
	}()
	sub.notify(prev, next)
}

// changed reports whether some fields with the prefix have different values in prev and next configs.
func changed(storage fmap.Storage, prefix string, prev, next any) bool {
	for _, path := range valuePaths(storage, prefix) {
		field := storage.MustFind(path)
		if !reflect.DeepEqual(convert.GetFieldValue(field, prev), convert.GetFieldValue(field, next)) {
			return true
		}
	}
	return false
}
