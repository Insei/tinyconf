// Package etcd contains the driver that reads config values from etcd, every field from its own key,
// and the source of the config document stored in etcd for the yaml, json and toml drivers.
package etcd

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

	"github.com/insei/tinyconf/v2"
	"github.com/insei/tinyconf/v2/convert"
	"github.com/insei/tinyconf/v2/drivers/internal/tree"
)

const name = "etcd"

type etcdDriver struct {
	prefix string
	client *client

	mu     sync.RWMutex
	loaded bool
	values map[string]string
	// revision of the values at the moment of the last Load, zero if it has failed
	revision int64
}

// New creates new etcd driver. Every field is read from its own key: the prefix and the path by the etcd tags,
// with dots replaced by slashes, so etcd:"port" inside etcd:"http" is read from prefix/http/port.
// Values are parsed like environment variables: slices are comma separated, structs in slices are JSON.
func New(prefix string, opts ...Option) (tinyconf.Driver, error) {
	return &etcdDriver{prefix: strings.Trim(prefix, "/"), client: newClient(opts)}, nil
}

func (d *etcdDriver) GetName() string {
	return name
}

// listPrefix returns the prefix of all keys of the driver.
func (d *etcdDriver) listPrefix() string {
	if d.prefix == "" {
		return ""
	}
	return d.prefix + "/"
}

func (d *etcdDriver) key(tagPath string) string {
	return d.listPrefix() + strings.ReplaceAll(tagPath, ".", "/")
}

// Load reads all keys with the prefix by one request.
func (d *etcdDriver) Load() error {
	return d.LoadContext(context.Background())
}

// LoadContext is Load with the context of the request, Manager calls it with the context of Parse.
func (d *etcdDriver) LoadContext(ctx context.Context) error {
	kvs, revision, err := d.client.get(ctx, d.listPrefix(), true)
	if err != nil {
		// the values from etcd are not in the config, Watch reports the change as soon as etcd is available
		d.mu.Lock()
		d.revision = 0
		d.mu.Unlock()
		return err
	}
	values := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		values[string(kv.Key)] = string(kv.Value)
	}
	d.mu.Lock()
	d.values, d.loaded, d.revision = values, true, revision
	d.mu.Unlock()
	return nil
}

func (d *etcdDriver) getValues() (map[string]string, error) {
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

func (d *etcdDriver) GetValue(field fmap.Field) (*tinyconf.Value, error) {
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
		return nil, fmt.Errorf("%w: %s is not found in etcd for %s config field", tinyconf.ErrValueNotFound, key, field.GetStructPath())
	}
	value, err := convert.StringToField(raw, field)
	if err != nil {
		return nil, fmt.Errorf("failed to parse etcd value from key %s for %s config field: %w", key, field.GetStructPath(), err)
	}
	return &tinyconf.Value{Source: key, Value: value, Warning: tree.ParentTagWarning(field, name, key)}, nil
}

// Watch reports changes of the keys with the prefix, etcd notifies about changes by the stream of the events.
func (d *etcdDriver) Watch(ctx context.Context, onChange func()) error {
	d.mu.RLock()
	revision := d.revision
	d.mu.RUnlock()
	return d.client.watch(ctx, d.listPrefix(), true, revision, onChange)
}

var plainArg = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

// shellQuote quotes the argument of the shell command if it is needed.
func shellQuote(arg string) string {
	if plainArg.MatchString(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// GenDoc generates the commands of etcdctl that put the current values of the fields to etcd.
// Values of the fields with hidden:"true" tag are empty.
func (d *etcdDriver) GenDoc(registers ...*tinyconf.Registered) string {
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
			doc.WriteString("etcdctl put -- " + shellQuote(key) + " " + shellQuote(value) + "\n")
		}
	}
	return doc.String()
}

// Document returns the source of the config document stored in etcd under the key,
// for the yaml, json and toml drivers:
//
//	yaml.New("config.yaml", yaml.WithSource(etcd.Document("app/config")))
//
// Values are read by the tags of the format driver and merged with the files of the driver.
// Absent key is an error, unless the driver has the optional option. Changes of the key are reported for the subscriptions of the manager.
func Document(key string, opts ...Option) tree.Source {
	c := newClient(opts)
	key = strings.Trim(key, "/")
	// revision of the document at the moment of the last reading, zero if it has failed
	var revision atomic.Int64
	return tree.Source{
		Name: "etcd key " + key,
		Open: func(ctx context.Context) (io.ReadCloser, error) {
			kvs, newRevision, err := c.get(ctx, key, false)
			if err != nil {
				revision.Store(0)
				return nil, err
			}
			revision.Store(newRevision)
			if len(kvs) == 0 {
				return nil, fs.ErrNotExist
			}
			return io.NopCloser(bytes.NewReader(kvs[0].Value)), nil
		},
		Watch: func(ctx context.Context, onChange func()) error {
			return c.watch(ctx, key, false, revision.Load(), onChange)
		},
	}
}
