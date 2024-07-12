package env

import (
	"cmp"
	"fmt"
	"github.com/insei/cast"
	"github.com/insei/fmap/v3"
	"github.com/insei/tinyconf"
	"os"
	"reflect"
	"slices"
	"strings"
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

type field struct {
	path string
	tag  reflect.StructTag
}

func (f field) getTag(tag string) string {
	if tagValue, ok := f.tag.Lookup(tag); ok {
		return fmt.Sprintf("#%s\n%s:\n", f.tag.Get("doc"), tagValue)
	}
	return ""
}

func (d envDriver) getUniqueFields(storages []fmap.Storage) []field {
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

func (d envDriver) getRootMap(fields []field) map[string]string {
	roots := map[string]string{}
	var keyPath string
	for _, field := range fields {
		depth := strings.Count(field.path, ".")
		if depth == 0 {
			keyPath = field.path
			roots[keyPath] = field.getTag(d.name)
			continue
		}

		if strings.HasPrefix(field.path, keyPath) {
			docTag := field.getTag(d.name)
			if strings.Contains(roots[keyPath], docTag) {
				continue
			}
			roots[keyPath] += docTag
		}
	}
	return roots
}

func (d envDriver) Doc(storages ...fmap.Storage) string {
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

		doc += roots[field.path]
	}

	fmt.Println(doc)
	return doc
}

func New() (tinyconf.Driver, error) {
	return envDriver{
		name: "env",
	}, nil
}
