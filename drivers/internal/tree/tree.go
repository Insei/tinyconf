// Package tree contains the common part of the drivers
// that reads config from the formats with nested maps: yaml, json and others.
package tree

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

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
	// Stat returns the state of the source, the source is changed if the state is changed.
	// Sources without Stat are never changed.
	Stat func() string
	// Optional source can be absent.
	Optional bool
}

// FileSource returns source that reads the file from the OS file system.
func FileSource(filePath string) Source {
	return Source{
		Name: filePath,
		Open: func() (io.ReadCloser, error) {
			return os.Open(filePath)
		},
		Stat: func() string {
			info, err := os.Stat(filePath)
			if err != nil {
				return "absent"
			}
			return fmt.Sprintf("%d %d", info.ModTime().UnixNano(), info.Size())
		},
	}
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

// Options is a common options of the tree based drivers.
type Options struct {
	Sources       []Source
	Optional      bool
	ExpandEnv     bool
	WatchInterval time.Duration
}

// AddFiles adds files from the OS file system.
func (o *Options) AddFiles(files ...string) {
	for _, file := range files {
		if file != "" {
			o.Sources = append(o.Sources, FileSource(file))
		}
	}
}

// AddFS adds files from the fsys.
func (o *Options) AddFS(fsys fs.FS, files ...string) {
	for _, file := range files {
		o.Sources = append(o.Sources, FSSource(fsys, file))
	}
}

// AddData adds in-memory content.
func (o *Options) AddData(name string, data []byte) {
	o.Sources = append(o.Sources, DataSource(name, data))
}

// NewDriver creates new tree driver from the options.
func (o *Options) NewDriver(name string, decode Decoder) *Driver {
	sources := make([]Source, len(o.Sources))
	copy(sources, o.Sources)
	for i := range sources {
		sources[i].Optional = o.Optional
	}
	d := New(name, decode, sources, o.ExpandEnv)
	if o.WatchInterval > 0 {
		d.watchInterval = o.WatchInterval
	}
	return d
}

// DefaultWatchInterval is a default interval of checking the sources for changes.
const DefaultWatchInterval = 5 * time.Second

// Driver is a common part of the tree based drivers, GenDoc should be implemented by the format driver.
type Driver struct {
	name          string
	decode        Decoder
	sources       []Source
	expandEnv     bool
	watchInterval time.Duration

	mu     sync.RWMutex
	loaded bool
	tree   map[string]any
	// state of the sources at the moment of the last Load
	state string
}

// New creates new tree driver, name is used as driver name and as name of the struct tag with keys.
func New(name string, decode Decoder, sources []Source, expandEnv bool) *Driver {
	return &Driver{name: name, decode: decode, sources: sources, expandEnv: expandEnv, watchInterval: DefaultWatchInterval}
}

func (d *Driver) GetName() string {
	return d.name
}

// Load reads and merges all sources, values from the next sources overrides values from the previous.
func (d *Driver) Load() error {
	// state is taken before reading, so the changes made during the reading will be detected by Watch
	state := d.stat()
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
	d.state = state
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

func (d *Driver) stat() string {
	var state strings.Builder
	for _, source := range d.sources {
		if source.Stat != nil {
			state.WriteString(source.Stat())
		}
		state.WriteByte(';')
	}
	return state.String()
}

// Watch checks the sources for changes with the watch interval and calls onChange if any source is changed.
// It blocks until ctx is done. Only files from the OS file system are checked.
func (d *Driver) Watch(ctx context.Context, onChange func()) error {
	ticker := time.NewTicker(d.watchInterval)
	defer ticker.Stop()
	// changes made after the last Load should be detected too
	d.mu.RLock()
	state, loaded := d.state, d.loaded
	d.mu.RUnlock()
	if !loaded {
		state = d.stat()
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if newState := d.stat(); newState != state {
				state = newState
				onChange()
			}
		}
	}
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
	// source is the key of the value in the tree, the path by the tags
	return &tinyconf.Value{Source: field.GetTagPath(d.name, true), Value: val, Warning: parentTagWarning(field, d.name)}, nil
}

// parentTagWarning returns the warning if some parents of the field have no tag:
// the path is built without such parents and all parents above them,
// so the value is read from the unexpected place of the config.
func parentTagWarning(field fmap.Field, tag string) string {
	var parents []string
	for parent := field.GetParent(); parent != nil; parent = parent.GetParent() {
		if name := strings.Split(parent.GetTag().Get(tag), ",")[0]; name == "" {
			parents = append([]string{parent.GetStructPath()}, parents...)
		}
	}
	if len(parents) == 0 {
		return ""
	}
	return fmt.Sprintf("parent fields %s have no '%s' tag, the value is read from the '%s' key",
		strings.Join(parents, ", "), tag, field.GetTagPath(tag, true))
}

// Lookup returns raw value for the field from the tree, path to the value is built from the tags of the field and its parents.
// If the parent has no tag, the path is built only from the tags below it.
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
