package env

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/insei/tinyconf/v2"
	"github.com/insei/tinyconf/v2/cmp118"
	"github.com/insei/tinyconf/v2/convert"
	"github.com/insei/tinyconf/v2/slices118"

	"github.com/insei/fmap/v3"
)

// secretFileSuffix is a suffix of the env variable with path to the file with value.
const secretFileSuffix = "_FILE"

type envDriver struct {
	name        string
	prefix      string
	autoKeys    bool
	secretFiles bool
	indexedKeys bool
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
	return autoKey(autoPath(field)), true
}

// autoPath returns the struct path of the field without the embedded structs:
// their fields are on the level of the embedding struct, like in encoding/json.
func autoPath(field fmap.Field) string {
	path := field.GetName()
	for parent := field.GetParent(); parent != nil; parent = parent.GetParent() {
		if !parent.GetAnonymous() {
			path = parent.GetName() + "." + path
		}
	}
	return path
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
	// values of simple types are not built from the separate variables, the environment is not scanned for them
	if !ok && d.indexedKeys && !convert.IsScalar(derefType(field.GetType())) {
		tree, found, err := indexedTree(field.GetType(), d.environ(envKey+"_"))
		if err != nil {
			return nil, fmt.Errorf("failed to parse env values from keys %s_* for %s config field: %w", envKey, field.GetStructPath(), err)
		}
		if found {
			value, err := convert.TreeToField(tree, field, d.name)
			if err != nil {
				return nil, fmt.Errorf("failed to parse env values from keys %s_* for %s config field: %w", envKey, field.GetStructPath(), err)
			}
			return &tinyconf.Value{Source: envKey + "_*", Value: value}, nil
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

// environ returns all env variables with the prefix, keys are the names of the variables without the prefix.
func (d envDriver) environ(prefix string) map[string]string {
	vars := make(map[string]string)
	add := func(envKey, envVal string) {
		if strings.HasPrefix(envKey, prefix) && len(envKey) > len(prefix) {
			vars[envKey[len(prefix):]] = envVal
		}
	}
	for envKey, envVal := range d.values {
		add(envKey, envVal)
	}
	// process environment has priority over .env files
	for _, env := range os.Environ() {
		if idx := strings.Index(env, "="); idx > 0 {
			add(env[:idx], env[idx+1:])
		}
	}
	return vars
}

// cutSegment splits the name of the variable to the first segment and the rest: 0_HOST -> 0, HOST.
func cutSegment(name string) (string, string) {
	if idx := strings.Index(name, "_"); idx >= 0 {
		return name[:idx], name[idx+1:]
	}
	return name, ""
}

// derefType returns the type without pointers.
func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

// indexedTree builds the value with type t from the separate variables:
// HOSTS_0, HOSTS_1 for slices and arrays, LABELS_key for maps, SERVER_HOST for structs
// and their combinations like SERVERS_0_HOST. Keys of vars are the names of the variables without the name of the field,
// empty key is used for the value of the whole type. Variables that do not match the type are ignored.
func indexedTree(t reflect.Type, vars map[string]string) (any, bool, error) {
	t = derefType(t)
	if val, ok := vars[""]; ok {
		return val, true, nil
	}
	if convert.IsScalar(t) {
		return nil, false, nil
	}
	// variables of the nested values by the first segment of the name
	groups := make(map[string]map[string]string)
	group := func(key, rest, val string) {
		if groups[key] == nil {
			groups[key] = make(map[string]string)
		}
		groups[key][rest] = val
	}
	items := make(map[string]any)
	build := func(elem reflect.Type) error {
		for key, vars := range groups {
			item, found, err := indexedTree(elem, vars)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			if found {
				items[key] = item
			}
		}
		return nil
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		for name, val := range vars {
			segment, rest := cutSegment(name)
			if idx, err := strconv.Atoi(segment); err == nil && strconv.Itoa(idx) == segment && idx >= 0 {
				group(segment, rest, val)
			}
		}
		if err := build(t.Elem()); err != nil {
			return nil, false, err
		}
		if len(items) == 0 {
			return nil, false, nil
		}
		list := make([]any, len(items))
		for i := range list {
			item, ok := items[strconv.Itoa(i)]
			if !ok {
				return nil, false, fmt.Errorf("index %d is not defined, indexes should be sequential and start from 0", i)
			}
			list[i] = item
		}
		return list, true, nil
	case reflect.Map:
		for name, val := range vars {
			if convert.IsScalar(t.Elem()) || t.Elem().Kind() == reflect.Interface {
				// the whole name is a key
				group(name, "", val)
				continue
			}
			segment, rest := cutSegment(name)
			group(segment, rest, val)
		}
		if err := build(t.Elem()); err != nil {
			return nil, false, err
		}
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			fld := t.Field(i)
			envKey, treeKey := fld.Tag.Get("env"), fld.Tag.Get("env")
			if envKey == "" && fld.Anonymous && derefType(fld.Type).Kind() == reflect.Struct && !convert.IsLeaf(fld.Type) {
				// fields of the embedded struct are on the level of the embedding struct, like in encoding/json
				embedded, found, err := indexedTree(fld.Type, vars)
				if err != nil {
					return nil, false, err
				}
				if found {
					for key, item := range embedded.(map[string]any) {
						items[key] = item
					}
				}
				continue
			}
			if fld.PkgPath != "" || envKey == "-" {
				continue
			}
			if envKey == "" {
				envKey, treeKey = autoKey(fld.Name), fld.Name
			}
			groups = make(map[string]map[string]string)
			for name, val := range vars {
				if name == envKey {
					group(treeKey, "", val)
				} else if strings.HasPrefix(name, envKey+"_") && !hasLongerField(t, name, envKey) {
					group(treeKey, name[len(envKey)+1:], val)
				}
			}
			if err := build(fld.Type); err != nil {
				return nil, false, err
			}
		}
	}
	if len(items) == 0 {
		return nil, false, nil
	}
	return items, true, nil
}

// hasLongerField reports whether the struct has another field that matches the name of the variable better than envKey:
// HOST_NAME should be used for the field HOST_NAME, not for the field HOST.
func hasLongerField(t reflect.Type, name, envKey string) bool {
	for i := 0; i < t.NumField(); i++ {
		fld := t.Field(i)
		other := fld.Tag.Get("env")
		if other == "" {
			other = autoKey(fld.Name)
		}
		if fld.PkgPath == "" && len(other) > len(envKey) && (name == other || strings.HasPrefix(name, other+"_")) {
			return true
		}
	}
	return false
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
	indexedKeys bool
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

// WithIndexedKeys enables reading of slices, arrays, maps and structs from the separate variables,
// if the variable with the whole value is not set:
//   - slices and arrays: HOSTS_0=a, HOSTS_1=b, indexes should be sequential and start from 0;
//   - maps: LABELS_env=prod, the rest of the variable name is a key, as is;
//   - structs: SERVER_HOST=a, SERVER_MAX_CONNS=1, names are taken from the env tags of the struct fields
//     or built from the field names;
//   - nested types: SERVERS_0_HOST=a, SERVERS_0_PORT=1, MATRIX_0_1=2. Keys of the maps with nested types
//     should not contain underscores.
//
// Variables that do not match the type of the field are ignored.
// For maps with simple values all variables with the name of the field as a prefix are used as map items,
// so the name of such field should not be a prefix of other variables: with LABELS and LABELS_EXTRA fields
// the variable LABELS_EXTRA_a is also an item of LABELS.
func WithIndexedKeys() Option {
	return func(o *options) {
		o.indexedKeys = true
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
		indexedKeys: o.indexedKeys,
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
