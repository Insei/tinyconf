package tinyconf

import (
	"errors"
	"fmt"
	"strings"

	"github.com/insei/fmap/v3"
)

var (
	ErrNotRegisteredConfig  = fmt.Errorf("config is not registered")
	ErrValueNotFound        = fmt.Errorf("value was not found")
	ErrIncorrectTagSettings = fmt.Errorf("incorrect tag settings")
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

type Value struct {
	Source string
	Value  interface{}
}

type Driver interface {
	GenDoc(...*Registered) string
	GetName() string
	GetValue(field fmap.Field) (*Value, error)
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
