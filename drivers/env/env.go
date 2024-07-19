package env

import (
	"cmp"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/insei/cast"
	"github.com/insei/fmap/v3"
	"github.com/insei/tinyconf"
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

func (f field) genDoc(driver string) string {
	if tagDriver, ok := f.tag.Lookup(driver); ok {
		tagDoc := f.tag.Get("doc")
		return fmt.Sprintf("#%s\n#%s=\n", tagDoc, tagDriver)
	}
	return ""
}

func (d envDriver) getUniqueFields(storages []fmap.Storage) []field {
	var fields []field
	for _, storage := range storages {
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
	var rootPath string
	for _, field := range fields {
		docField := field.genDoc(d.name)
		depth := strings.Count(field.path, ".")
		if depth == 0 {
			rootPath = field.path
			roots[rootPath] = docField
			continue
		}
		if strings.HasPrefix(field.path, rootPath) {
			if strings.Contains(roots[rootPath], field.tag.Get(d.name)) {
				continue
			}
			roots[rootPath] += docField
		}
	}
	return roots
}

func (d envDriver) GenDoc(storages ...fmap.Storage) string {
	uniqueFields := d.getUniqueFields(storages)

	sortedFields := slices.Clone(uniqueFields)
	slices.SortStableFunc(sortedFields, func(i, j field) int {
		return cmp.Compare(i.path, j.path)
	})

	roots := d.getRootMap(sortedFields)

	var doc string
	for _, field := range uniqueFields {
		depth := strings.Count(field.path, ".")
		if depth != 0 {
			continue
		}

		doc += roots[field.path] + "\n"
	}

	return doc
}

func New() (tinyconf.Driver, error) {
	return envDriver{
		name: "env",
	}, nil
}
