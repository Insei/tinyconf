package json

import (
	"math"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
)

func TestJSONDriver_GenDoc_NotEncodable(t *testing.T) {
	type config struct {
		Ratio float64 `json:"ratio"`
	}
	// NaN can not be encoded to JSON
	conf := &config{Ratio: math.NaN()}
	storage, err := fmap.GetFrom(conf)
	assert.NoError(t, err)

	driver, err := New("")
	assert.NoError(t, err)
	assert.Equal(t, "", driver.GenDoc(&tinyconf.Registered{Storage: storage, Config: conf}))
}
