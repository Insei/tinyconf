package tree

import (
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

func TestDriver_ParentTagWarning(t *testing.T) {
	type nested struct {
		Name string `test:"name"`
	}
	type config struct {
		Name   string `test:"name"`
		Tagged struct {
			Name  string `test:"name"`
			Inner nested
		} `test:"tagged,omitempty"`
		NoTag struct {
			Name  string `test:"name"`
			Inner nested
		}
		Ptr *nested
	}
	storage, err := fmap.Get[config]()
	assert.NoError(t, err)
	driver := New("test", func([]byte) (map[string]any, error) {
		return map[string]any{
			"name":   "root",
			"tagged": map[string]any{"name": "tagged"},
		}, nil
	}, []Source{DataSource("data", nil)}, false)

	tests := map[string]struct {
		value   string
		warning string
	}{
		"Name":        {value: "root"},
		"Tagged.Name": {value: "tagged"},
		"Tagged.Inner.Name": {
			// the path is built only from the tags below the parent without the tag
			value:   "root",
			warning: "parent fields Tagged.Inner have no 'test' tag, the value is read from the 'name' key",
		},
		"NoTag.Name": {
			value:   "root",
			warning: "parent fields NoTag have no 'test' tag, the value is read from the 'name' key",
		},
		"NoTag.Inner.Name": {
			value:   "root",
			warning: "parent fields NoTag, NoTag.Inner have no 'test' tag, the value is read from the 'name' key",
		},
		"Ptr.Name": {
			value:   "root",
			warning: "parent fields Ptr have no 'test' tag, the value is read from the 'name' key",
		},
	}
	for path, tt := range tests {
		val, err := driver.GetValue(storage.MustFind(path))
		if assert.NoError(t, err, path) {
			assert.Equal(t, tt.value, val.Value, path)
			assert.Equal(t, tt.warning, val.Warning, path)
		}
	}
}
