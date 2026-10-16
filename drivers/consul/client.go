package consul

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultAddress is used if the address is not set by the option and by the CONSUL_HTTP_ADDR variable.
	DefaultAddress = "127.0.0.1:8500"
	// DefaultWaitTime is a maximum time of waiting for changes in one request, the limit of Consul is 10 minutes.
	DefaultWaitTime = 5 * time.Minute
)

var (
	// requestTimeout limits requests that do not wait for changes.
	requestTimeout = 30 * time.Second
	// retryInterval is the first interval of retries of the failed requests while watching, it is doubled after every failure.
	retryInterval = time.Second
	// maxRetryInterval limits the interval of retries.
	maxRetryInterval = 30 * time.Second
)

type options struct {
	address    string
	token      string
	datacenter string
	httpClient *http.Client
	waitTime   time.Duration
}

// Option is an option of the Consul driver and the Consul document source.
type Option func(*options)

// WithAddress sets the address of the Consul agent: host:port, or URL with the scheme, https://consul:8501 for example.
// By default, the address is taken from the CONSUL_HTTP_ADDR variable, 127.0.0.1:8500 if it is not set.
func WithAddress(address string) Option {
	return func(o *options) {
		o.address = address
	}
}

// WithToken sets the ACL token, by default it is taken from the CONSUL_HTTP_TOKEN variable.
func WithToken(token string) Option {
	return func(o *options) {
		o.token = token
	}
}

// WithDatacenter sets the datacenter, the datacenter of the agent is used by default.
func WithDatacenter(datacenter string) Option {
	return func(o *options) {
		o.datacenter = datacenter
	}
}

// WithHTTPClient sets the HTTP client, with TLS settings for example.
// Timeout of the client should be greater than the wait time, otherwise watching for changes fails.
func WithHTTPClient(client *http.Client) Option {
	return func(o *options) {
		o.httpClient = client
	}
}

// WithWaitTime sets the maximum time of waiting for changes in one request, 5 minutes by default.
func WithWaitTime(waitTime time.Duration) Option {
	return func(o *options) {
		o.waitTime = waitTime
	}
}

type client struct {
	options
}

func newClient(opts []Option) *client {
	o := options{
		address:    os.Getenv("CONSUL_HTTP_ADDR"),
		token:      os.Getenv("CONSUL_HTTP_TOKEN"),
		httpClient: http.DefaultClient,
		waitTime:   DefaultWaitTime,
	}
	for _, opt := range opts {
		opt(&o)
	}
	if o.address == "" {
		o.address = DefaultAddress
	}
	if !strings.Contains(o.address, "://") {
		scheme := "http"
		if os.Getenv("CONSUL_HTTP_SSL") == "true" {
			scheme = "https"
		}
		o.address = scheme + "://" + o.address
	}
	o.address = strings.TrimRight(o.address, "/")
	return &client{options: o}
}

type pair struct {
	Key string
	// Value is nil for folders, base64 is decoded by encoding/json
	Value []byte
}

// get returns the pairs by the key, or by the prefix if recurse is true, and the index of the data.
// If index is not zero, the request waits until the index is changed or the wait time is over.
// Absent key is not an error, nil pairs are returned.
func (c *client) get(ctx context.Context, key string, recurse bool, index uint64) ([]pair, uint64, error) {
	query := url.Values{}
	if recurse {
		query.Set("recurse", "true")
	}
	if c.datacenter != "" {
		query.Set("dc", c.datacenter)
	}
	if index > 0 {
		query.Set("index", strconv.FormatUint(index, 10))
		query.Set("wait", c.waitTime.String())
	}
	reqURL := c.address + "/v1/kv/" + (&url.URL{Path: key}).EscapedPath() + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	if c.token != "" {
		req.Header.Set("X-Consul-Token", c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	newIndex, _ := strconv.ParseUint(resp.Header.Get("X-Consul-Index"), 10, 64)
	switch resp.StatusCode {
	case http.StatusOK:
		var pairs []pair
		if err = json.NewDecoder(resp.Body).Decode(&pairs); err != nil {
			return nil, 0, fmt.Errorf("failed to decode response of %s: %w", key, err)
		}
		return pairs, newIndex, nil
	case http.StatusNotFound:
		return nil, newIndex, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, 0, fmt.Errorf("failed to read %s: %s: %s", key, resp.Status, strings.TrimSpace(string(body)))
	}
}

// load returns the pairs by the key or by the prefix without waiting for changes.
func (c *client) load(key string, recurse bool) ([]pair, uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return c.get(ctx, key, recurse, 0)
}

// watch blocks until ctx is done and calls onChange after every change of the data by the key or the prefix.
// index is the index of the loaded data, changes made after the loading are reported too.
// Failed requests are retried, so the temporary unavailability of Consul does not stop watching.
func (c *client) watch(ctx context.Context, key string, recurse bool, index uint64, onChange func()) error {
	// without the loaded data the current index is taken, it is not a change
	known := index > 0
	retry := retryInterval
	for {
		_, newIndex, err := c.get(ctx, key, recurse, index)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(retry):
			}
			if retry *= 2; retry > maxRetryInterval {
				retry = maxRetryInterval
			}
			continue
		}
		retry = retryInterval
		if known && newIndex != index {
			onChange()
		}
		known = true
		// index should be greater than zero to wait for changes
		index = newIndex
		if index == 0 {
			index = 1
		}
	}
}
