package tinyconf

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/insei/fmap/v3"
)

var (
	ErrNotRegisteredConfig  = fmt.Errorf("config is not registered")
	ErrValueNotFound        = fmt.Errorf("value was not found")
	ErrIncorrectTagSettings = fmt.Errorf("incorrect tag settings")
	ErrRequired             = fmt.Errorf("required value is not set")
	ErrNoWatchers           = fmt.Errorf("no drivers that can watch for changes")
)

// parseErrors contains all driver errors collected during Manager.Parse.
type parseErrors []error

func (e parseErrors) Error() string {
	msgs := make([]string, 0, len(e))
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

func (e parseErrors) Is(target error) bool {
	for _, err := range e {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func (e parseErrors) As(target any) bool {
	for _, err := range e {
		if errors.As(err, target) {
			return true
		}
	}
	return false
}

// FieldSource describes the source of the config field value.
type FieldSource struct {
	// Field is a struct path of the field in the config, HTTP.Port for example.
	Field string
	// Driver is a name of the driver that has set the value.
	Driver string
	// Source is a source of the value inside of the driver: name of the env variable, flag and so on.
	Source string
}

type Value struct {
	Source string
	Value  interface{}
}

type Driver interface {
	GenDoc(...*Registered) string
	GetName() string
	GetValue(field fmap.Field) (*Value, error)
}

// Loader is an optional interface of the driver.
// Load is called once at the start of every Manager.Parse call, before any GetValue call,
// so driver can read its source once and not for every field.
// If Load returns an error, the driver is skipped and the error is returned from Manager.Parse.
type Loader interface {
	Load() error
}

// Watcher is an optional interface of the driver that can notify about changes of its source.
// Watch should block until ctx is done and call onChange after every change of the source.
type Watcher interface {
	Watch(ctx context.Context, onChange func()) error
}

type Option interface {
	apply(*Manager)
}

type Field struct {
	Key   string
	Value interface{}
}

func LogField(key string, value interface{}) Field {
	return Field{
		Key:   key,
		Value: value,
	}
}

type Logger interface {
	Debug(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	With(fields ...Field) Logger
}
