package convert

import (
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type stringerLevel int

func (l stringerLevel) String() string {
	return "level"
}

func TestToString(t *testing.T) {
	date := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	var nilPtr *int
	var nilSlice []string
	tests := []struct {
		name string
		v    any
		opts Options
		want string
		// value can not be parsed back to the same value
		noRoundTrip bool
	}{
		{name: "nil", v: nil, want: "", noRoundTrip: true},
		{name: "nil pointer", v: nilPtr, want: "", noRoundTrip: true},
		{name: "string", v: "value", want: "value"},
		{name: "int", v: -42, want: "-42"},
		{name: "float", v: 1.5, want: "1.5"},
		{name: "bool", v: true, want: "true"},
		{name: "pointer", v: ptr(42), want: "42"},
		{name: "named string", v: namedString("named"), want: "named"},
		{name: "named int", v: namedInt(7), want: "7"},
		{name: "duration", v: 90 * time.Second, want: "1m30s"},
		{name: "time", v: date, want: "2024-01-02T03:04:05Z"},
		{name: "url", v: mustURL("https://example.com/path?a=b"), want: "https://example.com/path?a=b"},
		{name: "text marshaler", v: net.ParseIP("127.0.0.1"), want: "127.0.0.1"},
		{name: "stringer", v: stringerLevel(1), want: "level", noRoundTrip: true},
		{name: "bytes", v: []byte("secret"), want: "secret"},
		{name: "slice", v: []string{"a", "b"}, want: "a,b"},
		{name: "nil slice", v: nilSlice, want: "", noRoundTrip: true},
		{name: "slice with quotes", v: []string{"a,b", " c", `"d"`, "it's"}, want: `"a,b"," c",'"d"',it's`},
		{name: "slice of durations", v: []time.Duration{time.Second, time.Minute}, want: "1s,1m0s"},
		{name: "slice custom separator", v: []string{"a,b", "c"}, opts: Options{Separator: ";"}, want: "a,b;c"},
		{name: "array", v: [2]int{1, 2}, want: "1,2"},
		{name: "map", v: map[string]int{"b": 2, "a": 1}, want: "a:1,b:2"},
		{name: "map with quotes", v: map[string]string{"a:b": "c,d"}, want: `"a:b":"c,d"`},
		{name: "map with int keys", v: map[int]string{1: "a"}, want: "1:a"},
		{name: "map custom separators", v: map[string]int{"a": 1, "b": 2}, opts: Options{Separator: ";", KeyValueSeparator: "="}, want: "a=1;b=2"},
		{name: "slice of slices", v: [][]int{{1, 2}, {3}}, want: "[[1,2],[3]]"},
		{name: "map of slices", v: map[string][]int{"a": {1}}, want: `{"a":[1]}`},
		{name: "struct", v: server{Host: "a", Port: 1, Timeout: time.Second, Tags: []string{"t"}}, want: `{"Tags":["t"],"host":"a","port":1,"timeout":"1s"}`},
		{name: "slice of structs", v: []server{{Host: "a", Tags: []string{}}}, want: `[{"Tags":[],"host":"a","port":0,"timeout":"0s"}]`},
		{name: "embedded struct", v: embedded{server: server{Host: "a", Tags: []string{}}, Name: "n"}, want: `{"Name":"n","Tags":[],"host":"a","port":0,"timeout":"0s"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToString(tt.v, tt.opts)
			assert.Equal(t, tt.want, got)
			if tt.noRoundTrip {
				return
			}
			parsed, err := FromString(got, reflect.TypeOf(tt.v), tt.opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.v, parsed)
		})
	}
}

func TestToTree(t *testing.T) {
	type config struct {
		Servers map[string]*server
		Level   *namedInt
		Empty   *server
		Any     any
	}
	level := namedInt(2)
	conf := config{
		Servers: map[string]*server{"main": {Host: "a", Port: 1, Timeout: time.Second}},
		Level:   &level,
		Any:     []int{1},
	}
	want := map[string]any{
		"Servers": map[string]any{"main": map[string]any{"address": "a", "Port": 1, "Timeout": "1s", "Tags": []any{}}},
		"Level":   2,
		"Empty":   nil,
		"Any":     []any{1},
	}
	assert.Equal(t, want, ToTree(conf, "yaml"))

	parsed, err := FromTree(ToTree(conf, "yaml"), reflect.TypeOf(conf), Options{Tag: "yaml"})
	assert.NoError(t, err)
	conf.Servers["main"].Tags = []string{}
	conf.Any = []any{1}
	assert.Equal(t, conf, parsed)
}
