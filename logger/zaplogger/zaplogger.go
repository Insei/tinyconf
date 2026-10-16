// Package zaplogger is the adapter of zap for the logger of tinyconf. It is a separate module,
// so zap is not a dependency of tinyconf for the applications that use other loggers.
//
//	config, err := tinyconf.New(tinyconf.WithLogger(zaplogger.New(zapLogger)))
package zaplogger

import (
	"go.uber.org/zap"

	"github.com/insei/tinyconf"
)

type zapLogger struct {
	l *zap.Logger
}

func (t *zapLogger) with(fields ...tinyconf.Field) *zap.Logger {
	l := t.l
	for _, field := range fields {
		l = l.With(zap.Any(field.Key, field.Value))
	}
	return l
}

// Debug logs a message at the DEBUG level.
func (t *zapLogger) Debug(msg string, fields ...tinyconf.Field) {
	t.with(fields...).Debug(msg)
}

// Error logs a message at the ERROR level.
func (t *zapLogger) Error(msg string, fields ...tinyconf.Field) {
	t.with(fields...).Error(msg)
}

// Warn logs a message at the WARN level.
func (t *zapLogger) Warn(msg string, fields ...tinyconf.Field) {
	t.with(fields...).Warn(msg)
}

// Info logs a message at the INFO level.
func (t *zapLogger) Info(msg string, fields ...tinyconf.Field) {
	t.with(fields...).Info(msg)
}

// With returns the logger with the fields.
func (t *zapLogger) With(fields ...tinyconf.Field) tinyconf.Logger {
	return &zapLogger{l: t.with(fields...)}
}

// New returns the logger of tinyconf that writes to the zap logger.
func New(l *zap.Logger) tinyconf.Logger {
	return &zapLogger{l: l}
}
