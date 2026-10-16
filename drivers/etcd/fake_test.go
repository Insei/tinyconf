package etcd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"sync"
)

// fakeEtcd is a minimal gRPC gateway of etcd: reading by key and range, watching, authentication.
type fakeEtcd struct {
	*httptest.Server

	mu       sync.Mutex
	changed  chan struct{}
	revision int64
	values   map[string]string
	// modify revisions of the keys, deleted keys are kept for the events
	modify map[string]int64
	// compacted is the revision of the compacted history, older revisions can not be watched
	compacted int64
	// users and their passwords, authentication is enabled if there are users
	users map[string]string
	// tokens are valid tokens of the users
	tokens map[string]bool
	// denied are the keys and the prefixes that can not be read
	denied   []string
	requests []string
}

func newFakeEtcd(values map[string]string) *fakeEtcd {
	f := &fakeEtcd{changed: make(chan struct{}), revision: 1, values: map[string]string{}, modify: map[string]int64{},
		tokens: map[string]bool{}}
	for key, value := range values {
		f.values[key] = value
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

// put sets the value, nil deletes the key.
func (f *fakeEtcd) put(key string, value *string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if value == nil {
		delete(f.values, key)
	} else {
		f.values[key] = *value
	}
	f.revision++
	f.modify[key] = f.revision
	close(f.changed)
	f.changed = make(chan struct{})
}

func (f *fakeEtcd) set(fn func(f *fakeEtcd)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeEtcd) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.requests...)
}

func writeError(w http.ResponseWriter, status, code int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message, "code": code, "message": message})
}

func inRange(key, start, end []byte) bool {
	if len(end) == 0 {
		return bytes.Equal(key, start)
	}
	return bytes.Compare(key, start) >= 0 && (bytes.Equal(end, []byte{0}) || bytes.Compare(key, end) < 0)
}

// check checks the token and the permissions of the request, it returns false if the error is written.
func (f *fakeEtcd) check(w http.ResponseWriter, r *http.Request, start, end []byte) bool {
	if len(f.users) > 0 {
		token := r.Header.Get("Authorization")
		if token == "" {
			writeError(w, http.StatusBadRequest, 3, "etcdserver: user name is empty")
			return false
		}
		if !f.tokens[token] {
			writeError(w, http.StatusUnauthorized, 16, "etcdserver: invalid auth token")
			return false
		}
	}
	for _, denied := range f.denied {
		if inRange([]byte(denied), start, end) || bytes.HasPrefix(start, []byte(denied)) {
			writeError(w, http.StatusForbidden, 7, "etcdserver: permission denied")
			return false
		}
	}
	return true
}

func (f *fakeEtcd) handle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key           []byte `json:"key"`
		RangeEnd      []byte `json:"range_end"`
		Name          string `json:"name"`
		Password      string `json:"password"`
		CreateRequest struct {
			Key           []byte `json:"key"`
			RangeEnd      []byte `json:"range_end"`
			StartRevision string `json:"start_revision"`
		} `json:"create_request"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.Path)
	switch r.URL.Path {
	case "/v3/auth/authenticate":
		defer f.mu.Unlock()
		if len(f.users) == 0 {
			writeError(w, http.StatusBadRequest, 9, "etcdserver: authentication is not enabled")
			return
		}
		if password, ok := f.users[req.Name]; !ok || password != req.Password {
			writeError(w, http.StatusBadRequest, 3, "etcdserver: authentication failed, invalid user ID or password")
			return
		}
		token := req.Name + "." + strconv.Itoa(len(f.tokens))
		f.tokens[token] = true
		_ = json.NewEncoder(w).Encode(map[string]any{"token": token})
	case "/v3/kv/range":
		defer f.mu.Unlock()
		if !f.check(w, r, req.Key, req.RangeEnd) {
			return
		}
		type kv struct {
			Key   []byte `json:"key"`
			Value []byte `json:"value"`
		}
		var kvs []kv
		for key, value := range f.values {
			if inRange([]byte(key), req.Key, req.RangeEnd) {
				kvs = append(kvs, kv{Key: []byte(key), Value: []byte(value)})
			}
		}
		sort.Slice(kvs, func(i, j int) bool { return bytes.Compare(kvs[i].Key, kvs[j].Key) < 0 })
		_ = json.NewEncoder(w).Encode(map[string]any{
			"header": map[string]string{"revision": strconv.FormatInt(f.revision, 10)},
			"kvs":    kvs,
		})
	case "/v3/watch":
		f.watch(w, r, req.CreateRequest.Key, req.CreateRequest.RangeEnd, req.CreateRequest.StartRevision)
	default:
		f.mu.Unlock()
		http.NotFound(w, r)
	}
}

// watch streams the events of the keys in the range, f.mu is locked and is unlocked by watch.
func (f *fakeEtcd) watch(w http.ResponseWriter, r *http.Request, start, end []byte, startRevision string) {
	header := func() map[string]string {
		return map[string]string{"revision": strconv.FormatInt(f.revision, 10)}
	}
	send := func(result map[string]any) {
		result["header"] = header()
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
		w.(http.Flusher).Flush()
	}
	if len(f.users) > 0 && !f.tokens[r.Header.Get("Authorization")] {
		send(map[string]any{"created": true, "canceled": true, "cancel_reason": "rpc error: code = Unauthenticated desc = etcdserver: invalid auth token"})
		f.mu.Unlock()
		return
	}
	for _, denied := range f.denied {
		if bytes.HasPrefix(start, []byte(denied)) || inRange([]byte(denied), start, end) {
			send(map[string]any{"created": true, "canceled": true, "cancel_reason": "rpc error: code = PermissionDenied desc = etcdserver: permission denied"})
			f.mu.Unlock()
			return
		}
	}
	next, _ := strconv.ParseInt(startRevision, 10, 64)
	if next <= f.compacted {
		send(map[string]any{"created": true, "canceled": true, "compact_revision": strconv.FormatInt(f.compacted, 10)})
		f.mu.Unlock()
		return
	}
	send(map[string]any{"created": true})
	token := r.Header.Get("Authorization")
	for {
		if len(f.users) > 0 && !f.tokens[token] {
			// the token is expired while watching
			send(map[string]any{"canceled": true, "cancel_reason": "rpc error: code = Unauthenticated desc = etcdserver: invalid auth token"})
			f.mu.Unlock()
			return
		}
		var events []map[string]any
		for key, revision := range f.modify {
			if revision >= next && inRange([]byte(key), start, end) {
				events = append(events, map[string]any{"kv": map[string]any{"key": []byte(key)}})
			}
		}
		if len(events) > 0 {
			send(map[string]any{"events": events})
			next = f.revision + 1
		}
		changed := f.changed
		f.mu.Unlock()
		select {
		case <-changed:
		case <-r.Context().Done():
			return
		}
		f.mu.Lock()
	}
}
