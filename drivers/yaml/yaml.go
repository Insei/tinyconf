package yaml

import (
	"fmt"
	"io/fs"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/cmp118"
	"github.com/insei/tinyconf/convert"
	"github.com/insei/tinyconf/drivers/internal/tree"
	"github.com/insei/tinyconf/slices118"
)

type yamlDriver struct {
	name string
	*tree.Driver
}

func (d *yamlDriver) GetName() string {
	return d.name
}

func decode(data []byte) (map[string]any, error) {
	yamlMap := make(map[string]any)
	if err := yaml.Unmarshal(data, &yamlMap); err != nil {
		return nil, fmt.Errorf("failed to decode yaml: %w", err)
	}
	return yamlMap, nil
}

type field struct {
	path  string
	value any
	tag   reflect.StructTag
}

func (f field) genDoc(driver string, depth int) string {
	var offset strings.Builder
	for i := 0; i < depth; i++ {
		offset.WriteRune('\t')
	}
	offset.WriteRune('#')
	tagDriver := offset.String() + f.tag.Get(driver)
	tagDoc := offset.String() + f.tag.Get("doc")
	offset.Reset()
	if reflect.TypeOf(f.value).Kind() == reflect.Struct || f.tag.Get("hidden") == "true" {
		f.value = ""
	}
	return fmt.Sprintf("%s\n%s: %v\n", tagDoc, tagDriver, f.value)
}

func (d *yamlDriver) getUniqueFields(registers []*tinyconf.Registered) []field {
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
				path:  fld.GetTagPath(d.name, false),
				value: convert.GetFieldValue(fld, register.Config),
				tag:   tag,
			}

			if slices118.ContainsFunc(fields, func(item field) bool {
				matchPath := item.path == member.path
				matchTagDriver := tagDriver == item.tag.Get(d.name)
				return matchPath && matchTagDriver
			}) {
				continue
			}
			fields = append(fields, member)
		}
	}
	return fields
}

func (d *yamlDriver) getRootMap(fields []field) map[string]string {
	roots := map[string]string{}
	var root struct {
		path, tag string
	}
	for _, field := range fields {
		depth := strings.Count(field.path, ".")
		if depth == 0 {
			root = struct{ path, tag string }{path: field.path, tag: field.tag.Get(d.name)}
			roots[root.tag] = ""
			continue
		}
		if strings.HasPrefix(field.path, root.path) {
			roots[root.tag] += field.genDoc(d.name, depth)
		}
	}
	return roots
}

func (d *yamlDriver) GenDoc(registers ...*tinyconf.Registered) string {
	uniqueFields := d.getUniqueFields(registers)

	sortedFields := slices118.Clone(uniqueFields)
	slices118.SortStableFunc(sortedFields, func(i, j field) int {
		return cmp118.Compare(i.path, j.path)
	})

	roots := d.getRootMap(sortedFields)

	var doc string
	for _, field := range uniqueFields {
		depth := strings.Count(field.path, ".")
		if depth != 0 {
			continue
		}

		tagRootDriver := field.tag.Get(d.name)
		if nestedDoc, ok := roots[tagRootDriver]; ok {
			rootFieldDoc := field.genDoc(d.name, 0)
			doc += rootFieldDoc + nestedDoc
		}
	}

	return doc
}

type options struct {
	sources   []tree.Source
	optional  bool
	expandEnv bool
}

// Option is a yaml driver option.
type Option func(*options)

// WithFiles adds additional config files. Files are merged in the order of addition,
// values from the next files overrides values from the previous, nested maps are merged by keys.
func WithFiles(files ...string) Option {
	return func(o *options) {
		for _, file := range files {
			o.sources = append(o.sources, tree.FileSource(file))
		}
	}
}

// WithFS adds config files from the fsys, embed.FS for example.
func WithFS(fsys fs.FS, files ...string) Option {
	return func(o *options) {
		for _, file := range files {
			o.sources = append(o.sources, tree.FSSource(fsys, file))
		}
	}
}

// WithData adds in-memory config content, name is used in error messages.
func WithData(name string, data []byte) Option {
	return func(o *options) {
		o.sources = append(o.sources, tree.DataSource(name, data))
	}
}

// WithOptional allows config files to be absent, by default absent file is an error.
func WithOptional() Option {
	return func(o *options) {
		o.optional = true
	}
}

// WithEnvExpansion replaces ${VAR} and ${VAR:-default} in the config content with the values of the environment variables.
// Default is used when variable is not set or empty, $$ is replaced with $.
func WithEnvExpansion() Option {
	return func(o *options) {
		o.expandEnv = true
	}
}

// New creates new yaml driver. File can be empty if config sources are added by options.
func New(file string, opts ...Option) (tinyconf.Driver, error) {
	o := &options{}
	if file != "" {
		o.sources = append(o.sources, tree.FileSource(file))
	}
	for _, opt := range opts {
		opt(o)
	}
	for i := range o.sources {
		o.sources[i].Optional = o.optional
	}
	return &yamlDriver{
		name:   "yaml",
		Driver: tree.New("yaml", decode, o.sources, o.expandEnv),
	}, nil
}
