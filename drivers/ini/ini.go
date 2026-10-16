// Package ini contains the driver that reads config from INI files.
package ini

import (
	"fmt"
	"io/fs"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/insei/fmap/v3"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/convert"
	"github.com/insei/tinyconf/drivers/internal/tree"
)

const name = "ini"

type iniDriver struct {
	*tree.Driver
}

// splitKey splits the key or the name of the section to the path: http.tls is tls inside http.
func splitKey(key string) ([]string, error) {
	keys := strings.Split(key, ".")
	for i, k := range keys {
		if keys[i] = strings.TrimSpace(k); keys[i] == "" {
			return nil, fmt.Errorf("incorrect key %q", key)
		}
	}
	return keys, nil
}

// parseValue returns the value without quotes and comments: "quoted with \"escapes\"", 'quoted as is',
// or the value until the comment that starts with ; or # after a space.
func parseValue(value string) (string, error) {
	switch {
	case strings.HasPrefix(value, `"`):
		return strconv.Unquote(value)
	case strings.HasPrefix(value, `'`) && len(value) > 1 && strings.HasSuffix(value, `'`):
		return value[1 : len(value)-1], nil
	}
	for i := 1; i < len(value); i++ {
		if (value[i] == ';' || value[i] == '#') && (value[i-1] == ' ' || value[i-1] == '\t') {
			return strings.TrimSpace(value[:i]), nil
		}
	}
	return value, nil
}

// decode decodes INI: sections are the nested objects, keys with dots are the nested objects too.
func decode(data []byte) (map[string]any, error) {
	doc := make(map[string]any)
	var section []string
	for n, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if line[0] == '[' {
			if !strings.HasSuffix(line, "]") {
				return nil, fmt.Errorf("line %d: section without ]", n+1)
			}
			keys, err := splitKey(line[1 : len(line)-1])
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", n+1, err)
			}
			if err = tree.SetPath(doc, keys, nil); err != nil {
				return nil, fmt.Errorf("line %d: %w", n+1, err)
			}
			section = keys
			continue
		}
		idx := strings.IndexAny(line, "=:")
		if idx < 0 {
			return nil, fmt.Errorf("line %d: key without value", n+1)
		}
		keys, err := splitKey(line[:idx])
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n+1, err)
		}
		value, err := parseValue(strings.TrimSpace(line[idx+1:]))
		if err != nil {
			return nil, fmt.Errorf("line %d: incorrect quoted value: %w", n+1, err)
		}
		if err = tree.SetPath(doc, append(append([]string{}, section...), keys...), value); err != nil {
			return nil, fmt.Errorf("line %d: %w", n+1, err)
		}
	}
	return doc, nil
}

// quote quotes the value if it can not be written as is: in single quotes as is, or in double quotes with escapes.
func quote(value string) string {
	if value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, ";#\"'\\\n\r") {
		return value
	}
	if value != "" && !strings.ContainsAny(value, "'\n\r") {
		return "'" + value + "'"
	}
	return strconv.Quote(value)
}

// encode writes the values of the object, then its nested objects as sections.
func encode(doc *strings.Builder, path []string, object map[string]any) {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var sections []string
	header := false
	for _, key := range keys {
		value, isValue := object[key].(string)
		if !isValue {
			sections = append(sections, key)
			continue
		}
		if !header && len(path) > 0 {
			if doc.Len() > 0 {
				doc.WriteString("\n")
			}
			doc.WriteString("[" + strings.Join(path, ".") + "]\n")
		}
		header = true
		doc.WriteString(key + " = " + quote(value) + "\n")
	}
	for _, key := range sections {
		encode(doc, append(append([]string{}, path...), key), object[key].(map[string]any))
	}
}

// GenDoc generates INI file with all registered config fields and their current values,
// nested structs are sections. Values of the fields with hidden:"true" tag are empty, nil values are skipped.
func (d *iniDriver) GenDoc(registers ...*tinyconf.Registered) string {
	values := tree.Doc(registers, name, func(v any, field fmap.Field) any {
		if valOf := reflect.ValueOf(v); !valOf.IsValid() || (valOf.Kind() == reflect.Ptr && valOf.IsNil()) {
			return nil
		}
		return convert.FieldToTagString(v, field, name)
	})
	var doc strings.Builder
	encode(&doc, nil, values)
	return doc.String()
}

// Option is an ini driver option.
type Option func(*tree.Options)

// WithFiles adds additional config files. Files are merged in the order of addition,
// values from the next files overrides values from the previous, sections are merged by keys.
func WithFiles(files ...string) Option {
	return func(o *tree.Options) {
		o.AddFiles(files...)
	}
}

// WithFS adds config files from the fsys, embed.FS for example.
func WithFS(fsys fs.FS, files ...string) Option {
	return func(o *tree.Options) {
		o.AddFS(fsys, files...)
	}
}

// WithData adds in-memory config content, name is used in error messages.
func WithData(name string, data []byte) Option {
	return func(o *tree.Options) {
		o.AddData(name, data)
	}
}

// WithSource adds the config content from another source, from a remote storage for example:
// consul.Document reads the content from Consul KV. Sources are merged with the files in the order of addition.
func WithSource(source tree.Source) Option {
	return func(o *tree.Options) {
		o.AddSource(source)
	}
}

// WithOptional allows config files to be absent, by default absent file is an error.
func WithOptional() Option {
	return func(o *tree.Options) {
		o.Optional = true
	}
}

// WithEnvExpansion replaces ${VAR} and ${VAR:-default} in the config content with the values of the environment variables.
// Default is used when variable is not set or empty, $$ is replaced with $.
func WithEnvExpansion() Option {
	return func(o *tree.Options) {
		o.ExpandEnv = true
	}
}

// WithWatchInterval sets the interval of polling the config files for changes while there are subscriptions, 5 seconds by default.
// The files are watched by the events of the file system too, polling detects the changes if the events are lost.
func WithWatchInterval(interval time.Duration) Option {
	return func(o *tree.Options) {
		o.WatchInterval = interval
	}
}

// New creates new ini driver. Keys are taken from the ini struct tags, the tags of the parents are the sections:
// ini:"port" inside ini:"http" is read from the port key of the [http] section.
// File can be empty if config sources are added by options.
func New(file string, opts ...Option) (tinyconf.Driver, error) {
	o := &tree.Options{}
	o.AddFiles(file)
	for _, opt := range opts {
		opt(o)
	}
	return &iniDriver{Driver: o.NewDriver(name, decode)}, nil
}
