package drivers_test

import (
	"bytes"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	yamlv3 "gopkg.in/yaml.v3"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/env"
	"github.com/insei/tinyconf/drivers/flag"
	jsondriver "github.com/insei/tinyconf/drivers/json"
	"github.com/insei/tinyconf/drivers/tag"
	tomldriver "github.com/insei/tinyconf/drivers/toml"
	"github.com/insei/tinyconf/drivers/yaml"
)

type conformanceNamedString string
type conformanceNamedInt int

type conformanceLevel int

func (l *conformanceLevel) UnmarshalText(text []byte) error {
	levels := map[string]conformanceLevel{"debug": 1, "info": 2}
	level, ok := levels[string(text)]
	if !ok {
		return assert.AnError
	}
	*l = level
	return nil
}

type conformanceServer struct {
	Host    string        `yaml:"host" json:"host" toml:"host"`
	Port    int           `yaml:"port" json:"port" toml:"port"`
	Timeout time.Duration `yaml:"timeout" json:"timeout" toml:"timeout"`
}

type conformanceNested struct {
	Timeout time.Duration `env:"CONFORMANCE_NESTED_TIMEOUT" yaml:"timeout" flag:"timeout" json:"timeout" toml:"timeout" initial:"1m"`
	Hosts   []string      `env:"CONFORMANCE_NESTED_HOSTS" yaml:"hosts" flag:"hosts" json:"hosts" toml:"hosts" initial:"a,b"`
}

// conformanceConfig contains all types supported by tinyconf.
// Every field should have tags for all drivers, value of the initial tag is also used as value of the env variable.
// The same values in the yaml format are in the conformanceYAML.
type conformanceConfig struct {
	String  string  `env:"CONFORMANCE_STRING" yaml:"string" flag:"string" json:"string" toml:"string" initial:"value"`
	Int     int     `env:"CONFORMANCE_INT" yaml:"int" flag:"int" json:"int" toml:"int" initial:"-1"`
	Int8    int8    `env:"CONFORMANCE_INT8" yaml:"int8" flag:"int8" json:"int8" toml:"int8" initial:"-8"`
	Int16   int16   `env:"CONFORMANCE_INT16" yaml:"int16" flag:"int16" json:"int16" toml:"int16" initial:"-16"`
	Int32   int32   `env:"CONFORMANCE_INT32" yaml:"int32" flag:"int32" json:"int32" toml:"int32" initial:"-32"`
	Int64   int64   `env:"CONFORMANCE_INT64" yaml:"int64" flag:"int64" json:"int64" toml:"int64" initial:"-64"`
	Uint    uint    `env:"CONFORMANCE_UINT" yaml:"uint" flag:"uint" json:"uint" toml:"uint" initial:"1"`
	Uint8   uint8   `env:"CONFORMANCE_UINT8" yaml:"uint8" flag:"uint8" json:"uint8" toml:"uint8" initial:"8"`
	Uint16  uint16  `env:"CONFORMANCE_UINT16" yaml:"uint16" flag:"uint16" json:"uint16" toml:"uint16" initial:"16"`
	Uint32  uint32  `env:"CONFORMANCE_UINT32" yaml:"uint32" flag:"uint32" json:"uint32" toml:"uint32" initial:"32"`
	Uint64  uint64  `env:"CONFORMANCE_UINT64" yaml:"uint64" flag:"uint64" json:"uint64" toml:"uint64" initial:"64"`
	Float32 float32 `env:"CONFORMANCE_FLOAT32" yaml:"float32" flag:"float32" json:"float32" toml:"float32" initial:"1.5"`
	Float64 float64 `env:"CONFORMANCE_FLOAT64" yaml:"float64" flag:"float64" json:"float64" toml:"float64" initial:"2.5"`
	Bool    bool    `env:"CONFORMANCE_BOOL" yaml:"bool" flag:"bool" json:"bool" toml:"bool" initial:"true"`

	StringPtr *string `env:"CONFORMANCE_STRING_PTR" yaml:"string_ptr" flag:"string-ptr" json:"string_ptr" toml:"string_ptr" initial:"pointer"`
	IntPtr    *int    `env:"CONFORMANCE_INT_PTR" yaml:"int_ptr" flag:"int-ptr" json:"int_ptr" toml:"int_ptr" initial:"42"`

	NamedString conformanceNamedString `env:"CONFORMANCE_NAMED_STRING" yaml:"named_string" flag:"named-string" json:"named_string" toml:"named_string" initial:"named"`
	NamedInt    conformanceNamedInt    `env:"CONFORMANCE_NAMED_INT" yaml:"named_int" flag:"named-int" json:"named_int" toml:"named_int" initial:"7"`

	Duration    time.Duration    `env:"CONFORMANCE_DURATION" yaml:"duration" flag:"duration" json:"duration" toml:"duration" initial:"1m30s"`
	DurationPtr *time.Duration   `env:"CONFORMANCE_DURATION_PTR" yaml:"duration_ptr" flag:"duration-ptr" json:"duration_ptr" toml:"duration_ptr" initial:"2s"`
	Time        time.Time        `env:"CONFORMANCE_TIME" yaml:"time" flag:"time" json:"time" toml:"time" initial:"2024-01-02T03:04:05Z"`
	TimePtr     *time.Time       `env:"CONFORMANCE_TIME_PTR" yaml:"time_ptr" flag:"time-ptr" json:"time_ptr" toml:"time_ptr" initial:"2024-01-02T03:04:05Z"`
	URL         url.URL          `env:"CONFORMANCE_URL" yaml:"url" flag:"url" json:"url" toml:"url" initial:"https://example.com/path?a=b"`
	IP          net.IP           `env:"CONFORMANCE_IP" yaml:"ip" flag:"ip" json:"ip" toml:"ip" initial:"127.0.0.1"`
	Level       conformanceLevel `env:"CONFORMANCE_LEVEL" yaml:"level" flag:"level" json:"level" toml:"level" initial:"info"`
	Bytes       []byte           `env:"CONFORMANCE_BYTES" yaml:"bytes" flag:"bytes" json:"bytes" toml:"bytes" initial:"secret"`

	Strings    []string           `env:"CONFORMANCE_STRINGS" yaml:"strings" flag:"strings" json:"strings" toml:"strings" initial:"a,b,c"`
	Quoted     []string           `env:"CONFORMANCE_QUOTED" yaml:"quoted" flag:"quoted" json:"quoted" toml:"quoted" initial:"'a,b',c"`
	Ints       []int              `env:"CONFORMANCE_INTS" yaml:"ints" flag:"ints" json:"ints" toml:"ints" initial:"1,2,3"`
	Durations  []time.Duration    `env:"CONFORMANCE_DURATIONS" yaml:"durations" flag:"durations" json:"durations" toml:"durations" initial:"1s,2m"`
	Levels     []conformanceLevel `env:"CONFORMANCE_LEVELS" yaml:"levels" flag:"levels" json:"levels" toml:"levels" initial:"debug,info"`
	Paths      []string           `env:"CONFORMANCE_PATHS" yaml:"paths" flag:"paths" json:"paths" toml:"paths" initial:"/bin:/usr/bin" separator:":"`
	StringsPtr *[]string          `env:"CONFORMANCE_STRINGS_PTR" yaml:"strings_ptr" flag:"strings-ptr" json:"strings_ptr" toml:"strings_ptr" initial:"a,b"`
	Array      [3]int             `env:"CONFORMANCE_ARRAY" yaml:"array" flag:"array" json:"array" toml:"array" initial:"1,2,3"`

	Map          map[string]int           `env:"CONFORMANCE_MAP" yaml:"map" flag:"map" json:"map" toml:"map" initial:"a:1,b:2"`
	MapIntKeys   map[int]string           `env:"CONFORMANCE_MAP_INT_KEYS" yaml:"map_int_keys" flag:"map-int-keys" json:"map_int_keys" toml:"map_int_keys" initial:"1:a,2:b"`
	MapDurations map[string]time.Duration `env:"CONFORMANCE_MAP_DURATIONS" yaml:"map_durations" flag:"map-durations" json:"map_durations" toml:"map_durations" initial:"read:1s,write:2s"`
	MapCustom    map[string]string        `env:"CONFORMANCE_MAP_CUSTOM" yaml:"map_custom" flag:"map-custom" json:"map_custom" toml:"map_custom" initial:"a=1;b=2" separator:";" kvseparator:"="`

	Matrix      [][]int                      `env:"CONFORMANCE_MATRIX" yaml:"matrix" flag:"matrix" json:"matrix" toml:"matrix" initial:"[[1,2],[3]]"`
	MapOfSlices map[string][]string          `env:"CONFORMANCE_MAP_OF_SLICES" yaml:"map_of_slices" flag:"map-of-slices" json:"map_of_slices" toml:"map_of_slices" initial:"{\"a\":[\"b\",\"c\"]}"`
	Servers     []conformanceServer          `env:"CONFORMANCE_SERVERS" yaml:"servers" flag:"servers" json:"servers" toml:"servers" initial:"[{\"host\":\"a\",\"port\":1,\"timeout\":\"1s\"},{\"host\":\"b\",\"port\":2}]"`
	ServersMap  map[string]conformanceServer `env:"CONFORMANCE_SERVERS_MAP" yaml:"servers_map" flag:"servers-map" json:"servers_map" toml:"servers_map" initial:"{\"main\":{\"host\":\"a\",\"port\":1}}"`
	ServerPtr   *conformanceServer           `env:"CONFORMANCE_SERVER_PTR" yaml:"server_ptr" flag:"server-ptr" json:"server_ptr" toml:"server_ptr" initial:"{\"host\":\"a\",\"port\":1}"`

	Nested conformanceNested `yaml:"nested" json:"nested" toml:"nested"`
}

const conformanceYAML = `
string: value
int: -1
int8: -8
int16: -16
int32: -32
int64: -64
uint: 1
uint8: 8
uint16: 16
uint32: 32
uint64: 64
float32: 1.5
float64: 2.5
bool: true
string_ptr: pointer
int_ptr: 42
named_string: named
named_int: 7
duration: 1m30s
duration_ptr: 2s
time: 2024-01-02T03:04:05Z
time_ptr: "2024-01-02T03:04:05Z"
url: https://example.com/path?a=b
ip: 127.0.0.1
level: info
bytes: secret
strings: [a, b, c]
quoted:
  - a,b
  - c
ints: [1, 2, 3]
durations: [1s, 2m]
levels: [debug, info]
paths: [/bin, /usr/bin]
strings_ptr: [a, b]
array: [1, 2, 3]
map: {a: 1, b: 2}
map_int_keys: {"1": a, "2": b}
map_durations:
  read: 1s
  write: 2s
map_custom: {a: "1", b: "2"}
matrix: [[1, 2], [3]]
map_of_slices:
  a: [b, c]
servers:
  - host: a
    port: 1
    timeout: 1s
  - host: b
    port: 2
servers_map:
  main:
    host: a
    port: 1
server_ptr:
  host: a
  port: 1
nested:
  timeout: 1m
  hosts: [a, b]
`

func ptr[T any](val T) *T {
	return &val
}

func conformanceExpected() conformanceConfig {
	date := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	u, _ := url.Parse("https://example.com/path?a=b")
	return conformanceConfig{
		String: "value",
		Int:    -1, Int8: -8, Int16: -16, Int32: -32, Int64: -64,
		Uint: 1, Uint8: 8, Uint16: 16, Uint32: 32, Uint64: 64,
		Float32: 1.5, Float64: 2.5,
		Bool:         true,
		StringPtr:    ptr("pointer"),
		IntPtr:       ptr(42),
		NamedString:  "named",
		NamedInt:     7,
		Duration:     90 * time.Second,
		DurationPtr:  ptr(2 * time.Second),
		Time:         date,
		TimePtr:      &date,
		URL:          *u,
		IP:           net.ParseIP("127.0.0.1"),
		Level:        2,
		Bytes:        []byte("secret"),
		Strings:      []string{"a", "b", "c"},
		Quoted:       []string{"a,b", "c"},
		Ints:         []int{1, 2, 3},
		Durations:    []time.Duration{time.Second, 2 * time.Minute},
		Levels:       []conformanceLevel{1, 2},
		Paths:        []string{"/bin", "/usr/bin"},
		StringsPtr:   &[]string{"a", "b"},
		Array:        [3]int{1, 2, 3},
		Map:          map[string]int{"a": 1, "b": 2},
		MapIntKeys:   map[int]string{1: "a", 2: "b"},
		MapDurations: map[string]time.Duration{"read": time.Second, "write": 2 * time.Second},
		MapCustom:    map[string]string{"a": "1", "b": "2"},
		Matrix:       [][]int{{1, 2}, {3}},
		MapOfSlices:  map[string][]string{"a": {"b", "c"}},
		Servers:      []conformanceServer{{Host: "a", Port: 1, Timeout: time.Second}, {Host: "b", Port: 2}},
		ServersMap:   map[string]conformanceServer{"main": {Host: "a", Port: 1}},
		ServerPtr:    &conformanceServer{Host: "a", Port: 1},
		Nested:       conformanceNested{Timeout: time.Minute, Hosts: []string{"a", "b"}},
	}
}

// conformanceFields returns all fields that should be parsed by drivers.
func conformanceFields(t *testing.T, typeOf reflect.Type) []reflect.StructField {
	var fields []reflect.StructField
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		if field.Type == reflect.TypeOf(conformanceNested{}) {
			fields = append(fields, conformanceFields(t, field.Type)...)
			continue
		}
		for _, tagName := range []string{"env", "yaml", "json", "toml", "flag", "initial"} {
			if _, ok := field.Tag.Lookup(tagName); !ok {
				t.Fatalf("field %s has no %s tag, all types should be checked for all drivers", field.Name, tagName)
			}
		}
		fields = append(fields, field)
	}
	return fields
}

// writeConformanceFiles writes the same content in the yaml, json and toml formats.
func writeConformanceFiles(t *testing.T, yamlDoc string) (string, string, string) {
	yamlPath := filepath.Join(t.TempDir(), "config.yaml")
	assert.NoError(t, os.WriteFile(yamlPath, []byte(yamlDoc), 0o600))

	var tree map[string]any
	assert.NoError(t, yamlv3.Unmarshal([]byte(yamlDoc), &tree))
	jsonDoc, err := json.Marshal(tree)
	assert.NoError(t, err)
	jsonPath := filepath.Join(t.TempDir(), "config.json")
	assert.NoError(t, os.WriteFile(jsonPath, jsonDoc, 0o600))

	var tomlDoc bytes.Buffer
	assert.NoError(t, toml.NewEncoder(&tomlDoc).Encode(tree))
	tomlPath := filepath.Join(t.TempDir(), "config.toml")
	assert.NoError(t, os.WriteFile(tomlPath, tomlDoc.Bytes(), 0o600))
	return yamlPath, jsonPath, tomlPath
}

// TestConformance checks that all drivers supports the same set of types.
func TestConformance(t *testing.T) {
	fields := conformanceFields(t, reflect.TypeOf(conformanceConfig{}))

	yamlPath, jsonPath, tomlPath := writeConformanceFiles(t, conformanceYAML)
	var args []string
	for _, field := range fields {
		t.Setenv(field.Tag.Get("env"), field.Tag.Get("initial"))
		args = append(args, "--"+field.Tag.Get("flag")+"="+field.Tag.Get("initial"))
	}

	drivers := map[string]func() (tinyconf.Driver, error){
		"env":  func() (tinyconf.Driver, error) { return env.New() },
		"tag":  func() (tinyconf.Driver, error) { return tag.New("initial") },
		"yaml": func() (tinyconf.Driver, error) { return yaml.New(yamlPath) },
		"json": func() (tinyconf.Driver, error) { return jsondriver.New(jsonPath) },
		"toml": func() (tinyconf.Driver, error) { return tomldriver.New(tomlPath) },
		"flag": func() (tinyconf.Driver, error) { return flag.New(flag.WithArgs(args)) },
	}
	expected := conformanceExpected()
	for name, newDriver := range drivers {
		newDriver := newDriver
		t.Run(name, func(t *testing.T) {
			driver, err := newDriver()
			assert.NoError(t, err)
			manager, err := tinyconf.New(tinyconf.WithDriver(driver))
			assert.NoError(t, err)

			conf := &conformanceConfig{}
			assert.NoError(t, manager.Register(conf))
			assert.NoError(t, manager.Parse(conf))

			expectedOf, confOf := reflect.ValueOf(expected), reflect.ValueOf(*conf)
			for i := 0; i < expectedOf.NumField(); i++ {
				fieldName := expectedOf.Type().Field(i).Name
				assert.Equal(t, expectedOf.Field(i).Interface(), confOf.Field(i).Interface(), fieldName)
			}

			t.Run("sub config", func(t *testing.T) {
				sub := &conformanceNested{}
				assert.NoError(t, manager.Parse(sub))
				assert.Equal(t, expected.Nested, *sub)
			})
		})
	}
}

// TestConformance_Errors checks that all drivers returns errors for the same incorrect values.
func TestConformance_Errors(t *testing.T) {
	type config struct {
		Int      int            `env:"CONFORMANCE_ERR_INT" yaml:"int" flag:"int" json:"int" toml:"int" initial:"abc"`
		Duration time.Duration  `env:"CONFORMANCE_ERR_DURATION" yaml:"duration" flag:"duration" json:"duration" toml:"duration" initial:"5"`
		Ints     []int          `env:"CONFORMANCE_ERR_INTS" yaml:"ints" flag:"ints" json:"ints" toml:"ints" initial:"1,b"`
		Array    [2]int         `env:"CONFORMANCE_ERR_ARRAY" yaml:"array" flag:"array" json:"array" toml:"array" initial:"1,2,3"`
		Map      map[string]int `env:"CONFORMANCE_ERR_MAP" yaml:"map" flag:"map" json:"map" toml:"map" initial:"a:b"`
		Matrix   [][]int        `env:"CONFORMANCE_ERR_MATRIX" yaml:"matrix" flag:"matrix" json:"matrix" toml:"matrix" initial:"[[1,\"b\"]]"`
		Chan     chan int       `env:"CONFORMANCE_ERR_CHAN" yaml:"chan" flag:"chan" json:"chan" toml:"chan" initial:"1"`
	}
	const yamlDoc = `
int: abc
duration: 5
ints: [1, b]
array: [1, 2, 3]
map: {a: b}
matrix: [[1, b]]
chan: 1
`
	typeOf := reflect.TypeOf(config{})
	yamlPath, jsonPath, tomlPath := writeConformanceFiles(t, yamlDoc)
	var args []string
	for i := 0; i < typeOf.NumField(); i++ {
		t.Setenv(typeOf.Field(i).Tag.Get("env"), typeOf.Field(i).Tag.Get("initial"))
		args = append(args, "--"+typeOf.Field(i).Tag.Get("flag"), typeOf.Field(i).Tag.Get("initial"))
	}

	drivers := map[string]func() (tinyconf.Driver, error){
		"env":  func() (tinyconf.Driver, error) { return env.New() },
		"tag":  func() (tinyconf.Driver, error) { return tag.New("initial") },
		"yaml": func() (tinyconf.Driver, error) { return yaml.New(yamlPath) },
		"json": func() (tinyconf.Driver, error) { return jsondriver.New(jsonPath) },
		"toml": func() (tinyconf.Driver, error) { return tomldriver.New(tomlPath) },
		"flag": func() (tinyconf.Driver, error) { return flag.New(flag.WithArgs(args)) },
	}
	for name, newDriver := range drivers {
		newDriver := newDriver
		t.Run(name, func(t *testing.T) {
			driver, err := newDriver()
			assert.NoError(t, err)
			manager, err := tinyconf.New(tinyconf.WithDriver(driver))
			assert.NoError(t, err)

			conf := &config{}
			assert.NoError(t, manager.Register(conf))
			err = manager.Parse(conf)
			assert.Error(t, err)
			for i := 0; i < typeOf.NumField(); i++ {
				assert.True(t, strings.Contains(err.Error(), "field "+typeOf.Field(i).Name+":"), typeOf.Field(i).Name)
			}
			assert.Equal(t, config{}, *conf)
		})
	}
}
