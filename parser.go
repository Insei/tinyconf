package tinyconf

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/insei/fmap/v3"

	"github.com/insei/tinyconf/convert"
)

type Registered struct {
	Storage fmap.Storage
	Config  any
}

type Manager struct {
	drivers    []Driver
	log        Logger
	registered map[reflect.Type]*Registered

	mu sync.RWMutex
	// types of the registered configs in the registration order
	order []reflect.Type
	// sources of the values from the last Parse call for every config type
	sources map[reflect.Type][]FieldSource
}

func getStorage(conf any) (fmap.Storage, error) {
	return fmap.GetFrom(conf)
}

func checkConfig(conf any) error {
	valOf := reflect.ValueOf(conf)
	if !valOf.IsValid() {
		return errors.New("config is not a correct value")
	}
	if !valOf.IsValid() || valOf.Kind() != reflect.Ptr {
		return errors.New("config can be only valid pointer")
	}
	valOf = valOf.Elem()
	if !valOf.IsValid() || valOf.Kind() != reflect.Struct {
		return errors.New("config must be a struct pointer")
	}
	return nil
}

func (c *Manager) Register(conf any) error {
	err := checkConfig(conf)
	if err != nil {
		return fmt.Errorf("config can't be registred: %w", err)
	}

	storage, err := getStorage(conf)
	if err != nil || storage == nil {
		return fmt.Errorf("config can't be registred: %w", err)
	}
	typeOf := reflect.TypeOf(conf)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.registered[typeOf]; !ok {
		c.order = append(c.order, typeOf)
	}
	c.registered[typeOf] = &Registered{
		Storage: storage,
		Config:  conf,
	}
	return nil
}

func getDereferencedValue(val any) any {
	valOf := reflect.ValueOf(val)
	for valOf.IsValid() && valOf.Kind() == reflect.Ptr {
		valOf = valOf.Elem()
	}
	if valOf.IsValid() && valOf.CanInterface() {
		return valOf.Interface()
	}
	return val
}

func getLoggerValue(field fmap.Field, val any) string {
	derefDriverValue := getDereferencedValue(val)
	valueLog := fmt.Sprintf("%v", derefDriverValue)
	if hidden, ok := field.GetTag().Lookup("hidden"); ok && hidden == "true" {
		valueLog = strings.Repeat("*", len(fmt.Sprintf("%s", valueLog)))
	}
	return valueLog
}

func copyToSubConfig(conf, subConf any, subpath string, parsedPaths []string) error {
	confFields, err := getStorage(conf)
	if err != nil {
		return err
	}
	subConfFields, err := getStorage(subConf)
	if err != nil {
		return err
	}
	prefix := subpath + "."
	for _, path := range confFields.GetAllPaths() {
		// exclude paths that is not parsed by tinyconf drivers
		skip := true
		for _, parsedPath := range parsedPaths {
			if path == parsedPath {
				skip = false
				break
			}
		}
		if skip || !strings.HasPrefix(path, prefix) {
			continue
		}
		field := confFields.MustFind(path)
		subField, ok := subConfFields.Find(strings.TrimPrefix(path, prefix))
		if !ok {
			return fmt.Errorf("subconf field %s not found", path)
		}
		convert.SetFieldValue(subField, subConf, convert.GetFieldValue(field, conf))
	}
	return nil
}

// getRegisters returns all registered configs in the registration order.
func (c *Manager) getRegisters() []*Registered {
	c.mu.RLock()
	defer c.mu.RUnlock()
	registers := make([]*Registered, 0, len(c.registered))
	for _, typeOf := range c.order {
		registers = append(registers, c.registered[typeOf])
	}
	return registers
}

// findRegister returns registered config for the config type.
// If the type is not registered, but it is a type of the field in some registered config (sub config),
// it returns this registered config and the struct path of the field.
// Registered configs are checked in the registration order, the first found field is used.
func (c *Manager) findRegister(confTypeOf reflect.Type) (register *Registered, registerTypeOf reflect.Type, subPath string) {
	c.mu.RLock()
	register, ok := c.registered[confTypeOf]
	c.mu.RUnlock()
	if ok {
		return register, confTypeOf, ""
	}
	for _, registered := range c.getRegisters() {
		for _, path := range registered.Storage.GetAllPaths() {
			field := registered.Storage.MustFind(path)
			// fields of the values parsed as a whole are not parsed by drivers, so they can not be a sub config
			if convert.IsInsideValue(registered.Storage, field) {
				continue
			}
			fieldType := field.GetDereferencedType()
			if fieldType.Kind() == reflect.Struct && reflect.PointerTo(fieldType) == confTypeOf {
				return registered, reflect.TypeOf(registered.Config), path
			}
		}
	}
	return nil, nil, ""
}

// Parse fills the config with values from the drivers, the next driver overrides values of the previous.
// Config should be registered, or it should be a field of the registered config (sub config).
//
// All fields are parsed even if some of them has errors, all errors are returned as one error,
// it can be checked with errors.Is and errors.As.
// Fields with required:"true" tag should have a value from any driver or non-zero value in the config,
// otherwise ErrRequired is returned.
func (c *Manager) Parse(conf any) error {
	if err := checkConfig(conf); err != nil {
		return fmt.Errorf("config can't be parsed: %w", err)
	}
	confParse := conf
	confTypeOf := reflect.TypeOf(conf)
	register, registerTypeOf, subPath := c.findRegister(confTypeOf)
	if register == nil {
		return ErrNotRegisteredConfig
	}
	// only for sub configs
	isSubConf := subPath != ""
	prefix := ""
	if isSubConf {
		confParse = reflect.New(registerTypeOf.Elem()).Interface()
		prefix = subPath + "."
	}
	sources := make(map[string]FieldSource)

	var errs parseErrors
	drivers := make([]Driver, 0, len(c.drivers))
	for _, d := range c.drivers {
		if loader, ok := d.(Loader); ok {
			if err := loader.Load(); err != nil {
				c.log.Error("failed to load", LogField("driver", d.GetName()), LogField("details", err.Error()))
				errs = append(errs, fmt.Errorf("%s driver: %w", d.GetName(), err))
				continue
			}
		}
		drivers = append(drivers, d)
	}
	for _, d := range drivers {
		for _, path := range register.Storage.GetAllPaths() {
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			field := register.Storage.MustFind(path)
			if !convert.IsValueField(register.Storage, field) {
				continue
			}
			log := c.log.With(
				LogField("config", confTypeOf.String()),
				LogField("driver", d.GetName()),
				LogField("field", path))
			driverValue, err := d.GetValue(field)
			switch {
			case errors.Is(err, ErrIncorrectTagSettings):
				log.Warn("ignore", LogField("details", err.Error()))
			case errors.Is(err, ErrValueNotFound):
				log.Debug("skip", LogField("details", err.Error()))
			case err != nil:
				log.Error("failed", LogField("details", err.Error()))
				errs = append(errs, fmt.Errorf("%s driver: field %s: %w", d.GetName(), path, err))
			default:
				if !reflect.DeepEqual(convert.GetFieldValue(field, confParse), driverValue.Value) {
					log.Debug("override", LogField("value", getLoggerValue(field, driverValue.Value)))
					convert.SetFieldValue(field, confParse, driverValue.Value)
				}
				sources[path] = FieldSource{
					Field:  strings.TrimPrefix(path, prefix),
					Driver: d.GetName(),
					Source: driverValue.Source,
				}
			}
		}
	}
	// sources in the order of the config fields
	fieldSources := make([]FieldSource, 0, len(sources))
	parsedPaths := make([]string, 0, len(sources))
	for _, path := range register.Storage.GetAllPaths() {
		if source, ok := sources[path]; ok {
			fieldSources = append(fieldSources, source)
			parsedPaths = append(parsedPaths, path)
		}
	}
	if isSubConf {
		if err := copyToSubConfig(confParse, conf, subPath, parsedPaths); err != nil {
			errs = append(errs, err)
		}
	}
	errs = append(errs, checkRequired(conf, sources, prefix)...)

	c.mu.Lock()
	if c.sources == nil {
		c.sources = make(map[reflect.Type][]FieldSource)
	}
	c.sources[confTypeOf] = fieldSources
	c.mu.Unlock()

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// checkRequired checks that all fields with required:"true" tag has a value from drivers or non-zero value in the config.
func checkRequired(conf any, sources map[string]FieldSource, prefix string) []error {
	storage, err := getStorage(conf)
	if err != nil {
		return []error{err}
	}
	var errs []error
	for _, path := range storage.GetAllPaths() {
		field := storage.MustFind(path)
		if field.GetTag().Get("required") != "true" || !convert.IsValueField(storage, field) {
			continue
		}
		if _, ok := sources[prefix+path]; ok {
			continue
		}
		valOf := reflect.ValueOf(convert.GetFieldValue(field, conf))
		isEmpty := !valOf.IsValid() || valOf.IsZero() ||
			((valOf.Kind() == reflect.Slice || valOf.Kind() == reflect.Map) && valOf.Len() == 0)
		if isEmpty {
			errs = append(errs, fmt.Errorf("field %s: %w", path, ErrRequired))
		}
	}
	return errs
}

// Sources returns sources of the config values from the last Parse call for this config type:
// which driver has set the value of the field. Fields without values from drivers are not included.
// Config should be the same pointer type that was used in Parse.
func (c *Manager) Sources(conf any) []FieldSource {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sources := c.sources[reflect.TypeOf(conf)]
	if sources == nil {
		return nil
	}
	return append([]FieldSource{}, sources...)
}

// Watch watches for changes of the config sources and blocks until ctx is done.
// After every change reported by drivers it parses the config to the new copy of conf
// and calls onChange with the pointer to this copy and the error of parsing.
// conf itself is never changed by Watch, its values at the moment of the call are used as defaults for every copy,
// so the current config can be read concurrently and replaced with the new one in onChange, with atomic.Pointer for example.
//
// Config should be registered, or it should be a field of the registered config (sub config).
// Changes are reported only by drivers that implements Watcher, ErrNoWatchers is returned if there are no such drivers.
func (c *Manager) Watch(ctx context.Context, conf any, onChange func(conf any, err error)) error {
	if err := checkConfig(conf); err != nil {
		return fmt.Errorf("config can't be watched: %w", err)
	}
	if register, _, _ := c.findRegister(reflect.TypeOf(conf)); register == nil {
		return ErrNotRegisteredConfig
	}
	var watchers []Driver
	for _, d := range c.drivers {
		if _, ok := d.(Watcher); ok {
			watchers = append(watchers, d)
		}
	}
	if len(watchers) == 0 {
		return ErrNoWatchers
	}
	// defaults for all next configs
	// structs stored by pointer are copied, their fields are set by Parse
	copyConf := func(src reflect.Value) reflect.Value {
		dst := reflect.New(src.Type().Elem())
		dst.Elem().Set(src.Elem())
		convert.CopySections(dst.Interface())
		return dst
	}
	base := copyConf(reflect.ValueOf(conf))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	changes := make(chan struct{}, 1)
	notify := func() {
		select {
		case changes <- struct{}{}:
		default:
			// change is already waiting for parsing
		}
	}
	errs := make(chan error, len(watchers))
	var wg sync.WaitGroup
	for _, d := range watchers {
		wg.Add(1)
		go func(d Driver) {
			defer wg.Done()
			if err := d.(Watcher).Watch(ctx, notify); err != nil && ctx.Err() == nil {
				errs <- fmt.Errorf("%s driver: %w", d.GetName(), err)
			}
		}(d)
	}
	defer wg.Wait()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errs:
			cancel()
			return err
		case <-changes:
			fresh := copyConf(base)
			err := c.Parse(fresh.Interface())
			onChange(fresh.Interface(), err)
		}
	}
}

func (c *Manager) GenDoc(driverName string) string {
	registers := c.getRegisters()

	var doc string
	for _, driver := range c.drivers {
		if driverName == driver.GetName() {
			doc = driver.GenDoc(registers...)
		}
	}

	return doc
}

func New(opts ...Option) (*Manager, error) {
	m := &Manager{log: &noopLogger{}, registered: map[reflect.Type]*Registered{}}
	count := countDrivers(opts...)
	if count > 0 {
		m.drivers = make([]Driver, 0, count)
	}
	for i, _ := range opts {
		//switch case for minimize heap allocations
		switch opt := opts[i].(type) {
		case driverOption:
			opt.apply(m)
		case loggerOption:
			opt.apply(m)
		}
	}
	return m, nil
}
