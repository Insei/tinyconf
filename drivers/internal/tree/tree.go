// Package tree contains the common part of the drivers
// that reads config from the formats with nested maps: yaml, json and others.
package tree

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/insei/fmap/v3"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/convert"
)

// Decoder decodes the config content to the map.
type Decoder func(data []byte) (map[string]any, error)

// Source is a source of the config content: file, embedded file, bytes.
type Source struct {
	Name string
	Open func() (io.ReadCloser, error)
	// Optional source can be absent.
	Optional bool
}

// FileSource returns source that reads the file from the OS file system.
func FileSource(filePath string) Source {
	return Source{Name: filePath, Open: func() (io.ReadCloser, error) {
		return os.Open(filePath)
	}}
}

// FSSource returns source that reads the file from the fsys.
func FSSource(fsys fs.FS, filePath string) Source {
	return Source{Name: filePath, Open: func() (io.ReadCloser, error) {
		return fsys.Open(filePath)
	}}
}

// DataSource returns source with in-memory content.
func DataSource(name string, data []byte) Source {
	return Source{Name: name, Open: func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(data))), nil
	}}
}

// Driver is a common part of the tree based drivers, GenDoc should be implemented by the format driver.
type Driver struct {
	name      string
	decode    Decoder
	sources   []Source
	expandEnv bool

	mu     sync.RWMutex
	loaded bool
	tree   map[string]any
}

// New creates new tree driver, name is used as driver name and as name of the struct tag with keys.
func New(name string, decode Decoder, sources []Source, expandEnv bool) *Driver {
	return &Driver{name: name, decode: decode, sources: sources, expandEnv: expandEnv}
}

func (d *Driver) GetName() string {
	return d.name
}

// Load reads and merges all sources, values from the next sources overrides values from the previous.
func (d *Driver) Load() error {
	tree := make(map[string]any)
	for _, source := range d.sources {
		sourceTree, err := d.loadSource(source)
		if err != nil {
			return fmt.Errorf("failed to load %s: %w", source.Name, err)
		}
		Merge(tree, sourceTree)
	}
	d.mu.Lock()
	d.tree = tree
	d.loaded = true
	d.mu.Unlock()
	return nil
}

func (d *Driver) loadSource(source Source) (map[string]any, error) {
	rc, err := source.Open()
	if err != nil {
		if source.Optional && errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	if d.expandEnv {
		data = []byte(ExpandEnv(string(data)))
	}
	return d.decode(data)
}

func (d *Driver) getTree() (map[string]any, error) {
	d.mu.RLock()
	tree, loaded := d.tree, d.loaded
	d.mu.RUnlock()
	if loaded {
		return tree, nil
	}
	// driver is used without tinyconf.Manager
	if err := d.Load(); err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.tree, nil
}

func (d *Driver) GetValue(field fmap.Field) (*tinyconf.Value, error) {
	tree, err := d.getTree()
	if err != nil {
		return nil, err
	}
	val, err := Lookup(field, d.name, tree)
	if err != nil {
		return nil, err
	}
	val, err = convert.TreeToField(val, field, d.name)
	if err != nil {
		return nil, fmt.Errorf("failed to convert %s value to field type value: %w", d.name, err)
	}
	return &tinyconf.Value{Source: d.name, Value: val}, nil
}

// Lookup returns raw value for the field from the tree, path to the value is built from the tags of the field and its parents.
func Lookup(field fmap.Field, tag string, tree map[string]any) (any, error) {
	pathKey := field.GetTagPath(tag, true)
	if pathKey == "" {
		return nil, fmt.Errorf("%w: '%s' tag is not set", tinyconf.ErrIncorrectTagSettings, tag)
	}
	keys := strings.Split(pathKey, ".")
	for _, key := range keys[:len(keys)-1] {
		nested, ok := tree[key].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: value not found in %s config", tinyconf.ErrValueNotFound, tag)
		}
		tree = nested
	}
	val, ok := tree[keys[len(keys)-1]]
	if !ok || val == nil {
		return nil, fmt.Errorf("%w: value not found in %s config", tinyconf.ErrValueNotFound, tag)
	}
	return val, nil
}

// Merge merges src to dst, nested maps are merged recursively, all other values are replaced.
func Merge(dst, src map[string]any) {
	for key, srcVal := range src {
		srcMap, srcIsMap := srcVal.(map[string]any)
		dstMap, dstIsMap := dst[key].(map[string]any)
		if srcIsMap && dstIsMap {
			Merge(dstMap, srcMap)
			continue
		}
		dst[key] = srcVal
	}
}

var envRegexp = regexp.MustCompile(`\$\$|\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

// ExpandEnv replaces ${VAR} and ${VAR:-default} with the values of the environment variables.
// Default is used when variable is not set or empty, $$ is replaced with $.
func ExpandEnv(s string) string {
	return envRegexp.ReplaceAllStringFunc(s, func(match string) string {
		if match == "$$" {
			return "$"
		}
		groups := envRegexp.FindStringSubmatch(match)
		if val := os.Getenv(groups[1]); val != "" {
			return val
		}
		return groups[3]
	})
}
