package env

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"
	"unicode"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/cmp118"
	"github.com/insei/tinyconf/convert"
	"github.com/insei/tinyconf/slices118"

	"github.com/insei/fmap/v3"
)

// secretFileSuffix is a suffix of the env variable with path to the file with value.
const secretFileSuffix = "_FILE"

type envDriver struct {
	name        string
	prefix      string
	autoKeys    bool
	secretFiles bool
	// values from .env files, that are not exported to the process environment
	values map[string]string
}

// getKey returns name of the env variable for the field without prefix.
func (d envDriver) getKey(field fmap.Field) (string, bool) {
	envKey, ok := field.GetTag().Lookup(d.name)
	if ok && envKey != "" {
		return envKey, envKey != "-"
	}
	if !d.autoKeys {
		return "", false
	}
	return autoKey(field.GetStructPath()), true
}

// autoKey converts struct path to the env variable name: HTTP.MaxConns -> HTTP_MAX_CONNS.
func autoKey(structPath string) string {
	var key strings.Builder
	runes := []rune(structPath)
	for i, r := range runes {
		if r == '.' {
			key.WriteRune('_')
			continue
		}
		if i > 0 && unicode.IsUpper(r) && runes[i-1] != '.' {
			prevIsLower := unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1])
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if prevIsLower || (unicode.IsUpper(runes[i-1]) && nextIsLower) {
				key.WriteRune('_')
			}
		}
		key.WriteRune(unicode.ToUpper(r))
	}
	return key.String()
}

func (d envDriver) lookup(envKey string) (string, bool) {
	if envVal, ok := os.LookupEnv(envKey); ok {
		return envVal, true
	}
	envVal, ok := d.values[envKey]
	return envVal, ok
}

func (d envDriver) GetValue(field fmap.Field) (*tinyconf.Value, error) {
	envKey, ok := d.getKey(field)
	if !ok {
		return nil, fmt.Errorf("%w: env tag is not set for %s config field", tinyconf.ErrIncorrectTagSettings, field.GetStructPath())
	}
	envKey = d.prefix + envKey
	envVal, ok := d.lookup(envKey)
	if !ok && d.secretFiles {
		if filePath, fileOk := d.lookup(envKey + secretFileSuffix); fileOk {
			data, err := os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read file from key %s for %s config field: %w", envKey+secretFileSuffix, field.GetStructPath(), err)
			}
			envKey, envVal, ok = envKey+secretFileSuffix, strings.TrimRight(string(data), "\r\n"), true
		}
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s is not defined in env for %s config field", tinyconf.ErrValueNotFound, envKey, field.GetStructPath())
	}
	value, err := convert.StringToField(envVal, field)
	if err != nil {
		return nil, fmt.Errorf("failed to parse env value from key %s for %s config field: %w", envKey, field.GetStructPath(), err)
	}
	return &tinyconf.Value{Source: envKey, Value: value}, err
}

func (d envDriver) GetName() string {
	return d.name
}

type field struct {
	path  string
	key   string
	value string
	depth int
	tag   reflect.StructTag
}

func (f field) genDoc() string {
	return fmt.Sprintf("#%s\n#%s=%s\n", f.tag.Get("doc"), f.key, f.value)
}

func (d envDriver) getUniqueFields(registers []*tinyconf.Registered) []field {
	var fields []field
	for _, register := range registers {
		for _, path := range register.Storage.GetAllPaths() {
			fld := register.Storage.MustFind(path)

			envKey, ok := d.getKey(fld)
			if !ok || !convert.IsValueField(register.Storage, fld) {
				continue
			}

			tag := fld.GetTag()
			member := field{
				path:  strings.Split(envKey, "_")[0],
				key:   d.prefix + envKey,
				depth: strings.Count(envKey, "_"),
				tag:   tag,
			}
			if tag.Get("hidden") != "true" {
				member.value = convert.FieldToString(convert.GetFieldValue(fld, register.Config), fld)
			}

			if slices118.ContainsFunc(fields, func(item field) bool {
				return item.key == member.key
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
		root[field.path] += field.genDoc()
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

type options struct {
	prefix      string
	autoKeys    bool
	secretFiles bool
	isolation   bool
	files       []string
	customFiles bool
}

// Option is an env driver option.
type Option func(*options)

// WithPrefix adds prefix to the names of all env variables: with prefix APP_ field with env:"HTTP_PORT" tag
// is read from the APP_HTTP_PORT variable.
func WithPrefix(prefix string) Option {
	return func(o *options) {
		o.prefix = prefix
	}
}

// WithAutoKeys enables reading of the fields without env tag,
// name of the variable is built from the struct path of the field: HTTP.MaxConns -> HTTP_MAX_CONNS.
// Field can be excluded with env:"-" tag.
func WithAutoKeys() Option {
	return func(o *options) {
		o.autoKeys = true
	}
}

// WithSecretFiles enables reading of the values from files: if variable HTTP_TOKEN is not set,
// but HTTP_TOKEN_FILE is set, the value is read from the file with path from HTTP_TOKEN_FILE.
// It is useful for Docker and Kubernetes secrets.
func WithSecretFiles() Option {
	return func(o *options) {
		o.secretFiles = true
	}
}

// WithFiles sets .env files, that should be read instead of the default .env files
// from the executable directory and from the working directory.
// All files should exist. Value from the previous file has priority over the next files,
// values of the process environment has priority over all files.
// WithFiles without arguments disables reading of the .env files.
func WithFiles(files ...string) Option {
	return func(o *options) {
		o.files = append(o.files, files...)
		o.customFiles = true
	}
}

// WithFilesIsolation disables export of the values from the .env files to the process environment,
// the values are available only for the driver.
func WithFilesIsolation() Option {
	return func(o *options) {
		o.isolation = true
	}
}

// New creates new env driver.
// By default, it reads .env files from the executable directory and from the working directory
// and exports their values to the process environment.
func New(opts ...Option) (tinyconf.Driver, error) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	d := envDriver{
		name:        "env",
		prefix:      o.prefix,
		autoKeys:    o.autoKeys,
		secretFiles: o.secretFiles,
	}
	set := setENV
	if o.isolation {
		d.values = make(map[string]string)
		set = func(envKey, envVal string) {
			if _, exist := d.values[envKey]; !exist && envKey != "" {
				d.values[envKey] = envVal
			}
		}
	}
	if !o.customFiles {
		execPath, _ := os.Executable()
		wd, _ := os.Getwd()
		for _, dir := range []string{path.Dir(execPath), wd} {
			// default .env files are optional
			_ = readENVFile(path.Join(dir, ".env"), set)
		}
	}
	for _, file := range o.files {
		if err := readENVFile(file, set); err != nil {
			return nil, fmt.Errorf("failed to read env file %s: %w", file, err)
		}
	}
	return d, nil
}

// setENV exports value to the process environment if it is not already set.
func setENV(envKey, envVal string) {
	if _, exist := os.LookupEnv(envKey); exist {
		return
	}
	_ = os.Setenv(envKey, envVal)
}

func setENVsFromPath(envDirPath string) {
	_ = readENVFile(path.Join(envDirPath, ".env"), setENV)
}

// readENVFile reads .env file and calls set for every variable.
func readENVFile(filePath string, set func(envKey, envVal string)) error {
	envFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer envFile.Close()

	fileScanner := bufio.NewScanner(envFile)
	fileScanner.Split(bufio.ScanLines)

	for fileScanner.Scan() {
		envLine := strings.TrimSpace(fileScanner.Text())
		if envLine == "" || strings.HasPrefix(envLine, "#") {
			continue
		}
		envRow := strings.SplitN(envLine, "=", 2)
		envVal := ""
		if len(envRow) > 1 {
			envVal = unquote(strings.TrimSpace(envRow[1]))
		}
		set(strings.TrimSpace(envRow[0]), envVal)
	}
	return fileScanner.Err()
}

// unquote removes matching single or double quotes around the value.
func unquote(val string) string {
	if len(val) > 1 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
		return val[1 : len(val)-1]
	}
	return val
}
