package yaml

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/insei/cast"
	"github.com/insei/fmap/v3"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/cmp118"
	"github.com/insei/tinyconf/slices118"
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
	path  string
	value any
	tag   reflect.StructTag
}

func (f field) genDoc(driver string, depth int) string {
	var offset strings.Builder
	for i := 0; i < depth; i++ {
		offset.WriteRune('\t')
	}
	offset.WriteRune('#')
	tagDriver := offset.String() + f.tag.Get(driver)
	tagDoc := offset.String() + f.tag.Get("doc")
	offset.Reset()
	if reflect.TypeOf(f.value).Kind() == reflect.Struct {
		f.value = ""
	}
	return fmt.Sprintf("%s\n%s: %v\n", tagDoc, tagDriver, f.value)
}

func (d *yamlDriver) getUniqueFields(registers []*tinyconf.Registered) []field {
	var fields []field
	for _, register := range registers {
		for _, path := range register.Storage.GetAllPaths() {
			fld := register.Storage.MustFind(path)

			tag := fld.GetTag()
			tagDriver, ok := tag.Lookup(d.name)
			if !ok {
				continue
			}

			member := field{
				path:  fld.GetTagPath(d.name, false),
				value: fld.Get(register.Config),
				tag:   tag,
			}

			if slices118.ContainsFunc(fields, func(item field) bool {
				matchPath := item.path == member.path
				matchTagDriver := tagDriver == item.tag.Get(d.name)
				return matchPath && matchTagDriver
			}) {
				continue
			}
			fields = append(fields, member)
		}
	}
	return fields
}

func (d *yamlDriver) getRootMap(fields []field) map[string]string {
	roots := map[string]string{}
	var root struct {
		path, tag string
	}
	for _, field := range fields {
		depth := strings.Count(field.path, ".")
		if depth == 0 {
			root = struct{ path, tag string }{path: field.path, tag: field.tag.Get(d.name)}
			roots[root.tag] = ""
			continue
		}
		if strings.HasPrefix(field.path, root.path) {
			roots[root.tag] += field.genDoc(d.name, depth)
		}
	}
	return roots
}

func (d *yamlDriver) GenDoc(registers ...*tinyconf.Registered) string {
	uniqueFields := d.getUniqueFields(registers)

	sortedFields := slices118.Clone(uniqueFields)
	slices118.SortStableFunc(sortedFields, func(i, j field) int {
		return cmp118.Compare(i.path, j.path)
	})

	roots := d.getRootMap(sortedFields)

	var doc string
	for _, field := range uniqueFields {
		depth := strings.Count(field.path, ".")
		if depth != 0 {
			continue
		}

		tagRootDriver := field.tag.Get(d.name)
		if nestedDoc, ok := roots[tagRootDriver]; ok {
			rootFieldDoc := field.genDoc(d.name, 0)
			doc += rootFieldDoc + nestedDoc
		}
	}

	return doc
}

func New(file string) (tinyconf.Driver, error) {
	return &yamlDriver{
		name:    "yaml",
		storage: &storageImpl{filePath: file},
	}, nil
}
