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
	type embedded struct {
		Level string `test:"level"`
	}
	type config struct {
		Name   string `test:"name"`
		Tagged struct {
			embedded
			Name  string `test:"name"`
			Inner nested
		} `test:"tagged,omitempty"`
		NoTag struct {
			embedded
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
			"level":  "root",
			"tagged": map[string]any{"name": "tagged", "level": "tagged"},
		}, nil
	}, []Source{DataSource("data", nil)}, false)

	tests := map[string]struct {
		value   string
		warning string
	}{
		"Name":        {value: "root"},
		"Tagged.Name": {value: "tagged"},
		// fields of the embedded structs are on the level of the embedding struct, it is expected
		"Tagged.embedded.Level": {value: "tagged"},
		"NoTag.embedded.Level": {
			value:   "root",
			warning: "parent fields NoTag have no 'test' tag, the value is read from the 'level' key",
		},
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
