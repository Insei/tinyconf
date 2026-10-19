package vault

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/insei/tinyconf/v2"
	"github.com/insei/tinyconf/v2/drivers/internal/tree"
)

const (
	// DefaultAddress is used if the address is not set by the option and by the VAULT_ADDR variable.
	DefaultAddress = "https://127.0.0.1:8200"
	// DefaultMount is the mount of the KV secrets engine, the dev server has KV version 2 there.
	DefaultMount = "secret"
	// DefaultWatchInterval is the interval of checking the secrets for changes, Vault does not notify about them.
	DefaultWatchInterval = 30 * time.Second
	// KubernetesTokenFile is the token of the service account of the pod, it is used by WithKubernetes by default.
	KubernetesTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

// requestTimeout limits the requests to Vault.
var requestTimeout = 30 * time.Second

type options struct {
	address    string
	token      string
	tokenFile  string
	login      *login
	authMount  string
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

// login is the auth method that gets the token from Vault.
type login struct {
	mount string
	// data returns the body of the login request, it is called before every login
	data func() (map[string]any, error)
}

// WithAppRole gets the token by the AppRole auth method with the role ID and the secret ID,
// the auth method is mounted at approle by default, it is changed by WithAuthMount.
// The token is requested again before its lease expires. It has priority over the token from the other options.
func WithAppRole(roleID, secretID string) Option {
	return func(o *options) {
		o.login = &login{mount: "approle", data: func() (map[string]any, error) {
			return map[string]any{"role_id": roleID, "secret_id": secretID}, nil
		}}
	}
}

// WithKubernetes gets the token by the Kubernetes auth method with the role and the token of the service account
// from the tokenFile, KubernetesTokenFile if it is empty. The file is read before every login, so the rotated token is used.
// The auth method is mounted at kubernetes by default, it is changed by WithAuthMount.
// The token of Vault is requested again before its lease expires. It has priority over the token from the other options.
func WithKubernetes(role, tokenFile string) Option {
	if tokenFile == "" {
		tokenFile = KubernetesTokenFile
	}
	return func(o *options) {
		o.login = &login{mount: "kubernetes", data: func() (map[string]any, error) {
			jwt, err := os.ReadFile(tokenFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read the token of the service account: %w", err)
			}
			return map[string]any{"role": role, "jwt": strings.TrimSpace(string(jwt))}, nil
		}}
	}
}

// WithAuthMount sets the mount of the auth method of WithAppRole or WithKubernetes.
func WithAuthMount(mount string) Option {
	return func(o *options) {
		o.authMount = strings.Trim(mount, "/")
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

	// token of the login and the time when it should be requested again, zero if it does not expire
	mu         sync.Mutex
	loginToken string
	renewAt    time.Time
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
	if o.login != nil && o.authMount != "" {
		o.login = &login{mount: o.authMount, data: o.login.data}
	}
	return &client{options: o}
}

// getToken returns the token: the token of the login, the file with the token is read every time.
func (c *client) getToken(ctx context.Context) (string, error) {
	if c.login != nil {
		return c.loginTokenFor(ctx)
	}
	if c.tokenFile == "" {
		return c.token, nil
	}
	data, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return "", fmt.Errorf("failed to read the token: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// loginTokenFor returns the token of the login, the new token is requested after 2/3 of the lease of the previous one.
func (c *client) loginTokenFor(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loginToken != "" && (c.renewAt.IsZero() || time.Now().Before(c.renewAt)) {
		return c.loginToken, nil
	}
	data, err := c.login.data()
	if err != nil {
		return "", err
	}
	// the data is the strings, so it is always encoded
	body, _ := json.Marshal(data)
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.address+"/v1/auth/"+c.login.mount+"/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	resp, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("failed to login by auth/%s: %w", c.login.mount, err)
	}
	switch resp.status {
	case http.StatusOK:
	case http.StatusBadRequest, http.StatusForbidden:
		// the wrong role or credentials
		return "", fmt.Errorf("%w: failed to login by auth/%s: %s", tinyconf.ErrAccessDenied, c.login.mount, vaultErrors(resp.body))
	default:
		return "", fmt.Errorf("failed to login by auth/%s: %s: %s", c.login.mount, resp.statusText, vaultErrors(resp.body))
	}
	var auth struct {
		Auth struct {
			ClientToken   string `json:"client_token"`
			LeaseDuration int    `json:"lease_duration"`
		} `json:"auth"`
	}
	if err = json.Unmarshal(resp.body, &auth); err != nil || auth.Auth.ClientToken == "" {
		return "", fmt.Errorf("failed to login by auth/%s: no token in the response", c.login.mount)
	}
	c.loginToken, c.renewAt = auth.Auth.ClientToken, time.Time{}
	if lease := time.Duration(auth.Auth.LeaseDuration) * time.Second; lease > 0 {
		c.renewAt = time.Now().Add(lease * 2 / 3)
	}
	return c.loginToken, nil
}

// resetLogin forgets the token of the login, the token is requested again by the next request:
// the token can be revoked before its lease expires.
func (c *client) resetLogin(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loginToken == token {
		c.loginToken = ""
	}
}

// response is the read response of Vault.
type response struct {
	status     int
	statusText string
	body       []byte
}

// do sends the request with the namespace and reads the response.
func (c *client) do(req *http.Request) (*response, error) {
	if c.namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.namespace)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	return &response{status: resp.StatusCode, statusText: resp.Status, body: body}, nil
}

// secretURL returns the URL of the secret in the KV secrets engine.
func (c *client) secretURL(path string) string {
	full := c.mount + "/data/" + path
	if c.kv1 {
		full = c.mount + "/" + path
	}
	return c.address + "/v1/" + (&url.URL{Path: full}).EscapedPath()
}

// readBody requests the secret and returns the token of the request and the body of the response.
func (c *client) readBody(ctx context.Context, path string) (string, []byte, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return "", nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.secretURL(path), nil)
	if err != nil {
		return token, nil, err
	}
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	resp, err := c.do(req)
	if err != nil {
		return token, nil, err
	}
	switch resp.status {
	case http.StatusOK:
		return token, resp.body, nil
	case http.StatusNotFound:
		return token, nil, fmt.Errorf("secret %s is not found: %w", path, fs.ErrNotExist)
	case http.StatusForbidden:
		return token, nil, fmt.Errorf("%w: failed to read secret %s: %s", tinyconf.ErrAccessDenied, path, vaultErrors(resp.body))
	default:
		return token, nil, fmt.Errorf("failed to read secret %s: %s: %s", path, resp.statusText, vaultErrors(resp.body))
	}
}

// read returns the data of the secret and its version: the number of the version for KV version 2,
// the hash of the data for KV version 1. Absent secret is fs.ErrNotExist,
// the token without the permission to read the secret is tinyconf.ErrAccessDenied.
// The secret is read again with the new token of the login, if the token is denied.
func (c *client) read(ctx context.Context, path string) (map[string]any, string, error) {
	token, body, err := c.readBody(ctx, path)
	if c.login != nil && token != "" && errors.Is(err, tinyconf.ErrAccessDenied) {
		c.resetLogin(token)
		_, body, err = c.readBody(ctx, path)
	}
	if err != nil {
		return nil, "", err
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
