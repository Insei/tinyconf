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

// getStorage returns fields of the config, it is a variable to check the storage errors in tests.
var getStorage = fmap.GetFrom

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

// copySubConfig copies values of the fields by paths between the config and its sub config.
// The sub config has the same type as the field of the config with the prefix, so all fields are found.
func copySubConfig(confFields fmap.Storage, conf any, subFields fmap.Storage, subConf any, prefix string, paths []string, toSub bool) {
	for _, path := range paths {
		field := confFields.MustFind(path)
		subField := subFields.MustFind(strings.TrimPrefix(path, prefix))
		if toSub {
			convert.SetFieldValue(subField, subConf, convert.GetFieldValue(field, conf))
		} else {
			convert.SetFieldValue(field, conf, convert.GetFieldValue(subField, subConf))
		}
	}
}

// valuePaths returns paths of the fields with prefix, whose values are parsed by drivers.
func valuePaths(storage fmap.Storage, prefix string) []string {
	var paths []string
	for _, path := range storage.GetAllPaths() {
		if strings.HasPrefix(path, prefix) && convert.IsValueField(storage, storage.MustFind(path)) {
			paths = append(paths, path)
		}
	}
	return paths
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
	_, err := c.parse(conf, false)
	return err
}

// parse parses the config and returns sources of the values.
// Debug messages about every field are logged only if reload is false, Watch logs only changed fields.
func (c *Manager) parse(conf any, reload bool) ([]FieldSource, error) {
	if err := checkConfig(conf); err != nil {
		return nil, fmt.Errorf("config can't be parsed: %w", err)
	}
	confParse := conf
	confTypeOf := reflect.TypeOf(conf)
	register, registerTypeOf, subPath := c.findRegister(confTypeOf)
	if register == nil {
		return nil, ErrNotRegisteredConfig
	}
	// only for sub configs
	isSubConf := subPath != ""
	prefix := ""
	if isSubConf {
		prefix = subPath + "."
	}
	paths := valuePaths(register.Storage, prefix)
	var subStorage fmap.Storage
	if isSubConf {
		var err error
		if subStorage, err = getStorage(conf); err != nil {
			return nil, fmt.Errorf("config can't be parsed: %w", err)
		}
		confParse = reflect.New(registerTypeOf.Elem()).Interface()
		// values of the sub config are initial values of the fields
		copySubConfig(register.Storage, confParse, subStorage, conf, prefix, paths, false)
	}
	if !reload {
		for _, path := range paths {
			field := register.Storage.MustFind(path)
			c.log.Debug("initial",
				LogField("config", confTypeOf.String()),
				LogField("field", path),
				LogField("value", getLoggerValue(field, convert.GetFieldValue(field, confParse))))
		}
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
		for _, path := range paths {
			field := register.Storage.MustFind(path)
			log := c.log.With(
				LogField("config", confTypeOf.String()),
				LogField("driver", d.GetName()),
				LogField("field", path))
			driverValue, err := d.GetValue(field)
			switch {
			case errors.Is(err, ErrIncorrectTagSettings):
				log.Warn("ignore", LogField("details", err.Error()))
			case errors.Is(err, ErrValueNotFound):
				if !reload {
					log.Debug("skip", LogField("details", err.Error()))
				}
			case err != nil:
				log.Error("failed", LogField("details", err.Error()))
				errs = append(errs, fmt.Errorf("%s driver: field %s: %w", d.GetName(), path, err))
			default:
				if driverValue.Warning != "" {
					log.Warn("warning", LogField("details", driverValue.Warning))
				}
				if !reflect.DeepEqual(convert.GetFieldValue(field, confParse), driverValue.Value) {
					if !reload {
						log.Debug("override",
							LogField("key", driverValue.Source),
							LogField("value", getLoggerValue(field, driverValue.Value)))
					}
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
	for _, path := range paths {
		if source, ok := sources[path]; ok {
			fieldSources = append(fieldSources, source)
			parsedPaths = append(parsedPaths, path)
		}
	}
	if isSubConf {
		copySubConfig(register.Storage, confParse, subStorage, conf, prefix, parsedPaths, true)
	}
	errs = append(errs, checkRequired(conf, sources, prefix)...)

	c.mu.Lock()
	if c.sources == nil {
		c.sources = make(map[reflect.Type][]FieldSource)
	}
	c.sources[confTypeOf] = fieldSources
	c.mu.Unlock()

	if len(errs) > 0 {
		return fieldSources, errs
	}
	return fieldSources, nil
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
// Only fields with the new values are logged on the debug level, relative to the last config parsed without errors.
//
// Config should be registered, or it should be a field of the registered config (sub config).
// Changes are reported only by drivers that implements Watcher, ErrNoWatchers is returned if there are no such drivers.
func (c *Manager) Watch(ctx context.Context, conf any, onChange func(conf any, err error)) error {
	if err := checkConfig(conf); err != nil {
		return fmt.Errorf("config can't be watched: %w", err)
	}
	register, _, subPath := c.findRegister(reflect.TypeOf(conf))
	if register == nil {
		return ErrNotRegisteredConfig
	}
	storage, err := getStorage(conf)
	if err != nil {
		return fmt.Errorf("config can't be watched: %w", err)
	}
	prefix := ""
	if subPath != "" {
		prefix = subPath + "."
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
	// the last config passed to onChange without errors, changes are logged relative to it
	prev := conf

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
			fresh := copyConf(base).Interface()
			sources, err := c.parse(fresh, true)
			if err == nil {
				c.logChanges(storage, prev, fresh, prefix, sources)
				prev = fresh
			}
			onChange(fresh, err)
		}
	}
}

// logChanges logs fields with different values in prev and next configs, next config is parsed with sources.
func (c *Manager) logChanges(storage fmap.Storage, prev, next any, prefix string, sources []FieldSource) {
	fieldSources := make(map[string]FieldSource, len(sources))
	for _, source := range sources {
		fieldSources[source.Field] = source
	}
	for _, path := range valuePaths(storage, "") {
		field := storage.MustFind(path)
		value := convert.GetFieldValue(field, next)
		if reflect.DeepEqual(convert.GetFieldValue(field, prev), value) {
			continue
		}
		fields := []Field{LogField("config", reflect.TypeOf(next).String())}
		source, ok := fieldSources[path]
		if ok {
			fields = append(fields, LogField("driver", source.Driver))
		}
		fields = append(fields, LogField("field", prefix+path))
		if ok {
			fields = append(fields, LogField("key", source.Source))
		}
		fields = append(fields, LogField("value", getLoggerValue(field, value)))
		c.log.Debug("override", fields...)
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
