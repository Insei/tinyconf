package env

import (
	"bufio"
	"cmp"
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/slices"

	"github.com/insei/cast"
	"github.com/insei/fmap/v3"
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
	path  string
	value any
	depth int
	tag   reflect.StructTag
}

func (f field) genDoc(driver string) string {
	if tagDriver, ok := f.tag.Lookup(driver); ok {
		tagDoc := f.tag.Get("doc")
		return fmt.Sprintf("#%s\n#%s=%v\n", tagDoc, tagDriver, f.value)
	}
	return ""
}

func (d envDriver) getUniqueFields(registers []*tinyconf.Registered) []field {
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
				path:  strings.Split(tagDriver, "_")[0],
				value: fld.Get(register.Config),
				depth: strings.Count(tagDriver, "_"),
				tag:   tag,
			}

			if slices.ContainsFunc(fields, func(item field) bool {
				return item.tag.Get(d.name) == member.tag.Get(d.name)
			}) {
				continue
			}
			fields = append(fields, member)
		}
	}
	return fields
}

func (d envDriver) getRootMap(fields []field) map[int]map[string]string {
	roots := make(map[int]map[string]string)
	root := make(map[string]string)

	for _, field := range fields {
		root[field.path] += field.genDoc(d.name)
		roots[field.depth] = root
	}
	return roots
}

func (d envDriver) GenDoc(registers ...*tinyconf.Registered) string {
	uniqueFields := d.getUniqueFields(registers)

	sortedFields := slices.Clone(uniqueFields)
	slices.SortStableFunc(sortedFields, func(i, j field) int {
		return cmp.Compare(j.depth, i.depth)
	})

	roots := d.getRootMap(sortedFields)
	marks := make([]string, 0)

	var doc string
	for _, field := range uniqueFields {
		if slices.Contains(marks, field.path) {
			continue
		}
		marks = append(marks, field.path)
		doc += roots[field.depth][field.path] + "\n"
	}
	return doc
}

func New() (tinyconf.Driver, error) {
	setENVsFromExecutable()
	setENVsFromWD()

	return envDriver{
		name: "env",
	}, nil
}

func setENVsFromExecutable() {
	execPath, _ := os.Executable()
	envPath := path.Dir(execPath)
	setENVsFromPath(envPath)
}

func setENVsFromWD() {
	wd, _ := os.Getwd()
	setENVsFromPath(wd)
}

func setENVsFromPath(envDirPath string) {
	envFile, err := os.Open(path.Join(envDirPath, ".env"))
	if err != nil {
		return
	}

	fileScanner := bufio.NewScanner(envFile)
	fileScanner.Split(bufio.ScanLines)
	var envLines []string

	for fileScanner.Scan() {
		envLine := fileScanner.Text()
		if envLine[:1] != "#" {
			envLines = append(envLines, fileScanner.Text())
		}
	}

	for _, envLine := range envLines {
		envRow := strings.Split(envLine, "=")
		envVal := ""
		if len(envRow) > 1 {
			envVal = envRow[1]
		}
		if _, exist := os.LookupEnv(strings.TrimSpace(envRow[0])); exist {
			continue
		}
		if err = os.Setenv(strings.TrimSpace(envRow[0]), envVal); err != nil {
			return
		}
	}
}
