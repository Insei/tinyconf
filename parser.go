package tinyconf

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/insei/fmap/v3"
)

type Manager struct {
	drivers    []Driver
	log        Logger
	registered map[reflect.Type]fmap.Storage
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
	c.registered[reflect.TypeOf(conf)], err = fmap.GetFrom(conf)
	if err != nil {
		return fmt.Errorf("config can't be registred: %w", err)
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

func (c *Manager) Parse(conf any) error {
	confTypeOf := reflect.TypeOf(conf)
	storage, ok := c.registered[confTypeOf]
	if !ok {
		return ErrNotRegisteredConfig
	}
	for _, d := range c.drivers {
		for _, path := range storage.GetAllPaths() {
			field := storage.MustFind(path)
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
				currentValue := field.Get(conf)
				if currentValue != driverValue.Value {
					log.Debug("override", LogField("value", getLoggerValue(field, driverValue.Value)))
					field.Set(conf, driverValue.Value)
				}
			}
		}
	}
	return nil
}

func New(opts ...Option) (*Manager, error) {
	m := &Manager{log: &noopLogger{}, registered: map[reflect.Type]fmap.Storage{}}
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
