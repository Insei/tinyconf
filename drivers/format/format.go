// Package format contains the driver for the config files of any format with nested objects:
// the format is set by the function that decodes the content to the map, like the yaml, json and toml drivers do.
package format

import (
	"errors"
	"io/fs"
	"time"

	"github.com/insei/fmap/v3"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/convert"
	"github.com/insei/tinyconf/drivers/internal/tree"
)

// Decoder decodes the content of the config to the map: keys of the map are the keys of the config,
// nested maps with string keys are nested objects, slices are lists, other values are scalars.
// Empty content can be decoded to nil map.
type Decoder func(data []byte) (map[string]any, error)

// Encoder encodes the document with the current values of the registered configs for GenDoc,
// the document has the same structure as the result of Decoder.
type Encoder func(doc map[string]any) ([]byte, error)

type options struct {
	tree   tree.Options
	encode Encoder
}

// Option is a format driver option.
type Option func(*options)

// WithFiles adds additional config files. Files are merged in the order of addition,
// values from the next files overrides values from the previous, nested objects are merged by keys.
func WithFiles(files ...string) Option {
	return func(o *options) {
		o.tree.AddFiles(files...)
	}
}

// WithFS adds config files from the fsys, embed.FS for example.
func WithFS(fsys fs.FS, files ...string) Option {
	return func(o *options) {
		o.tree.AddFS(fsys, files...)
	}
}

// WithData adds in-memory config content, name is used in error messages.
func WithData(name string, data []byte) Option {
	return func(o *options) {
		o.tree.AddData(name, data)
	}
}

// WithSource adds the config content from another source, from a remote storage for example:
// consul.Document reads the content from Consul KV. Sources are merged with the files in the order of addition.
func WithSource(source tree.Source) Option {
	return func(o *options) {
		o.tree.AddSource(source)
	}
}

// WithOptional allows config files to be absent, by default absent file is an error.
func WithOptional() Option {
	return func(o *options) {
		o.tree.Optional = true
	}
}

// WithEnvExpansion replaces ${VAR} and ${VAR:-default} in the config content with the values of the environment variables.
// Default is used when variable is not set or empty, $$ is replaced with $.
func WithEnvExpansion() Option {
	return func(o *options) {
		o.tree.ExpandEnv = true
	}
}

// WithWatchInterval sets the interval of polling the config files for changes while there are subscriptions, 5 seconds by default.
// The files are watched by the events of the file system too, polling detects the changes if the events are lost.
func WithWatchInterval(interval time.Duration) Option {
	return func(o *options) {
		o.tree.WatchInterval = interval
	}
}

// WithEncoder sets the encoder of the documents for GenDoc, without it GenDoc returns an empty string.
func WithEncoder(encode Encoder) Option {
	return func(o *options) {
		o.encode = encode
	}
}

type formatDriver struct {
	*tree.Driver
	name   string
	encode Encoder
}

// New creates new driver for the config files of the format, the name of the driver is the name of the struct tag with the keys:
//
//	format.New("hcl", decodeHCL, "config.hcl")
//
// reads the field with hcl:"port" inside hcl:"http" from the port key of the http object.
// The driver has the same features as the yaml, json and toml drivers: several files, merging, reloading, documentation.
// File can be empty if config sources are added by options.
func New(name string, decode Decoder, file string, opts ...Option) (tinyconf.Driver, error) {
	if name == "" {
		return nil, errors.New("name of the format driver is empty")
	}
	if decode == nil {
		return nil, errors.New("decoder of the format driver is nil")
	}
	o := &options{}
	o.tree.AddFiles(file)
	for _, opt := range opts {
		opt(o)
	}
	return &formatDriver{Driver: o.tree.NewDriver(name, tree.Decoder(decode)), name: name, encode: o.encode}, nil
}

// GenDoc generates the document with all registered config fields and their current values by the encoder.
// Values of the fields with hidden:"true" tag are empty strings, fields without values are skipped.
// Empty string is returned without the encoder, if there are no fields or the document can not be encoded.
func (d *formatDriver) GenDoc(registers ...*tinyconf.Registered) string {
	if d.encode == nil {
		return ""
	}
	doc := tree.Doc(registers, d.name, func(v any, _ fmap.Field) any {
		return convert.ToTree(v, d.name)
	})
	if len(doc) == 0 {
		return ""
	}
	data, err := d.encode(doc)
	if err != nil {
		return ""
	}
	return string(data)
}
