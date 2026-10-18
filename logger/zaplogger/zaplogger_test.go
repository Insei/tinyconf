package zaplogger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/insei/tinyconf/v2"
)

func TestLogger(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	logger := New(zap.New(core)).With(tinyconf.Field{Key: "driver", Value: "env"})
	logger.Debug("debug", tinyconf.Field{Key: "field", Value: "Name"})
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error", tinyconf.Field{Key: "details", Value: "failed"})

	entries := logs.All()
	assert.Len(t, entries, 4)
	for i, level := range []zapcore.Level{zapcore.DebugLevel, zapcore.InfoLevel, zapcore.WarnLevel, zapcore.ErrorLevel} {
		assert.Equal(t, level, entries[i].Level)
		assert.Equal(t, level.String(), entries[i].Message)
		assert.Equal(t, "env", entries[i].ContextMap()["driver"])
	}
	assert.Equal(t, "Name", entries[0].ContextMap()["field"])
	assert.Equal(t, "failed", entries[3].ContextMap()["details"])
}
