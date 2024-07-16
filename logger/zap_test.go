package logger

import (
	"os"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/insei/tinyconf"
)

func TestZapDebug(t *testing.T) {
	var config zapcore.EncoderConfig
	config = zap.NewDevelopmentEncoderConfig()

	zapLg := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.Lock(os.Stdout),
		zapcore.DebugLevel,
	))
	lg := NewZapLogger(zapLg)
	logger := lg.(*zapLogger)

	logger.Debug("Debug message", tinyconf.Field{Key: "Login", Value: "world"})
}

func TestZapError(t *testing.T) {
	var config zapcore.EncoderConfig
	config = zap.NewDevelopmentEncoderConfig()

	zapLg := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.Lock(os.Stdout),
		zapcore.ErrorLevel,
	))
	lg := NewZapLogger(zapLg)
	logger := lg.(*zapLogger)

	logger.Error("Error message", tinyconf.Field{Key: "Name", Value: "Eddy"})
}

func TestZapWarning(t *testing.T) {
	var config zapcore.EncoderConfig
	config = zap.NewDevelopmentEncoderConfig()

	zapLg := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.Lock(os.Stdout),
		zapcore.WarnLevel,
	))
	lg := NewZapLogger(zapLg)
	logger := lg.(*zapLogger)

	logger.Warn("Warn message", tinyconf.Field{Key: "Table", Value: "admins"})
}

func TestZapInfo(t *testing.T) {
	var config zapcore.EncoderConfig
	config = zap.NewDevelopmentEncoderConfig()

	zapLg := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.Lock(os.Stdout),
		zapcore.InfoLevel,
	))
	lg := NewZapLogger(zapLg)
	logger := lg.(*zapLogger)

	logger.Info("Info message", tinyconf.Field{Key: "Success", Value: "true"})
}

func TestNewZapLogger(t *testing.T) {
	var config zapcore.EncoderConfig
	config = zap.NewDevelopmentEncoderConfig()

	zapLg := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.Lock(os.Stdout),
		zapcore.InfoLevel,
	))
	logger := NewZapLogger(zapLg)
	_, ok := logger.(*zapLogger)
	if !ok {
		t.Error("The type of l is not '*zapLogger'")
	}
}
