package etcd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/insei/tinyconf/v2"
)

// DefaultEndpoint is used if the endpoints are not set by the option and by the ETCDCTL_ENDPOINTS variable.
const DefaultEndpoint = "http://127.0.0.1:2379"

var (
	// requestTimeout limits requests that do not wait for changes.
	requestTimeout = 30 * time.Second
	// retryInterval is the first interval of retries of the failed watching, it is doubled after every failure.
	retryInterval = time.Second
	// maxRetryInterval limits the interval of retries.
	maxRetryInterval = 30 * time.Second
	// errCompacted is returned by watching if the history of the changes since the known revision is compacted.
	errCompacted = errors.New("history of the changes is compacted")
)

type options struct {
	endpoints  []string
	username   string
	password   string
	httpClient *http.Client
}

// Option is an option of the etcd driver and the etcd document source.
type Option func(*options)

// WithEndpoints sets the endpoints of the etcd cluster: host:port, or URL with the scheme, https://etcd:2379 for example.
// Requests go to the next endpoint if the current one is not available.
// By default, the endpoints are taken from the ETCDCTL_ENDPOINTS variable, comma separated, http://127.0.0.1:2379 if it is not set.
func WithEndpoints(endpoints ...string) Option {
	return func(o *options) {
		o.endpoints = endpoints
	}
}

// WithAuth sets the user and the password for the clusters with authentication,
// by default they are taken from the ETCDCTL_USER (user or user:password) and ETCDCTL_PASSWORD variables.
func WithAuth(username, password string) Option {
	return func(o *options) {
		o.username, o.password = username, password
	}
}

// WithHTTPClient sets the HTTP client, with TLS settings and client certificates for example.
func WithHTTPClient(client *http.Client) Option {
	return func(o *options) {
		o.httpClient = client
	}
}

type client struct {
	options

	mu sync.Mutex
	// token of the authenticated user
	token string
	// current is the index of the endpoint that has answered the last time
	current int
}

func newClient(opts []Option) *client {
	o := options{}
	if endpoints := os.Getenv("ETCDCTL_ENDPOINTS"); endpoints != "" {
		o.endpoints = strings.Split(endpoints, ",")
	}
	if user := os.Getenv("ETCDCTL_USER"); user != "" {
		o.username, o.password, _ = strings.Cut(user, ":")
		if password := os.Getenv("ETCDCTL_PASSWORD"); password != "" {
			o.password = password
		}
	}
	for _, opt := range opts {
		opt(&o)
	}
	var endpoints []string
	for _, endpoint := range o.endpoints {
		if endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/"); endpoint == "" {
			continue
		}
		if !strings.Contains(endpoint, "://") {
			endpoint = "http://" + endpoint
		}
		endpoints = append(endpoints, endpoint)
	}
	if len(endpoints) == 0 {
		endpoints = []string{DefaultEndpoint}
	}
	o.endpoints = endpoints
	if o.httpClient == nil {
		o.httpClient = http.DefaultClient
	}
	return &client{options: o}
}

// gatewayError is the error of the gRPC gateway of etcd.
type gatewayError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// isAccessDenied reports whether the error of etcd is about the permissions or the authentication.
func isAccessDenied(code int, message string) bool {
	const permissionDenied, unauthenticated = 7, 16
	return code == permissionDenied || code == unauthenticated ||
		strings.Contains(message, "user name is empty") || strings.Contains(message, "authentication failed") ||
		strings.Contains(message, "PermissionDenied") || strings.Contains(message, "invalid auth token")
}

// isInvalidToken reports whether the token is expired or invalid, the user should be authenticated again.
func isInvalidToken(message string) bool {
	return strings.Contains(message, "invalid auth token")
}

// errorOf returns the error of the request by the error of etcd.
func errorOf(action, message string, code int) error {
	if isAccessDenied(code, message) {
		return fmt.Errorf("%w: failed to %s: %s", tinyconf.ErrAccessDenied, action, message)
	}
	return fmt.Errorf("failed to %s: %s", action, message)
}

// do sends the request to the endpoints of the cluster, starting from the one that has answered the last time,
// and returns the response with 200 status, the body of the response should be closed.
func (c *client) do(ctx context.Context, path string, body any, authenticated bool) (*http.Response, error) {
	// bodies are maps of strings and bytes, they are always encoded
	data, _ := json.Marshal(body)
	c.mu.Lock()
	first, token := c.current, c.token
	c.mu.Unlock()
	var lastErr error
	for i := range c.endpoints {
		idx := (first + i) % len(c.endpoints)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoints[idx]+path, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if authenticated && token != "" {
			req.Header.Set("Authorization", token)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			// the endpoint is not available, the next one is tried
			lastErr = err
			if ctx.Err() != nil {
				return nil, err
			}
			continue
		}
		c.mu.Lock()
		c.current = idx
		c.mu.Unlock()
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		defer resp.Body.Close()
		var gwErr gatewayError
		respData, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if json.Unmarshal(respData, &gwErr) != nil || gwErr.Message == "" {
			gwErr.Message = resp.Status + ": " + strings.TrimSpace(string(respData))
		}
		return nil, &requestError{gatewayError: gwErr}
	}
	return nil, lastErr
}

// requestError is the error of etcd for the request.
type requestError struct {
	gatewayError
}

func (e *requestError) Error() string {
	return e.Message
}

// call sends the request with the authentication and decodes the response.
// The user is authenticated before the first request and again when the token is expired.
func (c *client) call(ctx context.Context, path string, body, resp any) error {
	for attempt := 0; ; attempt++ {
		if err := c.authenticate(ctx, false); err != nil {
			return err
		}
		httpResp, err := c.do(ctx, path, body, true)
		var reqErr *requestError
		if errors.As(err, &reqErr) && isInvalidToken(reqErr.Message) && c.username != "" && attempt == 0 {
			// the token is expired
			if err = c.authenticate(ctx, true); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		defer httpResp.Body.Close()
		return json.NewDecoder(httpResp.Body).Decode(resp)
	}
}

// authenticate gets the token of the user, if the user is set and there is no token or the token is expired.
func (c *client) authenticate(ctx context.Context, expired bool) error {
	if c.username == "" {
		return nil
	}
	c.mu.Lock()
	hasToken := c.token != ""
	c.mu.Unlock()
	if hasToken && !expired {
		return nil
	}
	resp, err := c.do(ctx, "/v3/auth/authenticate", map[string]string{"name": c.username, "password": c.password}, false)
	var reqErr *requestError
	if errors.As(err, &reqErr) {
		return errorOf("authenticate user "+c.username, reqErr.Message, reqErr.Code)
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var auth struct {
		Token string `json:"token"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&auth); err != nil {
		return fmt.Errorf("failed to decode the token: %w", err)
	}
	c.mu.Lock()
	c.token = auth.Token
	c.mu.Unlock()
	return nil
}

type keyValue struct {
	// Key and Value are base64, it is decoded by encoding/json
	Key   []byte `json:"key"`
	Value []byte `json:"value"`
}

type header struct {
	Revision string `json:"revision"`
}

// rangeOf returns the key and the end of the range for the request of the key or all keys with the prefix.
func rangeOf(key string, prefix bool) ([]byte, []byte) {
	if !prefix {
		return []byte(key), nil
	}
	if key == "" {
		// all keys
		return []byte{0}, []byte{0}
	}
	end := []byte(key)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] < 0xff {
			end[i]++
			return []byte(key), end[:i+1]
		}
	}
	// all keys after the prefix of 0xff bytes
	return []byte(key), []byte{0}
}

// get returns the values by the key, or by the prefix if prefix is true, and the revision of the cluster.
// Absent key is not an error, nil values are returned.
func (c *client) get(ctx context.Context, key string, prefix bool) ([]keyValue, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	start, end := rangeOf(key, prefix)
	var resp struct {
		Header header     `json:"header"`
		Kvs    []keyValue `json:"kvs"`
	}
	err := c.call(ctx, "/v3/kv/range", map[string][]byte{"key": start, "range_end": end}, &resp)
	var reqErr *requestError
	if errors.As(err, &reqErr) {
		return nil, 0, errorOf("read "+key, reqErr.Message, reqErr.Code)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read %s: %w", key, err)
	}
	revision, _ := strconv.ParseInt(resp.Header.Revision, 10, 64)
	return resp.Kvs, revision, nil
}

// watch blocks until ctx is done and calls onChange after every change of the data by the key or the prefix.
// revision is the revision of the loaded data, changes made after it are reported too.
// Zero revision means that the data is not loaded or the last loading has failed, the change is reported at once.
// Failed watching is retried from the known revision, so no changes are missed; if the history is compacted,
// the change is reported. Access denied stops watching with tinyconf.ErrAccessDenied, it is not fixed by retries.
func (c *client) watch(ctx context.Context, key string, prefix bool, revision int64, onChange func()) error {
	failed := revision == 0
	retry := retryInterval
	for {
		var err error
		if failed {
			// the current state is taken, it is a change
			if _, revision, err = c.get(ctx, key, prefix); err == nil {
				failed = false
				onChange()
			}
		}
		if err == nil {
			err = c.watchStream(ctx, key, prefix, &revision, onChange)
		}
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, tinyconf.ErrAccessDenied) {
			return err
		}
		if errors.Is(err, errCompacted) {
			failed = true
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(retry):
		}
		if retry *= 2; retry > maxRetryInterval {
			retry = maxRetryInterval
		}
	}
}

// watchStream watches for changes after the revision until the stream is closed or ctx is done,
// the revision is updated after every change.
func (c *client) watchStream(ctx context.Context, key string, prefix bool, revision *int64, onChange func()) error {
	if err := c.authenticate(ctx, false); err != nil {
		return err
	}
	start, end := rangeOf(key, prefix)
	req := map[string]any{"create_request": map[string]any{
		"key": start, "range_end": end, "start_revision": strconv.FormatInt(*revision+1, 10),
	}}
	resp, err := c.do(ctx, "/v3/watch", req, true)
	var reqErr *requestError
	if errors.As(err, &reqErr) {
		return errorOf("watch "+key, reqErr.Message, reqErr.Code)
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)
	for {
		var msg struct {
			Result struct {
				Header          header     `json:"header"`
				Canceled        bool       `json:"canceled"`
				CancelReason    string     `json:"cancel_reason"`
				CompactRevision string     `json:"compact_revision"`
				Events          []struct{} `json:"events"`
			} `json:"result"`
			Error *gatewayError `json:"error"`
		}
		if err = decoder.Decode(&msg); err != nil {
			return fmt.Errorf("watching of %s is stopped: %w", key, err)
		}
		if msg.Error != nil {
			return errorOf("watch "+key, msg.Error.Message, msg.Error.Code)
		}
		result := msg.Result
		if result.CompactRevision != "" && result.CompactRevision != "0" {
			return errCompacted
		}
		if result.Canceled {
			if isInvalidToken(result.CancelReason) && c.username != "" {
				// the token is expired, it is taken again before the next watching
				c.mu.Lock()
				c.token = ""
				c.mu.Unlock()
				return fmt.Errorf("watching of %s is stopped: %s", key, result.CancelReason)
			}
			return errorOf("watch "+key, result.CancelReason, 0)
		}
		if len(result.Events) > 0 {
			*revision, _ = strconv.ParseInt(result.Header.Revision, 10, 64)
			onChange()
		}
	}
}
