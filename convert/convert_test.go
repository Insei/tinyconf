package convert

import (
	"net"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type namedString string
type namedInt int

type level int

func (l *level) UnmarshalText(text []byte) error {
	switch string(text) {
	case "debug":
		*l = 1
	case "info":
		*l = 2
	default:
		return assert.AnError
	}
	return nil
}

type server struct {
	Host    string        `json:"host" yaml:"address"`
	Port    int           `json:"port"`
	Timeout time.Duration `json:"timeout"`
	Tags    []string
	Skipped string `json:"-" yaml:"-"`
	private string
}

type embedded struct {
	server
	Name string
}

func ptr[T any](val T) *T {
	return &val
}

func mustURL(s string) url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return *u
}

func TestFromString(t *testing.T) {
	date := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name    string
		s       string
		opts    Options
		want    any
		wantErr bool
	}{
		{name: "string", s: "value", want: "value"},
		{name: "int", s: "-42", want: -42},
		{name: "uint8", s: "42", want: uint8(42)},
		{name: "uint8 overflow", s: "256", want: uint8(0), wantErr: true},
		{name: "float64", s: "1.5", want: 1.5},
		{name: "bool", s: "true", want: true},
		{name: "bool incorrect", s: "yes!", want: false, wantErr: true},
		{name: "pointer", s: "42", want: ptr(42)},
		{name: "pointer to pointer", s: "42", want: ptr(ptr(42))},
		{name: "named string", s: "value", want: namedString("value")},
		{name: "named int", s: "42", want: namedInt(42)},
		{name: "pointer to named int", s: "42", want: ptr(namedInt(42))},
		{name: "duration", s: "1m30s", want: 90 * time.Second},
		{name: "duration incorrect", s: "90", want: time.Duration(0), wantErr: true},
		{name: "time RFC3339", s: "2024-01-02T03:04:05Z", want: date},
		{name: "time without zone", s: "2024-01-02 03:04:05", want: date},
		{name: "time date only", s: "2024-01-02", want: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{name: "time incorrect", s: "tomorrow", want: time.Time{}, wantErr: true},
		{name: "url", s: "https://example.com/path?a=b", want: mustURL("https://example.com/path?a=b")},
		{name: "bytes", s: "a,b", want: []byte("a,b")},
		{name: "text unmarshaler", s: "info", want: level(2)},
		{name: "text unmarshaler error", s: "unknown", want: level(0), wantErr: true},
		{name: "text unmarshaler slice type", s: "127.0.0.1", want: net.ParseIP("127.0.0.1")},
		{name: "empty interface", s: "value", want: ptr(any("value"))},
		{name: "slice", s: "a, b ,c", want: []string{"a", "b", "c"}},
		{name: "slice of ints", s: "1,2,3", want: []int{1, 2, 3}},
		{name: "slice of ints incorrect item", s: "1,b,3", want: []int(nil), wantErr: true},
		{name: "slice empty", s: "", want: []string{}},
		{name: "slice single item", s: "a", want: []string{"a"}},
		{name: "slice with empty item", s: "a,,c", want: []string{"a", "", "c"}},
		{name: "slice quoted items", s: `"a,b", 'c,d', it's`, want: []string{"a,b", "c,d", "it's"}},
		{name: "slice not closed quote", s: `"a,b`, want: []string(nil), wantErr: true},
		{name: "slice custom separator", s: "a;b,c", opts: Options{Separator: ";"}, want: []string{"a", "b,c"}},
		{name: "slice of durations", s: "1s,2m", want: []time.Duration{time.Second, 2 * time.Minute}},
		{name: "slice of pointers", s: "1,2", want: []*int{ptr(1), ptr(2)}},
		{name: "slice of text unmarshalers", s: "debug,info", want: []level{1, 2}},
		{name: "slice of any", s: "a,b", want: []any{"a", "b"}},
		{name: "slice JSON", s: `["a,b", "c"]`, want: []string{"a,b", "c"}},
		{name: "slice JSON incorrect", s: `["a,b", "c"`, want: []string(nil), wantErr: true},
		{name: "slice of any JSON", s: `[1, 1.5, "a", [2], {"b": 3}]`, want: []any{1, 1.5, "a", []any{2}, map[string]any{"b": 3}}},
		{name: "slice JSON with trailing data", s: `[1] 2`, want: []int(nil), wantErr: true},
		{name: "slice of slices JSON", s: `[[1,2],[3]]`, want: [][]int{{1, 2}, {3}}},
		{name: "slice of slices without JSON", s: `1,2`, want: [][]int(nil), wantErr: true},
		{name: "pointer to slice", s: "a,b", want: ptr([]string{"a", "b"})},
		{name: "array", s: "1,2,3", want: [3]int{1, 2, 3}},
		{name: "array incorrect length", s: "1,2", want: [3]int{}, wantErr: true},
		{name: "array JSON", s: "[1,2,3]", want: [3]int{1, 2, 3}},
		{name: "map", s: "a:1, b:2", want: map[string]int{"a": 1, "b": 2}},
		{name: "map empty", s: "", want: map[string]int{}},
		{name: "map with separator in value", s: "api:http://localhost:80", want: map[string]string{"api": "http://localhost:80"}},
		{name: "map quoted", s: `"a:b":"c,d",e:'f'`, want: map[string]string{"a:b": "c,d", "e": "f"}},
		{name: "map without key value separator", s: "a", want: map[string]int(nil), wantErr: true},
		{name: "map custom separators", s: "a=1;b=2", opts: Options{Separator: ";", KeyValueSeparator: "="}, want: map[string]int{"a": 1, "b": 2}},
		{name: "map with int keys", s: "1:a,2:b", want: map[int]string{1: "a", 2: "b"}},
		{name: "map incorrect key", s: "a:b", want: map[int]string(nil), wantErr: true},
		{name: "map of durations", s: "read:1s", want: map[string]time.Duration{"read": time.Second}},
		{name: "map of any", s: "a:b", want: map[string]any{"a": "b"}},
		{name: "map JSON", s: `{"a": 1, "b": 2}`, want: map[string]int{"a": 1, "b": 2}},
		{name: "map of slices JSON", s: `{"a": [1, 2]}`, want: map[string][]int{"a": {1, 2}}},
		{name: "map of slices without JSON", s: `a:1`, want: map[string][]int(nil), wantErr: true},
		{
			name: "struct JSON",
			s:    `{"host": "localhost", "PORT": 1, "port": 80, "timeout": "5s", "tags": ["a"], "Skipped": "s", "private": "p"}`,
			want: server{Host: "localhost", Port: 80, Timeout: 5 * time.Second, Tags: []string{"a"}},
		},
		{name: "struct without JSON", s: "localhost", want: server{}, wantErr: true},
		{name: "struct JSON incorrect field", s: `{"port": "http"}`, want: server{}, wantErr: true},
		{name: "struct with embedded JSON", s: `{"host": "localhost", "name": "main"}`, want: embedded{server: server{Host: "localhost"}, Name: "main"}},
		{
			name: "slice of structs JSON",
			s:    `[{"host": "a", "port": 1}, {"host": "b", "port": 2}]`,
			want: []server{{Host: "a", Port: 1}, {Host: "b", Port: 2}},
		},
		{name: "slice of pointers to structs JSON", s: `[{"host": "a"}, null]`, want: []*server{{Host: "a"}, nil}},
		{name: "map of structs JSON", s: `{"main": {"host": "a"}}`, want: map[string]server{"main": {Host: "a"}}},
		{name: "custom tag", s: `{"address": "a"}`, opts: Options{Tag: "yaml"}, want: server{Host: "a"}},
		{name: "unsupported", s: "value", want: make(chan int), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typeOf := reflect.TypeOf(tt.want)
			if tt.name == "empty interface" {
				typeOf = typeOf.Elem()
				tt.want = "value"
			}
			got, err := FromString(tt.s, typeOf, tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFromTree(t *testing.T) {
	date := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name    string
		v       any
		opts    Options
		want    any
		wantErr bool
	}{
		{name: "same type", v: 42, want: 42},
		{name: "string to int", v: "42", want: 42},
		{name: "int to string", v: 42, want: "42"},
		{name: "int to float", v: 42, want: 42.0},
		{name: "float to int", v: 42.0, want: 42},
		{name: "float with fraction to int", v: 42.5, want: 0, wantErr: true},
		{name: "int to int8 overflow", v: 300, want: int8(0), wantErr: true},
		{name: "bool to string", v: true, want: "true"},
		{name: "pointer", v: 42, want: ptr(42)},
		{name: "nil to pointer", v: nil, want: (*int)(nil)},
		{name: "nil to value", v: nil, want: 0},
		{name: "named int", v: 42, want: namedInt(42)},
		{name: "duration", v: "5s", want: 5 * time.Second},
		{name: "time", v: date, want: date},
		{name: "time from string", v: "2024-01-02T03:04:05Z", want: date},
		{name: "time to string", v: date, want: "2024-01-02T03:04:05Z"},
		{name: "text unmarshaler", v: "debug", want: level(1)},
		{name: "bytes from string", v: "abc", want: []byte("abc")},
		{name: "bytes from list", v: []any{97, 98}, want: []byte("ab")},
		{name: "list to scalar", v: []any{1}, want: 0, wantErr: true},
		{name: "map to scalar", v: map[string]any{"a": 1}, want: "", wantErr: true},
		{name: "slice", v: []any{"a", "b"}, want: []string{"a", "b"}},
		{name: "slice with conversion", v: []any{1, "2", 3.0}, want: []int{1, 2, 3}},
		{name: "slice with incorrect item", v: []any{1, "b"}, want: []int(nil), wantErr: true},
		{name: "slice from string", v: "a,b", want: []string{"a", "b"}},
		{name: "slice from scalar", v: 80, want: []int{80}},
		{name: "slice of any", v: []any{1, "a"}, want: []any{1, "a"}},
		{name: "slice of durations", v: []any{"1s", "2m"}, want: []time.Duration{time.Second, 2 * time.Minute}},
		{name: "slice of slices", v: []any{[]any{1, 2}, []any{3}}, want: [][]int{{1, 2}, {3}}},
		{name: "slice from map", v: map[string]any{"a": 1}, want: []int(nil), wantErr: true},
		{name: "array", v: []any{1, 2}, want: [2]int{1, 2}},
		{name: "array incorrect length", v: []any{1, 2, 3}, want: [2]int{}, wantErr: true},
		{name: "map", v: map[string]any{"a": 1, "b": "2"}, want: map[string]int{"a": 1, "b": 2}},
		{name: "map with any keys", v: map[any]any{1: "a"}, want: map[int]string{1: "a"}},
		{name: "map with string keys to int keys", v: map[string]any{"1": "a"}, want: map[int]string{1: "a"}},
		{name: "map of any", v: map[string]any{"a": []any{1}}, want: map[string]any{"a": []any{1}}},
		{name: "map from string", v: "a:1", want: map[string]int{"a": 1}},
		{name: "map from list", v: []any{1}, want: map[string]int(nil), wantErr: true},
		{name: "map of slices", v: map[string]any{"a": []any{1, 2}}, want: map[string][]int{"a": {1, 2}}},
		{
			name: "struct",
			v:    map[string]any{"host": "a", "port": "80", "timeout": "5s", "TAGS": []any{"t"}, "Skipped": "s"},
			want: server{Host: "a", Port: 80, Timeout: 5 * time.Second, Tags: []string{"t"}},
		},
		{name: "struct with custom tag", v: map[string]any{"address": "a", "host": "b"}, opts: Options{Tag: "yaml"}, want: server{Host: "a"}},
		{name: "struct from JSON string", v: `{"host": "a"}`, want: server{Host: "a"}},
		{name: "struct from list", v: []any{1}, want: server{}, wantErr: true},
		{name: "pointer to struct", v: map[string]any{"host": "a"}, want: &server{Host: "a"}},
		{
			name: "slice of structs",
			v:    []any{map[string]any{"host": "a"}, map[string]any{"host": "b"}},
			want: []server{{Host: "a"}, {Host: "b"}},
		},
		{name: "map of structs", v: map[string]any{"main": map[string]any{"host": "a"}}, want: map[string]*server{"main": {Host: "a"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromTree(tt.v, reflect.TypeOf(tt.want), tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsLeaf(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want bool
	}{
		{name: "int", val: 1, want: true},
		{name: "slice", val: []server{}, want: true},
		{name: "map", val: map[string]server{}, want: true},
		{name: "time", val: time.Time{}, want: true},
		{name: "pointer to time", val: &time.Time{}, want: true},
		{name: "url", val: url.URL{}, want: true},
		{name: "struct", val: server{}, want: false},
		{name: "pointer to struct", val: &server{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsLeaf(reflect.TypeOf(tt.val)))
		})
	}
}

func TestDecodeJSON(t *testing.T) {
	got, err := DecodeJSON([]byte(`{"int": 1, "float": 1.5, "big": 1e40, "list": [1, "a", null, true]}`))
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"int": 1, "float": 1.5, "big": 1e40, "list": []any{1, "a", nil, true}}, got)

	_, err = DecodeJSON([]byte(`{"int": 1`))
	assert.Error(t, err)
	_, err = DecodeJSON([]byte(`{} {}`))
	assert.Error(t, err)
}

func TestIsScalar(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want bool
	}{
		{name: "int", val: 1, want: true},
		{name: "pointer to string", val: ptr(""), want: true},
		{name: "duration", val: time.Second, want: true},
		{name: "time", val: time.Time{}, want: true},
		{name: "bytes", val: []byte{}, want: true},
		{name: "text unmarshaler slice type", val: net.IP{}, want: true},
		{name: "slice", val: []string{}, want: false},
		{name: "array", val: [2]int{}, want: false},
		{name: "map", val: map[string]int{}, want: false},
		{name: "struct", val: server{}, want: false},
		{name: "pointer to struct", val: &server{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsScalar(reflect.TypeOf(tt.val)))
		})
	}
}
