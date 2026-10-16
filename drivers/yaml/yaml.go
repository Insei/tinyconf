package yaml

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"time"

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
	// section - struct that is parsed by its fields, it has no value
	section bool
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
	return fmt.Sprintf("%s\n%s: %s\n", tagDoc, tagDriver, f.formatValue())
}

// formatValue returns field value in the yaml format, slices and maps are formatted in the flow style.
func (f field) formatValue() string {
	typeOf := reflect.TypeOf(f.value)
	if typeOf == nil || f.tag.Get("hidden") == "true" || f.section {
		return ""
	}
	switch tree := convert.ToTree(f.value, "yaml").(type) {
	case nil:
		return ""
	case []any, map[string]any:
		// JSON is a valid yaml
		data, err := json.Marshal(tree)
		if err != nil {
			return ""
		}
		return string(data)
	default:
		return fmt.Sprint(tree)
	}
}

func (d *yamlDriver) getUniqueFields(registers []*tinyconf.Registered) []field {
	var fields []field
	for _, register := range registers {
		for _, path := range register.Storage.GetAllPaths() {
			fld := register.Storage.MustFind(path)

			tag := fld.GetTag()
			tagDriver, ok := tag.Lookup(d.name)
			if !ok || convert.IsInsideValue(register.Storage, fld) {
				continue
			}

			member := field{
				path:    fld.GetTagPath(d.name, false),
				value:   convert.GetFieldValue(fld, register.Config),
				tag:     tag,
				section: !convert.IsValueField(register.Storage, fld),
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

// Option is a yaml driver option.
type Option func(*tree.Options)

// WithFiles adds additional config files. Files are merged in the order of addition,
// values from the next files overrides values from the previous, nested maps are merged by keys.
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

// New creates new yaml driver. File can be empty if config sources are added by options.
func New(file string, opts ...Option) (tinyconf.Driver, error) {
	o := &tree.Options{}
	o.AddFiles(file)
	for _, opt := range opts {
		opt(o)
	}
	return &yamlDriver{
		name:   "yaml",
		Driver: o.NewDriver("yaml", decode),
	}, nil
}
