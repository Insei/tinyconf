// Package vault contains the driver that reads config values from the KV secrets engine of HashiCorp Vault.
package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"strings"

	"github.com/insei/fmap/v3"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/convert"
	"github.com/insei/tinyconf/drivers/internal/tree"
)

const name = "vault"

type vaultDriver struct {
	*tree.Driver
	client *client
	// paths of the secrets in the order of merging
	paths []string
}

// decode decodes the data of the secret, it is encoded by the source from the response of Vault, so it is always correct.
func decode(data []byte) (map[string]any, error) {
	val, _ := convert.DecodeJSON(data)
	secret, _ := val.(map[string]any)
	return secret, nil
}

// source returns the source of the config content from the secret, the version of the secret is its state for watching.
func (c *client) source(path string) tree.Source {
	return tree.Source{
		Name: "vault secret " + c.mount + "/" + path,
		Open: func() (io.ReadCloser, error) {
			secret, _, err := c.read(path)
			if err != nil {
				return nil, err
			}
			// the data is decoded from JSON, so it is always encoded back
			data, _ := json.Marshal(secret)
			return io.NopCloser(bytes.NewReader(data)), nil
		},
		Stat: func() string {
			_, version, err := c.read(path)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return "absent"
			case err != nil:
				// the change is reported when Vault is available again
				return "error: " + err.Error()
			}
			return version
		},
	}
}

// New creates new Vault driver that reads the secret by the path from the KV secrets engine.
// Values are read by the vault tags, the tags of the parents are the keys of the nested objects:
// vault:"password" inside vault:"db" is read from the db.password of the secret.
// The path can be empty if the paths are set by WithPaths.
//
// Vault does not notify about changes, so the secrets are checked with the watch interval while there are subscriptions.
func New(path string, opts ...Option) (tinyconf.Driver, error) {
	o := options{tree: tree.Options{WatchInterval: DefaultWatchInterval}}
	for _, opt := range opts {
		opt(&o)
	}
	c := newClient(o)
	var paths []string
	for _, p := range append([]string{path}, o.paths...) {
		if p = strings.Trim(p, "/"); p != "" {
			paths = append(paths, p)
			o.tree.AddSource(c.source(p))
		}
	}
	return &vaultDriver{Driver: o.tree.NewDriver(name, decode), client: c, paths: paths}, nil
}

var plainArg = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

// shellQuote quotes the argument of the shell command if it is needed.
func shellQuote(arg string) string {
	if plainArg.MatchString(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// GenDoc generates the command of the Vault CLI that puts the current values of the fields to the first secret.
// Values of the fields with hidden:"true" tag are empty strings.
func (d *vaultDriver) GenDoc(registers ...*tinyconf.Registered) string {
	doc := tree.Doc(registers, name, func(v any, _ fmap.Field) any {
		return convert.ToTree(v, name)
	})
	if len(doc) == 0 || len(d.paths) == 0 {
		return ""
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return ""
	}
	return fmt.Sprintf("vault kv put -mount=%s %s - <<'EOF'\n%s\nEOF\n", shellQuote(d.client.mount), shellQuote(d.paths[0]), data)
}
