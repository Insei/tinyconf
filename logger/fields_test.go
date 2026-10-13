package logger

import (
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/insei/tinyconf"
)

func TestFmtLogger_Fields(t *testing.T) {
	logger := NewFmtLogger(DEBUG).With(tinyconf.Field{Key: "driver", Value: "env"})
	out := captureStdout(func() {
		logger.Info("message", tinyconf.Field{Key: "field", Value: "Name"})
	})
	for _, part := range []string{"[INFO] message", "{driver: env}", "{field: Name}"} {
		if !strings.Contains(out, part) {
			t.Errorf("Expected output containing '%s', but got '%s'", part, out)
		}
	}
}

func TestZapLogger_With(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	logger := NewZapLogger(zap.New(core)).With(tinyconf.Field{Key: "driver", Value: "env"})
	logger.Info("message", tinyconf.Field{Key: "field", Value: "Name"})

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 log entry, but got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if entries[0].Message != "message" || fields["driver"] != "env" || fields["field"] != "Name" {
		t.Errorf("Unexpected log entry: %s %v", entries[0].Message, fields)
	}
}
