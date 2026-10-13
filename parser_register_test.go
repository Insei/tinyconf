package tinyconf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestManager_Parse_NotRegistered(t *testing.T) {
	type registered struct {
		Name string
	}
	type other struct {
		Name string
	}
	manager, err := New()
	assert.NoError(t, err)

	// there are no registered configs
	assert.ErrorIs(t, manager.Parse(&other{}), ErrNotRegisteredConfig)

	// config is not registered and it is not a field of the registered config
	assert.NoError(t, manager.Register(&registered{}))
	assert.ErrorIs(t, manager.Parse(&other{}), ErrNotRegisteredConfig)
	assert.Nil(t, manager.Sources(&other{}))
}

func Test_checkRequired_NotStruct(t *testing.T) {
	errs := checkRequired(new(int), nil, "")
	assert.Len(t, errs, 1)
}
