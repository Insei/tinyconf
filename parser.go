package tinyconf

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/insei/fmap/v3"
)

type Registered struct {
	Storage fmap.Storage
	Config  any
}

type Manager struct {
	drivers    []Driver
	log        Logger
	registered map[reflect.Type]*Registered
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

	storage, err := fmap.GetFrom(conf)
	if err != nil || storage == nil {
		return fmt.Errorf("config can't be registred: %w", err)
	}
	c.registered[reflect.TypeOf(conf)] = &Registered{
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
	confFields, err := fmap.GetFrom(conf)
	if err != nil {
		return err
	}
	subConfFields, err := fmap.GetFrom(subConf)
	if err != nil {
		return err
	}
	for _, path := range confFields.GetAllPaths() {
		// exclude paths that is not parsed by tinyconf drivers
		skip := true
		for _, parsedPath := range parsedPaths {
			if path == parsedPath {
				skip = false
				break
			}
		}
		if skip || path == subpath || !strings.HasPrefix(path, subpath) {
			continue
		}
		field := confFields.MustFind(path)
		subFieldPath := strings.Replace(path, subpath+".", "", -1)
		subField, ok := subConfFields.Find(subFieldPath)
		if !ok {
			return fmt.Errorf("subconf field %s not found", path)
		}
		subField.Set(subConf, field.Get(conf))
	}
	return nil
}

func (c *Manager) Parse(conf any) (err error) {
	confParse := conf
	confTypeOf := reflect.TypeOf(conf)
	register, ok := c.registered[confTypeOf]
	parsedPaths := make([]string, 0)
	if !ok {
	RegisteredLoop:
		for registeredTypeOf, registeredConf := range c.registered {
			for _, path := range registeredConf.Storage.GetAllPaths() {
				field := registeredConf.Storage.MustFind(path)
				fieldType := field.GetDereferencedType()
				if fieldType.Kind() == reflect.Struct &&
					reflect.PointerTo(fieldType) == confTypeOf {
					register = registeredConf
					confParse = reflect.New(registeredTypeOf.Elem()).Interface()
					defer func() {
						err = copyToSubConfig(confParse, conf, field.GetStructPath(), parsedPaths)
					}()
					break RegisteredLoop
				}
			}
		}
	}
	if register == nil {
		return ErrNotRegisteredConfig
	}
	for _, d := range c.drivers {
		for _, path := range register.Storage.GetAllPaths() {
			field := register.Storage.MustFind(path)
			if field.GetType().Kind() == reflect.Struct {
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
			case err != nil &&
				!errors.Is(err, ErrValueNotFound) &&
				!errors.Is(err, ErrIncorrectTagSettings):
				c.log.Error("failed", LogField("details", err.Error()))
			case err == nil:
				currentValue := field.Get(confParse)
				if currentValue != driverValue.Value {
					log.Debug("override", LogField("value", getLoggerValue(field, driverValue.Value)))
					field.Set(confParse, driverValue.Value)
					// only for sub configs
					parsedPaths = append(parsedPaths, path)
				}
			}
		}
	}
	return nil
}

func (c *Manager) GenDoc(driverName string) string {
	var registers []*Registered
	for _, register := range c.registered {
		registers = append(registers, register)
	}

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
