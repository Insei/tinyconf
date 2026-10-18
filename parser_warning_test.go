package tinyconf

import (
	"context"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

type warningMockDriver struct{}

func (md *warningMockDriver) GenDoc(...*Registered) string { return "" }
func (md *warningMockDriver) GetName() string              { return "warning" }
func (md *warningMockDriver) GetValue(field fmap.Field) (*Value, error) {
	if field.GetStructPath() == "Suspicious" {
		return &Value{Source: "mock", Value: "value", Warning: "value is read from the unexpected place"}, nil
	}
	return &Value{Source: "mock", Value: "value"}, nil
}

type recordLogger struct {
	fields   []Field
	warnings *[]map[string]any
}

func (l *recordLogger) Error(string, ...Field) {}
func (l *recordLogger) Debug(string, ...Field) {}
func (l *recordLogger) Info(string, ...Field)  {}
func (l *recordLogger) Warn(msg string, fields ...Field) {
	record := map[string]any{"msg": msg}
	for _, field := range append(append([]Field{}, l.fields...), fields...) {
		record[field.Key] = field.Value
	}
	*l.warnings = append(*l.warnings, record)
}

func (l *recordLogger) With(fields ...Field) Logger {
	return &recordLogger{fields: append(append([]Field{}, l.fields...), fields...), warnings: l.warnings}
}

func TestManager_Parse_ValueWarning(t *testing.T) {
	type Config struct {
		Usual      string
		Suspicious string
	}
	var warnings []map[string]any
	c, err := New(WithDriver(&warningMockDriver{}), WithLogger(&recordLogger{warnings: &warnings}))
	assert.NoError(t, err)
	conf := &Config{}
	assert.NoError(t, c.Register(conf))
	assert.NoError(t, c.Parse(context.Background(), conf))

	// the value is set, the warning of the driver is logged
	assert.Equal(t, Config{Usual: "value", Suspicious: "value"}, *conf)
	assert.Equal(t, []map[string]any{{
		"msg":     "warning",
		"config":  "*tinyconf.Config",
		"driver":  "warning",
		"field":   "Suspicious",
		"details": "value is read from the unexpected place",
	}}, warnings)
}
