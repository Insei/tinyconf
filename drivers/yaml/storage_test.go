package yaml

import (
	"errors"
	"testing"

	"github.com/insei/tinyconf"
	"github.com/stretchr/testify/assert"
)

func TestStorageImpl(t *testing.T) {
	t.Run("getMap", func(t *testing.T) {
		storage := &storageImpl{yamlMap: map[string]any{"key": "value"}}
		result := storage.getMap()
		assert.Equal(t, map[string]any{"key": "value"}, result)
	})

	t.Run("getReaderCloser", func(t *testing.T) {
		unaccessiblePath := "invalid_path.yaml"
		storage := &storageImpl{filePath: unaccessiblePath}
		_, err := storage.getReaderCloser()
		assert.Error(t, err)
	})

	t.Run("parseYAML", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			stor := &storageImpl{}
			mock := &mockReaderCloser{data: []byte("key: value")}
			err := stor.parseYAML(mock)
			assert.NoError(t, err)
			assert.Equal(t, map[string]any{"key": "value"}, stor.getMap())
		})
		t.Run("bad yaml", func(t *testing.T) {
			stor := &storageImpl{}
			mock := &mockReaderCloser{data: []byte("key: value:")}
			err := stor.parseYAML(mock)
			assert.Error(t, err)
		})
	})

	t.Run("load", func(t *testing.T) {
		t.Run("already initialized", func(t *testing.T) {
			storage := &storageImpl{
				initialized: true,
				yamlMap:     nil,
			}
			_, err := storage.load()
			assert.True(t, errors.Is(err, tinyconf.ErrValueNotFound))
		})

		t.Run("unable to read file", func(t *testing.T) {
			storage := &storageImpl{
				filePath: "invalid_path.yaml",
			}
			_, err := storage.load()
			assert.Error(t, err)
		})

		t.Run("successful load", func(t *testing.T) {
			storage := &storageImpl{
				filePath: "existing_file.yaml",
			}
			loadedMap, err := storage.load()
			assert.NoError(t, err)
			assert.Equal(t, map[string]any{"key": "value"}, loadedMap)
		})
	})
}
