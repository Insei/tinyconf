//go:build go1.21

package logger

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/insei/tinyconf/v2"
)

func TestSlogLogger(t *testing.T) {
	var out bytes.Buffer
	handler := slog.NewTextHandler(&out, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		// the time is removed for the stable output
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	logger := NewSlogLogger(slog.New(handler)).With(tinyconf.Field{Key: "driver", Value: "env"})
	logger.Debug("debug", tinyconf.Field{Key: "field", Value: "Name"})
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error", tinyconf.Field{Key: "details", Value: "failed"})
	want := "level=DEBUG msg=debug driver=env field=Name\n" +
		"level=INFO msg=info driver=env\n" +
		"level=WARN msg=warn driver=env\n" +
		"level=ERROR msg=error driver=env details=failed\n"
	if out.String() != want {
		t.Errorf("unexpected output:\n%s\nwant:\n%s", out.String(), want)
	}
}
