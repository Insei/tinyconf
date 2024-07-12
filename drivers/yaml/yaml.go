package yaml

import (
	"cmp"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/insei/cast"
	"github.com/insei/fmap/v3"
	"github.com/insei/tinyconf"
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

type field struct {
	path string
	tag  reflect.StructTag
}

func (d *yamlDriver) getUniqueFields(storages []fmap.Storage) []field {
	var fields []field
	for _, storage := range storages {
		if storage == nil {
			continue
		}
		for _, path := range storage.GetAllPaths() {
			member := field{path: path, tag: storage.MustFind(path).GetTag()}
			if slices.Contains(fields, member) {
				continue
			}
			fields = append(fields, member)
		}
	}
	return fields
}

func (d *yamlDriver) getRootMap(fields []field) map[string]string {
	roots := map[string]string{}
	var offset strings.Builder
	var keyPath, keyTag string

	for _, field := range fields {
		depth := strings.Count(field.path, ".")
		if depth == 0 {
			if tagValue, ok := field.tag.Lookup(d.name); ok {
				keyPath, keyTag = field.path, tagValue
				roots[keyTag] = ""
			}
			continue
		}

		if strings.HasPrefix(field.path, keyPath) {
			var path string
			for i := 0; i < depth; i++ {
				offset.WriteRune('\t')
			}
			if tagValue, ok := field.tag.Lookup(d.name); ok {
				remark := offset.String() + "#" + field.tag.Get("doc")
				tagValue = offset.String() + tagValue
				path = fmt.Sprintf("%s\n%s:\n", remark, tagValue)
				roots[keyTag] += path
			}
			offset.Reset()
		}
	}

	return roots
}

func (d *yamlDriver) Doc(storages ...fmap.Storage) string {
	fields := d.getUniqueFields(storages)

	sortedFields := slices.Clone(fields)
	slices.SortStableFunc(sortedFields, func(i, j field) int {
		return cmp.Compare(i.path, j.path)
	})

	roots := d.getRootMap(sortedFields)

	var doc string
	for _, field := range fields {
		depth := strings.Count(field.path, ".")
		if depth != 0 {
			continue
		}

		tagValue, ok := field.tag.Lookup(d.name)
		if !ok {
			continue
		}

		if v, ok := roots[tagValue]; ok {
			doc += fmt.Sprintf("#%s\n%s:\n%s", field.tag.Get("doc"), tagValue, v)
		}
	}

	fmt.Println(doc)
	return doc
}

func New(file string) (tinyconf.Driver, error) {
	return &yamlDriver{
		name:    "yaml",
		storage: &storageImpl{filePath: file},
	}, nil
}
