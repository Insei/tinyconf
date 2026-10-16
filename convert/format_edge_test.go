package convert

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

// onlyUnmarshaler is parsed from the text, but has no methods to format it back.
type onlyUnmarshaler struct {
	value string
}

func (u *onlyUnmarshaler) UnmarshalText(text []byte) error {
	u.value = string(text)
	return nil
}

func TestToTree_Edges(t *testing.T) {
	t.Run("default tag", func(t *testing.T) {
		assert.Equal(t, map[string]any{"id": 1}, ToTree(Base{ID: 1}, ""))
	})

	t.Run("not supported type is formatted as is", func(t *testing.T) {
		ch := make(chan int)
		assert.NotEmpty(t, ToTree(ch, "json"))
	})

	t.Run("embedded pointers", func(t *testing.T) {
		type withEmbedded struct {
			*Base
			Name string `json:"name"`
		}
		assert.Equal(t, map[string]any{"name": "n"}, ToTree(withEmbedded{Name: "n"}, "json"))
		assert.Equal(t, map[string]any{"id": 2, "name": "n"}, ToTree(withEmbedded{Base: &Base{ID: 2}, Name: "n"}, "json"))
	})

	t.Run("scalar without format methods", func(t *testing.T) {
		assert.Equal(t, "{text}", ToTree(onlyUnmarshaler{value: "text"}, "json"))
	})
}

func TestToString_NotEncodableJSON(t *testing.T) {
	// NaN can not be encoded to JSON, the tree is formatted as is
	assert.Equal(t, "[[NaN]]", ToString([][]float64{{math.NaN()}}, Options{}))
}

func TestFieldToString(t *testing.T) {
	type config struct {
		Hosts []string `separator:";"`
	}
	storage, err := fmap.Get[config]()
	assert.NoError(t, err)
	assert.Equal(t, "a;b", FieldToString([]string{"a", "b"}, storage.MustFind("Hosts")))
}

func TestFieldToTagString(t *testing.T) {
	type server struct {
		Host string `ini:"host"`
	}
	type config struct {
		Servers []server
	}
	storage, err := fmap.Get[config]()
	assert.NoError(t, err)
	field := storage.MustFind("Servers")
	// keys of the nested structs are taken from the tag, the value is parsed back by TreeToField with the tag
	value := FieldToTagString([]server{{Host: "a"}}, field, "ini")
	assert.Equal(t, `[{"host":"a"}]`, value)
	parsed, err := TreeToField(value, field, "ini")
	assert.NoError(t, err)
	assert.Equal(t, []server{{Host: "a"}}, parsed)
}

func TestNormalizeJSON_BigNumber(t *testing.T) {
	// number that is not fit into int and float64 is kept as a string
	assert.Equal(t, "1e999", normalizeJSON(json.Number("1e999")))
}

// the cases found by FuzzFromString
func TestToString_Quotes(t *testing.T) {
	tests := []struct {
		value     any
		formatted string
	}{
		// an empty string is an empty list, so the empty item is quoted
		{[]string{""}, `""`},
		// the values that start with [ and { are parsed as JSON
		{[]string{"[a", "b"}, `"[a",b`},
		{map[string]string{"{": ""}, `"{":""`},
		// the separator of the key and the value inside of the value
		{map[string]string{"": `0:"`}, `"":'0:"'`},
		// both quotes can not be quoted without escapes, the value is JSON
		{[]string{`"',0"`}, `["\"',0\""]`},
		{map[string]string{"a": `"',`}, `{"a":"\"',"}`},
	}
	for _, tt := range tests {
		formatted := ToString(tt.value, Options{})
		assert.Equal(t, tt.formatted, formatted)
		parsed, err := FromString(formatted, reflect.TypeOf(tt.value), Options{})
		assert.NoError(t, err)
		assert.Equal(t, tt.value, parsed)
	}
}
