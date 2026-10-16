package consul

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// fakeConsul is a minimal Consul KV HTTP API: reading by key and prefix, blocking queries, ACL token.
type fakeConsul struct {
	*httptest.Server

	mu      sync.Mutex
	changed chan struct{}
	index   uint64
	values  map[string]*string
	// modify indexes of the keys, deleted keys are kept like tombstones in Consul
	modify map[string]uint64
	token  string
	// denied are the prefixes of the keys that the token can not read, like ACL rules of Consul
	denied []string
	// fails is a number of the next requests that fail with 500 status
	fails    int
	requests []*http.Request
}

func newFakeConsul(values map[string]string) *fakeConsul {
	f := &fakeConsul{changed: make(chan struct{}), index: 1, values: map[string]*string{}, modify: map[string]uint64{}}
	for key, value := range values {
		value := value
		f.values[key] = &value
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

// put sets the value, nil deletes the key.
func (f *fakeConsul) put(key string, value *string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if value == nil {
		delete(f.values, key)
	} else {
		f.values[key] = value
	}
	f.index++
	f.modify[key] = f.index
	close(f.changed)
	f.changed = make(chan struct{})
}

func (f *fakeConsul) setFails(fails int) {
	f.mu.Lock()
	f.fails = fails
	f.mu.Unlock()
}

func (f *fakeConsul) lastRequest() *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func (f *fakeConsul) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r)
	if f.fails > 0 {
		f.fails--
		f.mu.Unlock()
		http.Error(w, "unavailable", http.StatusInternalServerError)
		return
	}
	if f.token != "" && r.Header.Get("X-Consul-Token") != f.token {
		f.mu.Unlock()
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}
	query := r.URL.Query()
	key := strings.TrimPrefix(r.URL.Path, "/v1/kv/")
	recurse := query.Get("recurse") == "true"
	// permissions are checked before waiting for changes
	if !recurse && f.isDenied(key) {
		f.mu.Unlock()
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}
	if index, _ := strconv.ParseUint(query.Get("index"), 10, 64); index > 0 {
		wait, _ := time.ParseDuration(query.Get("wait"))
		timeout := time.After(wait)
		for expired := false; !expired && f.queryIndex(key, recurse) <= index; {
			changed := f.changed
			f.mu.Unlock()
			select {
			case <-changed:
			case <-timeout:
				expired = true
			case <-r.Context().Done():
				return
			}
			f.mu.Lock()
		}
	}
	defer f.mu.Unlock()
	if recurse && f.isDenied(key) {
		// all keys are hidden, Consul answers as if there are no keys
		w.WriteHeader(http.StatusNotFound)
		return
	}
	type pair struct {
		Key   string
		Value *[]byte
	}
	var pairs []pair
	for k, v := range f.values {
		if k == key || (recurse && strings.HasPrefix(k, key)) {
			if f.isDenied(k) {
				w.Header().Set("X-Consul-Results-Filtered-By-ACLs", "true")
				continue
			}
			p := pair{Key: k}
			if v != nil {
				data := []byte(*v)
				p.Value = &data
			}
			pairs = append(pairs, p)
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Key < pairs[j].Key })
	w.Header().Set("X-Consul-Index", strconv.FormatUint(f.queryIndex(key, recurse), 10))
	if len(pairs) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(pairs)
}

// queryIndex returns the maximum modify index of the keys of the query,
// or the index of all keys if they have not been changed, like Consul does.
func (f *fakeConsul) queryIndex(key string, recurse bool) uint64 {
	var index uint64
	for k, modify := range f.modify {
		if (k == key || (recurse && strings.HasPrefix(k, key))) && modify > index {
			index = modify
		}
	}
	if index == 0 {
		return f.index
	}
	return index
}

func (f *fakeConsul) isDenied(key string) bool {
	for _, prefix := range f.denied {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
