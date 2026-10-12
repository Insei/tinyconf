package env

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/cmp118"
	"github.com/insei/tinyconf/slices118"

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
		if f.tag.Get("hidden") == "true" {
			f.value = ""
		}
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

			if slices118.ContainsFunc(fields, func(item field) bool {
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

	sortedFields := slices118.Clone(uniqueFields)
	slices118.SortStableFunc(sortedFields, func(i, j field) int {
		return cmp118.Compare(j.depth, i.depth)
	})

	roots := d.getRootMap(sortedFields)
	marks := make([]string, 0)

	var doc string
	for _, field := range uniqueFields {
		if slices118.Contains(marks, field.path) {
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
		envLine := strings.TrimSpace(fileScanner.Text())
		if envLine != "" && !strings.HasPrefix(envLine, "#") {
			envLines = append(envLines, envLine)
		}
	}

	for _, envLine := range envLines {
		envRow := strings.SplitN(envLine, "=", 2)
		envKey := strings.TrimSpace(envRow[0])
		envVal := ""
		if len(envRow) > 1 {
			envVal = unquote(strings.TrimSpace(envRow[1]))
		}
		if _, exist := os.LookupEnv(envKey); exist {
			continue
		}
		if err = os.Setenv(envKey, envVal); err != nil {
			return
		}
	}
}

// unquote removes matching single or double quotes around the value.
func unquote(val string) string {
	if len(val) > 1 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
		return val[1 : len(val)-1]
	}
	return val
}
