package tinyconf

import (
	"testing"
)

func TestLogField(t *testing.T) {
	testCases := []struct {
		name          string
		key           string
		value         interface{}
		expectedKey   string
		expectedValue interface{}
	}{
		{
			"Empty Key and Value",
			"",
			nil,
			"",
			nil,
		},
		{
			"Valid Key No Value",
			"testKey",
			nil,
			"testKey",
			nil,
		},
		{
			"No Key Valid Value",
			"",
			"testValue",
			"",
			"testValue",
		},
		{
			"Valid Key and Value",
			"testKey",
			"testValue",
			"testKey",
			"testValue",
		},
		{
			"Special Characters Key and Value",
			"@#%&*()",
			"!@#$%^&*()",
			"@#%&*()",
			"!@#$%^&*()",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			field := LogField(tc.key, tc.value)
			if field.Key != tc.expectedKey {
				t.Errorf("wantErr key %s, got %s", tc.expectedKey, field.Key)
			}
			if field.Value != tc.expectedValue {
				t.Errorf("wantErr value %v, got %v", tc.expectedValue, field.Value)
			}
		})
	}
}

// TestNoopLoggerDebug is a table driven test for the method Debug of noopLogger
func TestNoopLoggerDebug(t *testing.T) {
	var l Logger = &noopLogger{}
	l.Debug("test debug message")
	l.Info("test info message")
	l.Warn("test warn message")
	l.Error("test error message")
	l = l.With(LogField("empty", "value"))
	l.Debug("test debug message")
	l.Info("test info message")
	l.Warn("test warn message")
	l.Error("test error message")
}
