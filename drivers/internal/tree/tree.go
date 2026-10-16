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
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/insei/fmap/v3"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/convert"
)

// Decoder decodes the config content to the map.
type Decoder func(data []byte) (map[string]any, error)

// Source is a source of the config content: file, embedded file, bytes.
type Source struct {
	Name string
	// Open opens the content of the source, ctx is the context of Parse, remote sources should use it for the requests.
	Open func(ctx context.Context) (io.ReadCloser, error)
	// Stat returns the state of the source, the source is changed if the state is changed.
	// Sources without Stat and Watch are never changed.
	Stat func(ctx context.Context) string
	// Watch blocks until ctx is done and calls onChange after every change of the source,
	// for sources that are notified about changes, a remote storage for example.
	// It can be called from several goroutines.
	Watch func(ctx context.Context, onChange func()) error
	// Optional source can be absent.
	Optional bool
	// File is the path of the file in the OS file system, the events of its directory are watched.
	File string
}

// FileSource returns source that reads the file from the OS file system.
func FileSource(filePath string) Source {
	return Source{
		Name: filePath,
		File: filePath,
		Open: func(context.Context) (io.ReadCloser, error) {
			return os.Open(filePath)
		},
		Stat: func(context.Context) string {
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
	return Source{Name: filePath, Open: func(context.Context) (io.ReadCloser, error) {
		return fsys.Open(filePath)
	}}
}

// DataSource returns source with in-memory content.
func DataSource(name string, data []byte) Source {
	return Source{Name: name, Open: func(context.Context) (io.ReadCloser, error) {
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

// AddSource adds the source, a remote storage for example.
func (o *Options) AddSource(source Source) {
	o.Sources = append(o.Sources, source)
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
	return d.LoadContext(context.Background())
}

// LoadContext is Load with the context for the remote sources, Manager calls it with the context of ParseContext.
func (d *Driver) LoadContext(ctx context.Context) error {
	// state is taken before reading, so the changes made during the reading will be detected by Watch
	state := d.stat(ctx)
	tree := make(map[string]any)
	for _, source := range d.sources {
		sourceTree, err := d.loadSource(ctx, source)
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

func (d *Driver) loadSource(ctx context.Context, source Source) (map[string]any, error) {
	rc, err := source.Open(ctx)
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

func (d *Driver) stat(ctx context.Context) string {
	var state strings.Builder
	for _, source := range d.sources {
		if source.Stat != nil {
			state.WriteString(source.Stat(ctx))
		}
		state.WriteByte(';')
	}
	return state.String()
}

var (
	// newFileWatcher creates the watcher of the file system events, it is a variable to check its errors in tests.
	newFileWatcher = fsnotify.NewWatcher
	// eventsDelay is the time of waiting for the next events after the event of the file system:
	// editors write the file by several operations, the file is checked when they are finished.
	eventsDelay = 100 * time.Millisecond
)

// fileEvents returns the channel of the events of the directories with the files of the sources and the function
// that stops watching. Nil channel is returned if the directories can not be watched, the files are checked by polling then.
// Directories are watched instead of the files, so the replaced files and symlinks are watched too:
// editors, atomic renames, updates of ConfigMap volumes in Kubernetes.
func (d *Driver) fileEvents() (<-chan fsnotify.Event, func()) {
	dirs := make(map[string]bool)
	for _, source := range d.sources {
		if source.File != "" {
			dirs[filepath.Dir(source.File)] = true
		}
	}
	if len(dirs) == 0 {
		return nil, func() {}
	}
	watcher, err := newFileWatcher()
	if err != nil {
		return nil, func() {}
	}
	added := false
	for dir := range dirs {
		// absent directories are checked by polling
		if watcher.Add(dir) == nil {
			added = true
		}
	}
	if !added {
		_ = watcher.Close()
		return nil, func() {}
	}
	go func() {
		// errors should be read, otherwise the watcher is blocked; the files are checked by polling anyway
		for range watcher.Errors {
		}
	}()
	return watcher.Events, func() { _ = watcher.Close() }
}

// Watch checks the sources for changes and calls onChange if any source is changed.
// Files are checked after the events of the file system and with the watch interval, if the events are lost.
// Sources with Watch report their changes by themselves. It blocks until ctx is done.
func (d *Driver) Watch(ctx context.Context, onChange func()) error {
	// watchers of the sources are stopped and finished before return
	var wg sync.WaitGroup
	defer wg.Wait()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, len(d.sources))
	for _, source := range d.sources {
		if source.Watch == nil {
			continue
		}
		wg.Add(1)
		go func(source Source) {
			defer wg.Done()
			if err := source.Watch(ctx, onChange); err != nil && ctx.Err() == nil {
				errs <- fmt.Errorf("failed to watch %s: %w", source.Name, err)
			}
		}(source)
	}
	ticker := time.NewTicker(d.watchInterval)
	defer ticker.Stop()
	events, stopEvents := d.fileEvents()
	defer stopEvents()
	// changes made after the last Load should be detected too
	d.mu.RLock()
	state, loaded := d.state, d.loaded
	d.mu.RUnlock()
	if !loaded {
		state = d.stat(ctx)
	}
	check := func() {
		if newState := d.stat(ctx); newState != state {
			state = newState
			onChange()
		}
	}
	var delay <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errs:
			return err
		case <-ticker.C:
			check()
		case _, ok := <-events:
			if !ok {
				// the watcher is closed, the files are checked by polling
				events = nil
				continue
			}
			delay = time.After(eventsDelay)
		case <-delay:
			delay = nil
			check()
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
	key := field.GetTagPath(d.name, true)
	return &tinyconf.Value{Source: key, Value: val, Warning: ParentTagWarning(field, d.name, key)}, nil
}

// ParentTagWarning returns the warning if some parents of the field have no tag:
// the path is built without such parents and all parents above them,
// so the value is read from the unexpected place of the config, key is the place of the value in the source.
func ParentTagWarning(field fmap.Field, tag, key string) string {
	var parents []string
	for parent := field.GetParent(); parent != nil; parent = parent.GetParent() {
		// embedded structs without the tag are on the level of the embedding struct, it is expected
		if name := strings.Split(parent.GetTag().Get(tag), ",")[0]; name == "" && !parent.GetAnonymous() {
			parents = append([]string{parent.GetStructPath()}, parents...)
		}
	}
	if len(parents) == 0 {
		return ""
	}
	return fmt.Sprintf("parent fields %s have no '%s' tag, the value is read from the '%s' key",
		strings.Join(parents, ", "), tag, key)
}

// Lookup returns raw value for the field from the tree, path to the value is built from the tags of the field and its parents.
// If the parent has no tag, the path is built only from the tags below it.
// Fields of embedded structs without the tag are on the level of the embedding struct, like in encoding/json.
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

// Doc returns the document with the current values of the fields of the registered configs:
// nested maps by the paths of the tags, fields under the parents without the tag are skipped.
// Values of the fields with hidden:"true" tag are empty strings.
// Value converts the value of the field to the value of the document, nil value skips the field.
func Doc(registers []*tinyconf.Registered, tag string, value func(v any, field fmap.Field) any) map[string]any {
	doc := make(map[string]any)
	for _, register := range registers {
		for _, path := range register.Storage.GetAllPaths() {
			fld := register.Storage.MustFind(path)
			tagPath := fld.GetTagPath(tag, false)
			if tagPath == "" || !convert.IsValueField(register.Storage, fld) {
				continue
			}
			var val any = ""
			if fld.GetTag().Get("hidden") != "true" {
				val = value(convert.GetFieldValue(fld, register.Config), fld)
			}
			if val == nil {
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
			node[keys[len(keys)-1]] = val
		}
	}
	return doc
}

// SetPath sets the value in the document by the path of the keys, nested maps are created on the way.
// Nil value creates the nested map only. A key can not be a value and an object at the same time,
// the formats with flat keys, like INI and properties, are checked by it.
func SetPath(doc map[string]any, keys []string, value any) error {
	node := doc
	for i, key := range keys {
		if i == len(keys)-1 && value != nil {
			if _, isObject := node[key].(map[string]any); isObject {
				return fmt.Errorf("%s is an object, it can not have a value", strings.Join(keys, "."))
			}
			node[key] = value
			return nil
		}
		next, exists := node[key]
		nested, isObject := next.(map[string]any)
		if exists && !isObject {
			return fmt.Errorf("%s has a value, it can not be an object", strings.Join(keys[:i+1], "."))
		}
		if !isObject {
			nested = make(map[string]any)
			node[key] = nested
		}
		node = nested
	}
	return nil
}
