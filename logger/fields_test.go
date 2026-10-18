package logger

import (
	"strings"
	"testing"

	"github.com/insei/tinyconf/v2"
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
