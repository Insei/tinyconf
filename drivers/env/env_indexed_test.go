package env

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_indexedTree_Pointer(t *testing.T) {
	tree, found, err := indexedTree(reflect.TypeOf(new([]string)), map[string]string{"0": "a", "1": "b"})
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []any{"a", "b"}, tree)
}
