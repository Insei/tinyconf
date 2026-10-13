package yaml

import (
	"math"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
)

func TestYamlDriver_GenDoc_NotEncodable(t *testing.T) {
	type config struct {
		Ratios []float64 `yaml:"ratios"`
		Name   string    `yaml:"name"`
	}
	// NaN can not be encoded to JSON, that is used for slices, the value is not shown
	conf := &config{Ratios: []float64{math.NaN()}, Name: "app"}
	storage, err := fmap.GetFrom(conf)
	assert.NoError(t, err)

	driver, err := New("")
	assert.NoError(t, err)
	doc := driver.GenDoc(&tinyconf.Registered{Storage: storage, Config: conf})
	assert.Contains(t, doc, "ratios: \n")
	assert.Contains(t, doc, "name: app\n")
}
