package tinyconf

import (
	"context"
	"reflect"
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
	assert.ErrorIs(t, manager.Parse(context.Background(), &other{}), ErrNotRegisteredConfig)

	// config is not registered and it is not a field of the registered config
	assert.NoError(t, manager.Register(&registered{}))
	assert.ErrorIs(t, manager.Parse(context.Background(), &other{}), ErrNotRegisteredConfig)
	assert.Nil(t, manager.Sources(&other{}))
}

func Test_checkRequired_NotStruct(t *testing.T) {
	errs := checkRequired(new(int), nil, "")
	assert.Len(t, errs, 1)
}

func TestManager_findRegister_Cache(t *testing.T) {
	type Sub struct {
		Name string
	}
	type First struct {
		Sub Sub
	}
	type Second struct {
		Other Sub
	}
	c, err := New(WithDriver(&staticMockDriver{}))
	assert.NoError(t, err)
	// not found is cached too
	register, _, _ := c.findRegister(reflect.TypeOf(&Sub{}))
	assert.Nil(t, register)

	// the cache is cleared by Register, the first registered config with the sub config type is used
	assert.NoError(t, c.Register(&First{}))
	assert.NoError(t, c.Register(&Second{}))
	register, typeOf, path := c.findRegister(reflect.TypeOf(&Sub{}))
	assert.Equal(t, reflect.TypeOf(&First{}), typeOf)
	assert.Equal(t, "Sub", path)
	cached, _, _ := c.findRegister(reflect.TypeOf(&Sub{}))
	assert.Same(t, register, cached)
}
