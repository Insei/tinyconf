package tag

import (
	"fmt"

	"github.com/insei/cast"
	"github.com/insei/fmap/v3"
	"github.com/insei/tinyconf"
)

type defaultTagDriver struct {
	tag  string
	name string
}

func (d defaultTagDriver) GetValue(field fmap.Field) (*tinyconf.Value, error) {
	valueStr, ok := field.GetTag().Lookup(d.tag)
	if !ok {
		return nil, fmt.Errorf("%w: %s tag is not set for %s config field", tinyconf.ErrIncorrectTagSettings, d.tag, field.GetStructPath())
	}
	if valueStr == "" {
		return nil, fmt.Errorf("%w: %s tag is set, but has empty value for %s config field", tinyconf.ErrIncorrectTagSettings, d.tag, field.GetStructPath())
	}
	value, err := cast.ToReflect(valueStr, field.GetType())
	if err != nil {
		return nil, fmt.Errorf("failed to parse value from tag: %s", err)
	}
	return &tinyconf.Value{Source: d.tag, Value: value}, err
}

func (d defaultTagDriver) GetName() string {
	return d.name
}

func (d defaultTagDriver) GenDoc(registers ...tinyconf.Registered) string {
	return ""
}

func New(tagName string) (tinyconf.Driver, error) {
	return defaultTagDriver{
		tag:  tagName,
		name: "tag",
	}, nil
}
