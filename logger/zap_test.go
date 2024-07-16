package logger

import (
	"strconv"
	"testing"

	"go.uber.org/zap/zapcore"

	"github.com/insei/tinyconf"
)

func TestZapMsgMethod(t *testing.T) {
	lg := NewZapLogger("INFO")
	logger := lg.(*zapLogger)

	// Test with different levels
	t.Run("Debug", func(t *testing.T) {
		logger.msg("DEBUG", "Debug message", tinyconf.Field{Key: "key1", Value: "value1"})
	})

	t.Run("Info", func(t *testing.T) {
		logger.msg("INFO", "Info message", tinyconf.Field{Key: "key2", Value: "value2"})
	})

	t.Run("Warn", func(t *testing.T) {
		logger.msg("WARN", "Warn message", tinyconf.Field{Key: "key3", Value: "value3"})
	})

	t.Run("Error", func(t *testing.T) {
		logger.msg("ERROR", "Error message", tinyconf.Field{Key: "key4", Value: "value4"})
	})

	t.Run("Fatal", func(t *testing.T) {
		logger.msg("FATAL", "Fatal message", tinyconf.Field{Key: "key5", Value: "value5"})
	})

	// Test with different levels and fields
	t.Run("WithFields", func(t *testing.T) {
		logger.msg("INFO", "Info message with fields",
			tinyconf.Field{Key: "key1", Value: "value1"},
			tinyconf.Field{Key: "key2", Value: "value2"},
		)
	})

	// Test with level filter
	t.Run("LevelFilter", func(t *testing.T) {
		logger.lvl = zapcore.WarnLevel
		logger.msg("DEBUG", "Debug message (should be filtered)", tinyconf.Field{Key: "key1", Value: "value1"})
		logger.msg("WARN", "Warn message (should be logged)", tinyconf.Field{Key: "key1", Value: "value1"})
	})
}

func TestZapDebug(t *testing.T) {
	lg := NewZapLogger("DEBUG")
	logger := lg.(*zapLogger)

	logger.Debug("Debug message", tinyconf.Field{Key: "Login", Value: "world"})
}

func TestZapError(t *testing.T) {
	lg := NewZapLogger("ERROR")
	logger := lg.(*zapLogger)

	logger.Error("Error message", tinyconf.Field{Key: "Name", Value: "Eddy"})
}

func TestZapWarning(t *testing.T) {
	lg := NewZapLogger("WARN")
	logger := lg.(*zapLogger)

	logger.Warn("Warn message", tinyconf.Field{Key: "Table", Value: "admins"})
}

func TestZapInfo(t *testing.T) {
	lg := NewZapLogger("INFO")
	logger := lg.(*zapLogger)

	logger.Info("Info message", tinyconf.Field{Key: "Success", Value: "true"})
}

func TestZapWith(t *testing.T) {
	field := tinyconf.LogField("email", "test@test.com")
	logger := &zapLogger{lvl: zapcore.DebugLevel}

	newLogger := logger.With(field)

	if newLogger.(*zapLogger).fields[0] != field {
		t.Error("fields don't match in logger.With()")
	}
}

func TestZapLevelFromString(t *testing.T) {
	tests := []struct {
		in  string
		out zapcore.Level
	}{
		{"DEBUG", zapcore.DebugLevel},
		{"INFO", zapcore.InfoLevel},
		{"WARN", zapcore.WarnLevel},
		{"ERROR", zapcore.ErrorLevel},
		{"FATAL", zapcore.FatalLevel},
	}

	for _, tt := range tests {
		t.Run(strconv.Itoa(int(tt.out)), func(t *testing.T) {
			if result := zapLevelFromString(tt.in); result != tt.out {
				t.Errorf("Expected level '%d', but got '%d'", tt.out, result)
			}
		})
	}
}

func TestNewZapLogger(t *testing.T) {
	logger := NewZapLogger("DEBUG")

	_, ok := logger.(*zapLogger)
	if !ok {
		t.Error("The type of logger is not '*zapLogger'")
	}
}
