package env

import (
	"fmt"
	"os"
	"tinyconf"

	"github.com/insei/cast"
	"github.com/insei/fmap/v2"
)

type envDriver struct {
	name string
}

func (d envDriver) GetValue(field fmap.Field) (*tinyconf.Value, error) {
	envKey, ok := field.GetTag().Lookup(d.name)
	if !ok || envKey == "" {
		return nil, fmt.Errorf("%w: env tag is not set for %s config field", tinyconf.ErrIncorrectTagSettings, field.GetStructPath())
	}
	envVal, ok := os.LookupEnv(envKey)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not defined in env for %s config field", tinyconf.ErrValueNotFound, envKey, field.GetStructPath())
	}
	value, err := cast.ToReflect(envVal, field.GetType())
	if err != nil {
		return nil, fmt.Errorf("failed to parse env value from key %s for %s config field: %s", envKey, field.GetStructPath(), err)
	}
	return &tinyconf.Value{Source: envKey, Value: value}, err
}

func (d envDriver) GetName() string {
	return d.name
}

func New() (tinyconf.Driver, error) {
	return envDriver{
		name: "env",
	}, nil
}
