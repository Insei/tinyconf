package consul

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/insei/tinyconf"
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

// response is the result of reading the keys.
type response struct {
	pairs []pair
	// index of the data for waiting for changes
	index uint64
	// filtered is true if some keys are hidden by ACL, the token has no permission to read them
	filtered bool
}

// get returns the pairs by the key, or by the prefix if recurse is true.
// If index is not zero, the request waits until the index is changed or the wait time is over.
// Absent key is not an error, nil pairs are returned.
func (c *client) get(ctx context.Context, key string, recurse bool, index uint64) (response, error) {
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
		return response{}, err
	}
	if c.token != "" {
		req.Header.Set("X-Consul-Token", c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return response{}, err
	}
	defer resp.Body.Close()
	result := response{filtered: resp.Header.Get("X-Consul-Results-Filtered-By-ACLs") == "true"}
	result.index, _ = strconv.ParseUint(resp.Header.Get("X-Consul-Index"), 10, 64)
	switch resp.StatusCode {
	case http.StatusOK:
		if err = json.NewDecoder(resp.Body).Decode(&result.pairs); err != nil {
			return response{}, fmt.Errorf("failed to decode response of %s: %w", key, err)
		}
		return result, nil
	case http.StatusNotFound:
		return result, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		err = fmt.Errorf("failed to read %s: %s: %s", key, resp.Status, strings.TrimSpace(string(body)))
		if resp.StatusCode == http.StatusForbidden {
			err = fmt.Errorf("%w: %s", tinyconf.ErrAccessDenied, err)
		}
		return response{}, err
	}
}

// load returns the pairs by the key or by the prefix without waiting for changes.
// Keys hidden by ACL are an error: Consul answers as if they are absent, so the config would be silently incomplete.
func (c *client) load(key string, recurse bool) ([]pair, uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	result, err := c.get(ctx, key, recurse, 0)
	if err != nil {
		return nil, 0, err
	}
	if result.filtered {
		return nil, 0, fmt.Errorf("%w: failed to read %s: some keys are hidden by ACL, the token has no permission to read them", tinyconf.ErrAccessDenied, key)
	}
	if recurse && len(result.pairs) == 0 && key != "" {
		// the empty list is returned without the permission to read the prefix too,
		// the request of the prefix as a key is denied in this case
		if _, err = c.get(ctx, key, false, 0); err != nil {
			return nil, 0, err
		}
	}
	return result.pairs, result.index, nil
}

// watch blocks until ctx is done and calls onChange after every change of the data by the key or the prefix.
// index is the index of the loaded data, changes made after the loading are reported too.
// Failed requests are retried, so the temporary unavailability of Consul does not stop watching.
// After the failure the change is always reported: the data could be changed while Consul was not available,
// and the indexes of Consul that has lost its data can be the same as the known ones.
// Access denied stops watching with tinyconf.ErrAccessDenied, it is not fixed by retries.
func (c *client) watch(ctx context.Context, key string, recurse bool, index uint64, onChange func()) error {
	// without the loaded data the current index is taken, it is not a change
	known := index > 0
	failed := false
	retry := retryInterval
	for {
		waitIndex := index
		if failed {
			// the current state is taken without waiting
			waitIndex = 0
		}
		result, err := c.get(ctx, key, recurse, waitIndex)
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, tinyconf.ErrAccessDenied) {
			return err
		}
		if err != nil {
			failed = true
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
		if known && (failed || result.index != index) {
			onChange()
		}
		known, failed = true, false
		// index should be greater than zero to wait for changes
		index = result.index
		if index == 0 {
			index = 1
		}
	}
}
