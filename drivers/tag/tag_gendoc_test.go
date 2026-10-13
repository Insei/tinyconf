package tag

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagDriver_GenDoc(t *testing.T) {
	driver, err := New("initial")
	assert.NoError(t, err)
	// default values are documented by other drivers
	assert.Equal(t, "", driver.GenDoc())
}
