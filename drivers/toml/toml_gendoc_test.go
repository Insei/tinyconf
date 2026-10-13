package toml

import (
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
)

func TestTOMLDriver_GenDoc_NilItems(t *testing.T) {
	type config struct {
		Limits map[string]*int `toml:"limits"`
		Ports  []*int          `toml:"ports"`
	}
	one := 1
	// TOML has no null, nil items are skipped
	conf := &config{
		Limits: map[string]*int{"cpu": &one, "memory": nil},
		Ports:  []*int{nil, &one},
	}
	storage, err := fmap.GetFrom(conf)
	assert.NoError(t, err)

	driver, err := New("")
	assert.NoError(t, err)
	doc := driver.GenDoc(&tinyconf.Registered{Storage: storage, Config: conf})
	assert.Equal(t, "ports = [1]\n\n[limits]\n  cpu = 1\n", doc)
}

func Test_encodeDoc(t *testing.T) {
	assert.Equal(t, "", encodeDoc(nil))
	assert.Equal(t, "name = \"app\"\n", encodeDoc(map[string]any{"name": "app"}))
	// not supported type
	assert.Equal(t, "", encodeDoc(map[string]any{"name": make(chan int)}))
}
