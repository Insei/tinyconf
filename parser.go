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
		subField.Set(subConf, field.Get(conf))
	}
	return nil
}

func (c *Manager) Parse(conf any) error {
	if err := checkConfig(conf); err != nil {
		return fmt.Errorf("config can't be parsed: %w", err)
	}
	confParse := conf
	confTypeOf := reflect.TypeOf(conf)
	register, ok := c.registered[confTypeOf]
	// only for sub configs
	isSubConf := false
	subPath := ""
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
					isSubConf = true
					subPath = field.GetStructPath()
					break RegisteredLoop
				}
			}
		}
	}
	if register == nil {
		return ErrNotRegisteredConfig
	}
	var errs parseErrors
	for _, d := range c.drivers {
		for _, path := range register.Storage.GetAllPaths() {
			if isSubConf && !strings.HasPrefix(path, subPath+".") {
				continue
			}
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
			case err != nil:
				log.Error("failed", LogField("details", err.Error()))
				errs = append(errs, fmt.Errorf("%s driver: field %s: %w", d.GetName(), path, err))
			default:
				if !reflect.DeepEqual(field.Get(confParse), driverValue.Value) {
					log.Debug("override", LogField("value", getLoggerValue(field, driverValue.Value)))
					field.Set(confParse, driverValue.Value)
				}
				parsedPaths = append(parsedPaths, path)
			}
		}
	}
	if isSubConf {
		if err := copyToSubConfig(confParse, conf, subPath, parsedPaths); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs
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
