package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/insei/tinyconf"
)

// zapLogger is a simple logger that uses zap.Logger
type zapLogger struct {
	lvl    zapcore.Level
	logger *zap.Logger
	fields []tinyconf.Field
}

// zapLevelFromString converts a string to a zapcore.Level
func zapLevelFromString(s string) zapcore.Level {
	switch s {
	case "DEBUG":
		return zap.DebugLevel
	case "ERROR":
		return zap.ErrorLevel
	case "WARN":
		return zap.WarnLevel
	case "INFO":
		return zap.InfoLevel
	case "FATAL":
		return zap.FatalLevel
	}
	return zapcore.InfoLevel // Default to INFO level if the string is invalid
}

// msg logs a message with the correct level and fields
func (t zapLogger) msg(lvl, msg string, fld ...tinyconf.Field) {
	msgLvl := zapLevelFromString(lvl)
	if t.lvl < msgLvl {
		return
	}
	fields := make([]zap.Field, 0, len(fld)+len(t.fields))
	for _, f := range t.fields {
		fields = append(fields, zap.Any(f.Key, f.Value))
	}
	for _, f := range fld {
		fields = append(fields, zap.Any(f.Key, f.Value))
	}
	switch lvl {
	case "DEBUG":
		t.logger.Debug(msg, fields...)
	case "INFO":
		t.logger.Info(msg, fields...)
	case "WARN":
		t.logger.Warn(msg, fields...)
	case "ERROR":
		t.logger.Error(msg, fields...)
	case "FATAL":
		t.logger.Fatal(msg, fields...)
	}
}

// Debug logs a message at the DEBUG level
func (t zapLogger) Debug(msg string, fld ...tinyconf.Field) {
	t.msg("DEBUG", msg, fld...)
}

// Error logs a message at the ERROR level
func (t zapLogger) Error(msg string, fld ...tinyconf.Field) {
	t.msg("ERROR", msg, fld...)
}

// Warn logs a message at the WARN level
func (t zapLogger) Warn(msg string, fld ...tinyconf.Field) {
	t.msg("WARN", msg, fld...)
}

// Info logs a message at the INFO level
func (t zapLogger) Info(msg string, fld ...tinyconf.Field) {
	t.msg("INFO", msg, fld...)
}

// With adds fields to the logger
func (t zapLogger) With(flds ...tinyconf.Field) tinyconf.Logger {
	fields := make([]tinyconf.Field, 0, len(flds)+len(t.fields))
	copy(t.fields, fields)
	fields = append(t.fields, flds...)
	return &zapLogger{
		lvl:    t.lvl,
		logger: t.logger,
		fields: fields,
	}
}

// NewZapLogger creates a new zapLogger
func NewZapLogger(lvl string) tinyconf.Logger {
	zapLevel := zapLevelFromString(lvl)

	var config zapcore.EncoderConfig
	if zapLevel == zap.DebugLevel {
		config = zap.NewDevelopmentEncoderConfig()
	} else {
		config = zap.NewProductionEncoderConfig()
	}

	logger := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.Lock(os.Stdout),
		zapLevel,
	))

	return &zapLogger{
		lvl:    zapLevel,
		logger: logger,
	}
}
