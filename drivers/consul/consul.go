// Package consul contains the driver that reads config values from Consul KV, every field from its own key,
// and the source of the config document stored in Consul KV for the yaml, json and toml drivers.
package consul

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/insei/fmap/v3"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/convert"
	"github.com/insei/tinyconf/drivers/internal/tree"
)

const name = "consul"

type consulDriver struct {
	prefix string
	client *client

	mu     sync.RWMutex
	loaded bool
	values map[string]string
	// index of the values at the moment of the last Load, zero if it has failed
	index uint64
}

// New creates new Consul KV driver. Every field is read from its own key: the prefix and the path by the consul tags,
// with dots replaced by slashes, so consul:"port" inside consul:"http" is read from prefix/http/port.
// Values are parsed like environment variables: slices are comma separated, structs in slices are JSON.
func New(prefix string, opts ...Option) (tinyconf.Driver, error) {
	return &consulDriver{prefix: strings.Trim(prefix, "/"), client: newClient(opts)}, nil
}

func (d *consulDriver) GetName() string {
	return name
}

// listPrefix returns the prefix of all keys of the driver.
func (d *consulDriver) listPrefix() string {
	if d.prefix == "" {
		return ""
	}
	return d.prefix + "/"
}

func (d *consulDriver) key(tagPath string) string {
	return d.listPrefix() + strings.ReplaceAll(tagPath, ".", "/")
}

// Load reads all keys with the prefix by one request.
func (d *consulDriver) Load() error {
	return d.LoadContext(context.Background())
}

// LoadContext is Load with the context of the request, Manager calls it with the context of Parse.
func (d *consulDriver) LoadContext(ctx context.Context) error {
	pairs, index, err := d.client.load(ctx, d.listPrefix(), true)
	if err != nil {
		// the values from Consul are not in the config, Watch reports the change as soon as Consul is available
		d.mu.Lock()
		d.index = 0
		d.mu.Unlock()
		return err
	}
	values := make(map[string]string, len(pairs))
	for _, p := range pairs {
		// folders have no values
		if p.Value != nil || !strings.HasSuffix(p.Key, "/") {
			values[p.Key] = string(p.Value)
		}
	}
	d.mu.Lock()
	d.values, d.loaded, d.index = values, true, index
	d.mu.Unlock()
	return nil
}

func (d *consulDriver) getValues() (map[string]string, error) {
	d.mu.RLock()
	values, loaded := d.values, d.loaded
	d.mu.RUnlock()
	if loaded {
		return values, nil
	}
	// driver is used without tinyconf.Manager
	if err := d.Load(); err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.values, nil
}

func (d *consulDriver) GetValue(field fmap.Field) (*tinyconf.Value, error) {
	tagPath := field.GetTagPath(name, true)
	if tagPath == "" {
		return nil, fmt.Errorf("%w: '%s' tag is not set", tinyconf.ErrIncorrectTagSettings, name)
	}
	values, err := d.getValues()
	if err != nil {
		return nil, err
	}
	key := d.key(tagPath)
	raw, ok := values[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s is not found in consul for %s config field", tinyconf.ErrValueNotFound, key, field.GetStructPath())
	}
	value, err := convert.StringToField(raw, field)
	if err != nil {
		return nil, fmt.Errorf("failed to parse consul value from key %s for %s config field: %w", key, field.GetStructPath(), err)
	}
	return &tinyconf.Value{Source: key, Value: value, Warning: tree.ParentTagWarning(field, name, key)}, nil
}

// Watch reports changes of the keys with the prefix, Consul notifies about changes by blocking queries.
func (d *consulDriver) Watch(ctx context.Context, onChange func()) error {
	d.mu.RLock()
	index := d.index
	d.mu.RUnlock()
	return d.client.watch(ctx, d.listPrefix(), true, index, onChange)
}

var plainArg = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

// shellQuote quotes the argument of the shell command if it is needed.
func shellQuote(arg string) string {
	if plainArg.MatchString(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// GenDoc generates the commands of the Consul CLI that put the current values of the fields to Consul KV.
// Values of the fields with hidden:"true" tag are empty.
func (d *consulDriver) GenDoc(registers ...*tinyconf.Registered) string {
	var doc strings.Builder
	seen := make(map[string]bool)
	for _, register := range registers {
		for _, path := range register.Storage.GetAllPaths() {
			fld := register.Storage.MustFind(path)
			tagPath := fld.GetTagPath(name, false)
			if tagPath == "" || !convert.IsValueField(register.Storage, fld) {
				continue
			}
			key := d.key(tagPath)
			if seen[key] {
				continue
			}
			seen[key] = true
			value := ""
			if fld.GetTag().Get("hidden") != "true" {
				value = convert.FieldToString(convert.GetFieldValue(fld, register.Config), fld)
			}
			if text := fld.GetTag().Get("doc"); text != "" {
				doc.WriteString("# " + text + "\n")
			}
			doc.WriteString("consul kv put " + shellQuote(key) + " " + shellQuote(value) + "\n")
		}
	}
	return doc.String()
}

// Document returns the source of the config document stored in Consul KV under the key,
// for the yaml, json and toml drivers:
//
//	yaml.New("config.yaml", yaml.WithSource(consul.Document("app/config")))
//
// Values are read by the tags of the format driver and merged with the files of the driver.
// Absent key is an error, unless the driver has the optional option. Changes of the key are reported for the subscriptions of the manager.
func Document(key string, opts ...Option) tree.Source {
	c := newClient(opts)
	key = strings.Trim(key, "/")
	// index of the document at the moment of the last reading, zero if it has failed
	var index atomic.Uint64
	return tree.Source{
		Name: "consul key " + key,
		Open: func(ctx context.Context) (io.ReadCloser, error) {
			pairs, newIndex, err := c.load(ctx, key, false)
			if err != nil {
				index.Store(0)
				return nil, err
			}
			index.Store(newIndex)
			if len(pairs) == 0 {
				return nil, fs.ErrNotExist
			}
			return io.NopCloser(bytes.NewReader(pairs[0].Value)), nil
		},
		Watch: func(ctx context.Context, onChange func()) error {
			return c.watch(ctx, key, false, index.Load(), onChange)
		},
	}
}
