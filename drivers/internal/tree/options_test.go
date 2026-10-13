package tree

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"testing/iotest"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

func TestOptions(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "config")
	assert.NoError(t, os.WriteFile(filePath, []byte("name=file\nport=80"), 0o600))

	opts := &Options{Optional: true}
	// empty file names are ignored
	opts.AddFiles("", filePath, filepath.Join(t.TempDir(), "missing"))
	opts.AddFS(fstest.MapFS{"fs": {Data: []byte("name=fs")}}, "fs")
	opts.AddData("data", []byte("name=data"))
	assert.Len(t, opts.Sources, 4)

	driver := opts.NewDriver("test", testDecode)
	// options can be changed after creation of the driver
	opts.AddData("ignored", []byte("name=ignored"))
	assert.Equal(t, "test", driver.GetName())
	assert.NoError(t, driver.Load())

	storage, err := fmap.Get[testConfig]()
	assert.NoError(t, err)
	val, err := driver.GetValue(storage.MustFind("Name"))
	assert.NoError(t, err)
	assert.Equal(t, "data", val.Value)
	val, err = driver.GetValue(storage.MustFind("Port"))
	assert.NoError(t, err)
	assert.Equal(t, 80, val.Value)
}

func TestDriver_ReadError(t *testing.T) {
	readErr := errors.New("read error")
	driver := New("test", testDecode, []Source{{Name: "broken", Open: func() (io.ReadCloser, error) {
		return io.NopCloser(iotest.ErrReader(readErr)), nil
	}}}, false)
	assert.ErrorIs(t, driver.Load(), readErr)
}
