package tinyconf

import (
	"fmt"
	"github.com/insei/fmap/v3"
)

var (
	ErrNotRegisteredConfig  = fmt.Errorf("config is not registered")
	ErrValueNotFound        = fmt.Errorf("value was not found")
	ErrIncorrectTagSettings = fmt.Errorf("incorrect tag settings")
)

type Value struct {
	Source string
	Value  interface{}
}

type Driver interface {
	GenDoc(...Registered) string
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
