package tinyconf

import (
	"errors"
	"fmt"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

type customParseError struct {
	field string
}

func (e *customParseError) Error() string { return "custom error of " + e.field }

func Test_parseErrors(t *testing.T) {
	var err error = parseErrors{
		fmt.Errorf("first: %w", ErrValueNotFound),
		fmt.Errorf("second: %w", &customParseError{field: "Name"}),
	}
	assert.Equal(t, "first: value was not found; second: custom error of Name", err.Error())

	assert.ErrorIs(t, err, ErrValueNotFound)
	assert.False(t, errors.Is(err, ErrRequired))

	var custom *customParseError
	if assert.True(t, errors.As(err, &custom)) {
		assert.Equal(t, "Name", custom.field)
	}
	var other *testAsError
	assert.False(t, errors.As(err, &other))
}

type testAsError struct{}

func (e *testAsError) Error() string { return "" }

func Test_copyToSubConfig_Errors(t *testing.T) {
	type Sub struct {
		Name string
	}
	type Other struct {
		Title string
	}
	type Config struct {
		Sub Sub
	}
	// not structs
	assert.Error(t, copyToSubConfig(new(int), &Sub{}, "Sub", nil))
	assert.Error(t, copyToSubConfig(&Config{}, new(int), "Sub", nil))
	// sub config has no parsed field
	assert.Error(t, copyToSubConfig(&Config{}, &Other{}, "Sub", []string{"Sub.Name"}))

	sub := &Sub{}
	assert.NoError(t, copyToSubConfig(&Config{Sub: Sub{Name: "name"}}, sub, "Sub", []string{"Sub.Name"}))
	assert.Equal(t, "name", sub.Name)
}

func TestManager_StorageErrors(t *testing.T) {
	type Sub struct {
		Name string
	}
	type Config struct {
		Sub Sub
	}
	manager, err := New(WithDriver(&staticMockDriver{}))
	assert.NoError(t, err)
	assert.NoError(t, manager.Register(&Config{}))

	storageErr := errors.New("storage error")
	original := getStorage
	getStorage = func(any) (fmap.Storage, error) { return nil, storageErr }
	defer func() { getStorage = original }()

	type Other struct {
		Name string
	}
	assert.ErrorIs(t, manager.Register(&Other{}), storageErr)
	// parsed values can not be copied to the sub config
	assert.ErrorIs(t, manager.Parse(&Sub{}), storageErr)
}
