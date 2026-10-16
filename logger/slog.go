//go:build go1.21

package logger

import (
	"context"
	"log/slog"

	"github.com/insei/tinyconf"
)

type slogLogger struct {
	l *slog.Logger
}

func attrs(fields []tinyconf.Field) []any {
	args := make([]any, 0, len(fields))
	for _, field := range fields {
		args = append(args, slog.Any(field.Key, field.Value))
	}
	return args
}

func (s *slogLogger) log(level slog.Level, msg string, fields []tinyconf.Field) {
	s.l.Log(context.Background(), level, msg, attrs(fields)...)
}

// Debug logs a message at the DEBUG level.
func (s *slogLogger) Debug(msg string, fields ...tinyconf.Field) {
	s.log(slog.LevelDebug, msg, fields)
}

// Error logs a message at the ERROR level.
func (s *slogLogger) Error(msg string, fields ...tinyconf.Field) {
	s.log(slog.LevelError, msg, fields)
}

// Warn logs a message at the WARN level.
func (s *slogLogger) Warn(msg string, fields ...tinyconf.Field) {
	s.log(slog.LevelWarn, msg, fields)
}

// Info logs a message at the INFO level.
func (s *slogLogger) Info(msg string, fields ...tinyconf.Field) {
	s.log(slog.LevelInfo, msg, fields)
}

// With returns the logger with the fields.
func (s *slogLogger) With(fields ...tinyconf.Field) tinyconf.Logger {
	return &slogLogger{l: s.l.With(attrs(fields)...)}
}

// NewSlogLogger returns the logger of tinyconf that writes to the slog logger, it needs Go 1.21 or newer.
func NewSlogLogger(l *slog.Logger) tinyconf.Logger {
	return &slogLogger{l: l}
}
