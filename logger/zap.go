package logger

import (
	"go.uber.org/zap"

	"github.com/insei/tinyconf"
)

// zapLogger is a simple l that uses zap.Logger
type zapLogger struct {
	l *zap.Logger
}

func (t *zapLogger) castToZapFields(fields ...tinyconf.Field) []zap.Field {
	zapFields := make([]zap.Field, len(fields))
	for i, f := range fields {
		zapFields[i] = zap.Any(f.Key, f.Value)
	}
	return zapFields
}

// Debug logs a message at the DEBUG level
func (t *zapLogger) Debug(msg string, fld ...tinyconf.Field) {
	t.l.Debug(msg, t.castToZapFields(fld...)...)
}

// Error logs a message at the ERROR level
func (t *zapLogger) Error(msg string, fld ...tinyconf.Field) {
	t.l.Error(msg, t.castToZapFields(fld...)...)
}

// Warn logs a message at the WARN level
func (t *zapLogger) Warn(msg string, fld ...tinyconf.Field) {
	t.l.Warn(msg, t.castToZapFields(fld...)...)
}

// Info logs a message at the INFO level
func (t *zapLogger) Info(msg string, fld ...tinyconf.Field) {
	t.l.Info(msg, t.castToZapFields(fld...)...)
}

func (t *zapLogger) With(fld ...tinyconf.Field) tinyconf.Logger {
	t.l.With(t.castToZapFields(fld...)...)
	return t
}

// NewZapLogger creates a new zapLogger
func NewZapLogger(l *zap.Logger) tinyconf.Logger {
	return &zapLogger{l: l}
}
