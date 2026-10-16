// Package json contains the driver that reads config from JSON files.
package json

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/convert"
	"github.com/insei/tinyconf/drivers/internal/tree"
)

type jsonDriver struct {
	name string
	*tree.Driver
}

func (d *jsonDriver) GetName() string {
	return d.name
}

func decode(data []byte) (map[string]any, error) {
	if strings.TrimSpace(string(data)) == "" {
		return nil, nil
	}
	val, err := convert.DecodeJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode json: %w", err)
	}
	if val == nil {
		return nil, nil
	}
	jsonMap, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("failed to decode json: object expected")
	}
	return jsonMap, nil
}

// GenDoc generates JSON document with all registered config fields and their current values.
// Values of the fields with hidden:"true" tag are replaced with empty strings.
func (d *jsonDriver) GenDoc(registers ...*tinyconf.Registered) string {
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
				value = convert.GetFieldValue(fld, register.Config)
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
			node[keys[len(keys)-1]] = convert.ToTree(value, d.name)
		}
	}
	if len(doc) == 0 {
		return ""
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return ""
	}
	return string(data) + "\n"
}

// Option is a json driver option.
type Option func(*tree.Options)

// WithFiles adds additional config files. Files are merged in the order of addition,
// values from the next files overrides values from the previous, nested objects are merged by keys.
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

// WithWatchInterval sets the interval of checking the config files for changes while there are subscriptions, 5 seconds by default.
func WithWatchInterval(interval time.Duration) Option {
	return func(o *tree.Options) {
		o.WatchInterval = interval
	}
}

// New creates new json driver. Keys are taken from the json struct tags.
// File can be empty if config sources are added by options.
func New(file string, opts ...Option) (tinyconf.Driver, error) {
	o := &tree.Options{}
	o.AddFiles(file)
	for _, opt := range opts {
		opt(o)
	}
	return &jsonDriver{
		name:   "json",
		Driver: o.NewDriver("json", decode),
	}, nil
}
