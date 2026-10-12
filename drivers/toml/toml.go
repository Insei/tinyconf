// Package toml contains the driver that reads config from TOML files.
package toml

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/convert"
	"github.com/insei/tinyconf/drivers/internal/tree"
)

type tomlDriver struct {
	name string
	*tree.Driver
}

func (d *tomlDriver) GetName() string {
	return d.name
}

func decode(data []byte) (map[string]any, error) {
	tomlMap := make(map[string]any)
	if err := toml.Unmarshal(data, &tomlMap); err != nil {
		return nil, fmt.Errorf("failed to decode toml: %w", err)
	}
	return normalize(tomlMap).(map[string]any), nil
}

// normalize converts decoded values to the same types that are used by other tree drivers:
// integers to int, arrays of tables to []any.
func normalize(v any) any {
	switch val := v.(type) {
	case int64:
		if int64(int(val)) == val {
			return int(val)
		}
	case []any:
		for i := range val {
			val[i] = normalize(val[i])
		}
	case []map[string]any:
		items := make([]any, len(val))
		for i := range val {
			items[i] = normalize(val[i])
		}
		return items
	case map[string]any:
		for key := range val {
			val[key] = normalize(val[key])
		}
	}
	return v
}

// GenDoc generates TOML document with all registered config fields and their current values.
// Values of the fields with hidden:"true" tag are replaced with empty strings, fields without values are skipped.
func (d *tomlDriver) GenDoc(registers ...*tinyconf.Registered) string {
	doc := make(map[string]any)
	for _, register := range registers {
		for _, path := range register.Storage.GetAllPaths() {
			fld := register.Storage.MustFind(path)
			tagPath := fld.GetTagPath(d.name, false)
			if tagPath == "" || !convert.IsValueField(register.Storage, fld) {
				continue
			}
			var value any = ""
			if fld.GetTag().Get("hidden") != "true" {
				value = withoutNil(convert.ToTree(convert.GetFieldValue(fld, register.Config), d.name))
			}
			if value == nil {
				// TOML has no null
				continue
			}
			keys := strings.Split(tagPath, ".")
			node := doc
			for _, key := range keys[:len(keys)-1] {
				nested, ok := node[key].(map[string]any)
				if !ok {
					nested = make(map[string]any)
					node[key] = nested
				}
				node = nested
			}
			node[keys[len(keys)-1]] = value
		}
	}
	return encodeDoc(doc)
}

// encodeDoc encodes the document to TOML, empty string is returned if the document is empty or can not be encoded.
func encodeDoc(doc map[string]any) string {
	if len(doc) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(doc); err != nil {
		return ""
	}
	return buf.String()
}

// withoutNil removes nil values from the tree, TOML has no null.
func withoutNil(v any) any {
	switch val := v.(type) {
	case []any:
		items := make([]any, 0, len(val))
		for _, item := range val {
			if item = withoutNil(item); item != nil {
				items = append(items, item)
			}
		}
		return items
	case map[string]any:
		for key, item := range val {
			if item = withoutNil(item); item == nil {
				delete(val, key)
			} else {
				val[key] = item
			}
		}
	}
	return v
}

// Option is a toml driver option.
type Option func(*tree.Options)

// WithFiles adds additional config files. Files are merged in the order of addition,
// values from the next files overrides values from the previous, nested tables are merged by keys.
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

// WithWatchInterval sets the interval of checking the config files for changes in Manager.Watch, 5 seconds by default.
func WithWatchInterval(interval time.Duration) Option {
	return func(o *tree.Options) {
		o.WatchInterval = interval
	}
}

// New creates new toml driver. Keys are taken from the toml struct tags.
// File can be empty if config sources are added by options.
func New(file string, opts ...Option) (tinyconf.Driver, error) {
	o := &tree.Options{}
	o.AddFiles(file)
	for _, opt := range opts {
		opt(o)
	}
	return &tomlDriver{
		name:   "toml",
		Driver: o.NewDriver("toml", decode),
	}, nil
}
