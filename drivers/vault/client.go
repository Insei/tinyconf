package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/internal/tree"
)

const (
	// DefaultAddress is used if the address is not set by the option and by the VAULT_ADDR variable.
	DefaultAddress = "https://127.0.0.1:8200"
	// DefaultMount is the mount of the KV secrets engine, the dev server has KV version 2 there.
	DefaultMount = "secret"
	// DefaultWatchInterval is the interval of checking the secrets for changes, Vault does not notify about them.
	DefaultWatchInterval = 30 * time.Second
)

// requestTimeout limits the requests to Vault.
var requestTimeout = 30 * time.Second

type options struct {
	address    string
	token      string
	tokenFile  string
	namespace  string
	mount      string
	kv1        bool
	httpClient *http.Client
	tree       tree.Options
	paths      []string
}

// Option is an option of the Vault driver.
type Option func(*options)

// WithAddress sets the address of Vault: URL with the scheme, https://vault:8200 for example.
// By default, the address is taken from the VAULT_ADDR variable, https://127.0.0.1:8200 if it is not set.
func WithAddress(address string) Option {
	return func(o *options) {
		o.address = address
	}
}

// WithToken sets the token, by default it is taken from the VAULT_TOKEN variable.
func WithToken(token string) Option {
	return func(o *options) {
		o.token = token
	}
}

// WithTokenFile sets the file with the token, it is read before every request,
// so the token renewed by Vault Agent or another sidecar is used without restart.
func WithTokenFile(path string) Option {
	return func(o *options) {
		o.tokenFile = path
	}
}

// WithNamespace sets the namespace of Vault Enterprise, by default it is taken from the VAULT_NAMESPACE variable.
func WithNamespace(namespace string) Option {
	return func(o *options) {
		o.namespace = namespace
	}
}

// WithMount sets the mount of the KV secrets engine, secret by default.
func WithMount(mount string) Option {
	return func(o *options) {
		o.mount = mount
	}
}

// WithKVv1 reads the secrets from the KV secrets engine version 1, version 2 is used by default.
func WithKVv1() Option {
	return func(o *options) {
		o.kv1 = true
	}
}

// WithHTTPClient sets the HTTP client, with TLS settings for example.
func WithHTTPClient(client *http.Client) Option {
	return func(o *options) {
		o.httpClient = client
	}
}

// WithPaths adds the paths of the secrets. Secrets are merged in the order of addition,
// values from the next secrets override values from the previous, nested objects are merged by keys.
func WithPaths(paths ...string) Option {
	return func(o *options) {
		o.paths = append(o.paths, paths...)
	}
}

// WithOptional allows the secrets to be absent, by default absent secret is an error.
func WithOptional() Option {
	return func(o *options) {
		o.tree.Optional = true
	}
}

// WithWatchInterval sets the interval of checking the secrets for changes while there are subscriptions, 30 seconds by default.
func WithWatchInterval(interval time.Duration) Option {
	return func(o *options) {
		o.tree.WatchInterval = interval
	}
}

type client struct {
	options
}

func newClient(o options) *client {
	if o.address == "" {
		o.address = os.Getenv("VAULT_ADDR")
	}
	if o.address == "" {
		o.address = DefaultAddress
	}
	o.address = strings.TrimRight(o.address, "/")
	if o.token == "" && o.tokenFile == "" {
		o.token = os.Getenv("VAULT_TOKEN")
	}
	if o.namespace == "" {
		o.namespace = os.Getenv("VAULT_NAMESPACE")
	}
	if o.mount == "" {
		o.mount = DefaultMount
	}
	o.mount = strings.Trim(o.mount, "/")
	if o.httpClient == nil {
		o.httpClient = http.DefaultClient
	}
	return &client{options: o}
}

// getToken returns the token, the file with the token is read every time.
func (c *client) getToken() (string, error) {
	if c.tokenFile == "" {
		return c.token, nil
	}
	data, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return "", fmt.Errorf("failed to read the token: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// secretURL returns the URL of the secret in the KV secrets engine.
func (c *client) secretURL(path string) string {
	full := c.mount + "/data/" + path
	if c.kv1 {
		full = c.mount + "/" + path
	}
	return c.address + "/v1/" + (&url.URL{Path: full}).EscapedPath()
}

// read returns the data of the secret and its version: the number of the version for KV version 2,
// the hash of the data for KV version 1. Absent secret is fs.ErrNotExist,
// the token without the permission to read the secret is tinyconf.ErrAccessDenied.
func (c *client) read(path string) (map[string]any, string, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.secretURL(path), nil)
	if err != nil {
		return nil, "", err
	}
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	if c.namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.namespace)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", err
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, "", fmt.Errorf("secret %s is not found: %w", path, fs.ErrNotExist)
	case http.StatusForbidden:
		return nil, "", fmt.Errorf("%w: failed to read secret %s: %s", tinyconf.ErrAccessDenied, path, vaultErrors(body))
	default:
		return nil, "", fmt.Errorf("failed to read secret %s: %s: %s", path, resp.Status, vaultErrors(body))
	}
	if c.kv1 {
		var secret struct {
			Data map[string]any `json:"data"`
		}
		if err = json.Unmarshal(body, &secret); err != nil {
			return nil, "", fmt.Errorf("failed to decode secret %s: %w", path, err)
		}
		// the body has the id of the request, only the data is the version; keys of the maps are sorted by json
		data, _ := json.Marshal(secret.Data)
		hash := sha256.Sum256(data)
		return secret.Data, hex.EncodeToString(hash[:]), nil
	}
	var secret struct {
		Data struct {
			Data     map[string]any `json:"data"`
			Metadata struct {
				Version int `json:"version"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if err = json.Unmarshal(body, &secret); err != nil {
		return nil, "", fmt.Errorf("failed to decode secret %s: %w", path, err)
	}
	return secret.Data.Data, strconv.Itoa(secret.Data.Metadata.Version), nil
}

// vaultErrors returns the errors from the body of the response of Vault, or the body itself.
func vaultErrors(body []byte) string {
	var resp struct {
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && len(resp.Errors) > 0 {
		return strings.Join(resp.Errors, ", ")
	}
	return strings.TrimSpace(string(body))
}
