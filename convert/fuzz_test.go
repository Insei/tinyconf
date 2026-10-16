package convert

import (
	"net/url"
	"reflect"
	"testing"
	"time"
	"unicode/utf8"
)

type fuzzServer struct {
	Host string
	Port int
}

// fuzzTypes are the types of the fields with the different rules of parsing.
var fuzzTypes = []reflect.Type{
	reflect.TypeOf(""), reflect.TypeOf(0), reflect.TypeOf(uint8(0)), reflect.TypeOf(0.0), reflect.TypeOf(false),
	reflect.TypeOf(time.Duration(0)), reflect.TypeOf(time.Time{}), reflect.TypeOf(url.URL{}), reflect.TypeOf([]byte{}),
	reflect.TypeOf([]string{}), reflect.TypeOf([2]int{}), reflect.TypeOf(map[string]int{}), reflect.TypeOf(map[int]string{}),
	reflect.TypeOf([]fuzzServer{}), reflect.TypeOf(map[string]fuzzServer{}), reflect.TypeOf([][]string{}),
	reflect.TypeOf(new(int)), reflect.TypeOf([]*fuzzServer{}), reflect.TypeOf(fuzzServer{}),
}

// FuzzFromString checks that the string values do not panic for any type,
// and that the parsed lists and maps of strings are formatted back to the same values, for UTF-8 text.
func FuzzFromString(f *testing.F) {
	for _, seed := range []string{
		"a,b,c", `"a,b",c`, "a:1,b:2", `[{"host":"a","port":1}]`, `{"a":{"host":"b"}}`, "1m30s",
		"2024-01-02T03:04:05Z", "https://example.com/path?q=1", `[["a"],["b","c"]]`, `"unterminated`, "", "\\,",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, typeOf := range fuzzTypes {
			_, _ = FromString(s, typeOf, Options{})
		}
		if !utf8.ValidString(s) {
			// JSON replaces the bytes that are not UTF-8, the values of configs are text
			return
		}
		for _, typeOf := range []reflect.Type{reflect.TypeOf([]string{}), reflect.TypeOf(map[string]string{})} {
			value, err := FromString(s, typeOf, Options{})
			if err != nil {
				continue
			}
			formatted := ToString(value, Options{})
			parsed, err := FromString(formatted, typeOf, Options{})
			if err != nil {
				t.Fatalf("%s: formatted value %q of %q is not parsed: %v", typeOf, formatted, s, err)
			}
			if reflect.ValueOf(value).Len() > 0 && !reflect.DeepEqual(value, parsed) {
				t.Fatalf("%s: value %q is changed by formatting %q: %#v != %#v", typeOf, s, formatted, value, parsed)
			}
		}
	})
}
