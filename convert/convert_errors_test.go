package convert

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFromString_Errors(t *testing.T) {
	tests := []struct {
		name  string
		value string
		typ   reflect.Type
	}{
		{"pointer to incorrect value", "abc", reflect.TypeOf(new(int))},
		{"not closed quote in map", `"a:1`, reflect.TypeOf(map[string]int{})},
		{"incorrect map value", "a:x", reflect.TypeOf(map[string]int{})},
		{"incorrect url", ":bad", reflect.TypeOf(url.URL{})},
		{"unsupported type", "x", reflect.TypeOf(make(chan int))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FromString(tt.value, tt.typ, Options{})
			assert.Error(t, err)
		})
	}
}

type Base struct {
	ID int `json:"id"`
}

type hidden struct {
	Secret string `json:"secret"`
}

func TestFromTree_Errors(t *testing.T) {
	type withEmbedded struct {
		*Base
		Name string `json:"name"`
	}
	tests := []struct {
		name  string
		value any
		typ   reflect.Type
	}{
		{"pointer to incorrect value", "abc", reflect.TypeOf(new(int))},
		{"incorrect map key", map[string]any{"a": 1}, reflect.TypeOf(map[int]int{})},
		{"incorrect map value", map[string]any{"a": "x"}, reflect.TypeOf(map[string]int{})},
		{"incorrect field of embedded struct", map[string]any{"id": "x"}, reflect.TypeOf(withEmbedded{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FromTree(tt.value, tt.typ, Options{Tag: "json"})
			assert.Error(t, err)
		})
	}
}

func TestFromTree_EmbeddedPointers(t *testing.T) {
	type withEmbedded struct {
		*Base
		// unexported embedded pointer can not be created
		*hidden
		Name string `json:"name"`
	}
	val, err := FromTree(map[string]any{"id": 7, "secret": "s", "name": "n"}, reflect.TypeOf(withEmbedded{}), Options{Tag: "json"})
	assert.NoError(t, err)
	assert.Equal(t, withEmbedded{Base: &Base{ID: 7}, Name: "n"}, val)
}

func TestFromTree_Bytes(t *testing.T) {
	val, err := FromTree([]byte("abc"), reflect.TypeOf(""), Options{})
	assert.NoError(t, err)
	assert.Equal(t, "abc", val)
}

func TestScalarFromString_Unsupported(t *testing.T) {
	_, err := scalarFromString("x", reflect.TypeOf(make(chan int)))
	assert.Error(t, err)
}
