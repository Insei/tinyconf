package tinyconf

import (
	"errors"
	"fmt"
	"github.com/insei/fmap/v2"
	"reflect"
	"strings"
)

type Manager struct {
	drivers    []Driver
	log        Logger
	registered map[reflect.Type]map[string]fmap.Field
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
	if err := checkConfig(conf); err != nil {
		return fmt.Errorf("config can't be registred: %w", err)
	}
	c.registered[reflect.TypeOf(conf)] = fmap.GetFrom(conf)
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
	fields, ok := c.registered[confTypeOf]
	if !ok {
		return ErrNotRegisteredConfig
	}
	for _, d := range c.drivers {
		for path, field := range fields {
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
	m := &Manager{log: &noopLogger{}, registered: map[reflect.Type]map[string]fmap.Field{}}
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
