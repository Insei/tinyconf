package yaml

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/insei/tinyconf"

	"github.com/insei/cast"
	"github.com/insei/fmap/v2"
)

type yamlDriver struct {
	name string
	storage
}

func getMapValue(field fmap.Field, yamlMap any) (any, error) {
	yamlPathKey := field.GetTagPath("yaml", true)
	if yamlPathKey == "" {
		return nil, fmt.Errorf("%w: 'yaml' tag is not set", tinyconf.ErrIncorrectTagSettings)
	}
	yamlPathKeyArr := strings.Split(yamlPathKey, ".")
	if len(yamlPathKeyArr) > 1 {
		for i := 0; i < len(yamlPathKeyArr)-1; i++ {
			mCast := yamlMap.(map[string]interface{})
			if newM, ok := mCast[yamlPathKeyArr[i]]; ok {
				if mapCasted, ok := newM.(map[string]interface{}); ok {
					yamlMap = mapCasted
				}
			}
		}
	}
	mMap := yamlMap.(map[string]interface{})
	val, ok := mMap[yamlPathKeyArr[len(yamlPathKeyArr)-1]]
	if !ok || val == nil {
		return nil, fmt.Errorf("%w: value not found in yaml config", tinyconf.ErrValueNotFound)
	}
	return val, nil
}

func convertValToType(field fmap.Field, val any) (any, error) {
	valOf := reflect.ValueOf(val)
	fieldType := field.GetType()
	if valOf.Type() == fieldType {
		return val, nil
	}
	val, err := cast.ToReflect(fmt.Sprintf("%v", val), fieldType)
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (d *yamlDriver) GetValue(field fmap.Field) (*tinyconf.Value, error) {
	yamlMap, err := d.load()
	if err != nil {
		return nil, err
	}
	val, err := getMapValue(field, yamlMap)
	if err != nil {
		return nil, err
	}
	val, err = convertValToType(field, val)
	if err != nil {
		return nil, fmt.Errorf("failed to convert yaml map value to field type value: %w", err)
	}
	return &tinyconf.Value{
		Source: d.name,
		Value:  val,
	}, nil
}

func (d *yamlDriver) GetName() string {
	return d.name
}

func New(file string) (tinyconf.Driver, error) {
	return &yamlDriver{
		name:    "yaml",
		storage: &storageImpl{filePath: file},
	}, nil
}
