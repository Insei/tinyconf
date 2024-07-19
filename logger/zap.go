package logger

import (
	"go.uber.org/zap"

	"github.com/insei/tinyconf"
)

// zapLogger is a simple l that uses zap.Logger
type zapLogger struct {
	l *zap.Logger
}

func (t *zapLogger) castToZapFields(flds ...tinyconf.Field) *zapLogger {
	l := t.l
	for _, fld := range flds {
		l = l.With(zap.Any(fld.Key, fld.Value))
	}
	return &zapLogger{l: l}
}

// Debug logs a message at the DEBUG level
func (t *zapLogger) Debug(msg string, fld ...tinyconf.Field) {
	t.castToZapFields(fld...).l.Debug(msg)
}

// Error logs a message at the ERROR level
func (t *zapLogger) Error(msg string, fld ...tinyconf.Field) {
	t.castToZapFields(fld...).l.Error(msg)
}

// Warn logs a message at the WARN level
func (t *zapLogger) Warn(msg string, fld ...tinyconf.Field) {
	t.castToZapFields(fld...).l.Warn(msg)
}

// Info logs a message at the INFO level
func (t *zapLogger) Info(msg string, fld ...tinyconf.Field) {
	t.castToZapFields(fld...).l.Info(msg)
}

func (t *zapLogger) With(fld ...tinyconf.Field) tinyconf.Logger {
	return t.castToZapFields(fld...)
}

// NewZapLogger creates a new zapLogger
func NewZapLogger(l *zap.Logger) tinyconf.Logger {
	return &zapLogger{l: l}
}
