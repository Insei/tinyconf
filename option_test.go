package tinyconf

import (
	"github.com/insei/fmap/v2"
	"reflect"
	"testing"
)

// Mock implementations of Driver interface for the purposes of testing
type mockDriver struct{}

func (md mockDriver) GetName() string { return "mockDriver" }
func (md mockDriver) GetValue(field fmap.Field) (*Value, error) {
	return nil, nil
}

// Mock implementations of Logger interface for the purposes of testing
type mockLogger struct{}

func (l *mockLogger) Debug(string, ...Field) {}
func (l *mockLogger) Error(string, ...Field) {}
func (l *mockLogger) Warn(string, ...Field)  {}
func (l *mockLogger) Info(string, ...Field)  {}
func (l *mockLogger) With(...Field) Logger   { return l }

func TestCountDrivers(t *testing.T) {
	testCases := []struct {
		name  string
		opts  []Option
		count uint8
	}{
		{
			name:  "NoDrivers",
			opts:  []Option{},
			count: 0,
		},
		{
			name:  "SingleDriver",
			opts:  []Option{WithDriver(mockDriver{})},
			count: 1,
		},
		{
			name:  "MultipleDrivers",
			opts:  []Option{WithDriver(mockDriver{}), WithDriver(mockDriver{}), WithLogger(&noopLogger{})},
			count: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if count := countDrivers(tc.opts...); count != tc.count {
				t.Errorf("countDrivers(%v) = %v, expected %v", tc.opts, count, tc.count)
			}
		})
	}
}

func TestApply(t *testing.T) {
	tests := []struct {
		name           string
		opts           driverOption
		expectedConfig *Manager
	}{
		{
			name:           "driver_present",
			opts:           driverOption{driver: &mockDriver{}},
			expectedConfig: &Manager{drivers: []Driver{&mockDriver{}}},
		},
		{
			name:           "driver_absent",
			opts:           driverOption{driver: nil},
			expectedConfig: &Manager{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			managerConfig := &Manager{}
			tt.opts.apply(managerConfig)
			if !reflect.DeepEqual(managerConfig, tt.expectedConfig) {
				t.Errorf("managerConfig got %v, want %v", managerConfig, tt.expectedConfig)
			}
		})
	}
}

func TestLoggerOption_apply(t *testing.T) {
	tests := []struct {
		name     string
		logger   Logger
		expected Logger
	}{
		{
			name:     "With valid logger",
			logger:   &noopLogger{},
			expected: &noopLogger{},
		},
		{
			name:     "With nil logger",
			logger:   nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := loggerOption{
				logger: tt.logger,
			}
			manager := &Manager{}

			opt.apply(manager)

			if manager.log != tt.expected {
				t.Errorf("Expected: %v, but got: %v", tt.expected, manager.log)
			}
		})
	}
}
