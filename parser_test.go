package tinyconf

import (
	"errors"
	"github.com/insei/fmap/v2"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func Test_checkConfig(t *testing.T) {
	type testData struct{}
	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{
			name:    "Nil Input",
			input:   nil,
			wantErr: true,
		},
		{
			name:    "Invalid Type Int",
			input:   123,
			wantErr: true,
		},
		{
			name:    "Invalid Type String",
			input:   "test",
			wantErr: true,
		},
		{
			name:    "Valid Pointer Class",
			input:   new(testData),
			wantErr: false,
		},
		{
			name:    "Invalid Non-Pointer Class",
			input:   testData{},
			wantErr: true,
		},
		{
			name:    "Invalid Pointer non-Class",
			input:   new(int),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkConfig(tt.input)
			if !tt.wantErr && result != nil {
				t.Errorf("Unexpected error for %v", tt.name)
			}
			if tt.wantErr && result == nil {
				t.Errorf("Expected an error for %v", tt.name)
			}
		})
	}
}

func TestManager_Register(t *testing.T) {
	type test struct {
		name      string
		conf      any
		wantError bool
	}

	tests := []test{
		{
			name:      "ValidConfig",
			conf:      &struct{ Test string }{}, // Assume a valid config
			wantError: false,
		},
		{
			name:      "InvalidConfig",
			conf:      new(int), // Assume an invalid config
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, _ := New()
			err := manager.Register(tt.conf)

			if (err != nil) != tt.wantError {
				t.Errorf("Manager.Register() error = %v, wantError %v", err, tt.wantError)
			}

			if !tt.wantError {
				if _, ok := manager.registered[reflect.TypeOf(tt.conf)]; !ok {
					t.Errorf("Manager.Register() did not register the config properly")
				}
			}
		})
	}
}

func TestGetDereferencedValue(t *testing.T) {
	type test struct {
		name string
		val  any
		want any
	}

	tests := []test{
		{
			name: "Test normal integer",
			val:  15,
			want: 15,
		},
		{
			name: "Test pointer to integer",
			val:  new(int),
			want: 0,
		},
		{
			name: "Test string",
			val:  "hello",
			want: "hello",
		},
		{
			name: "Test pointer to string",
			val:  new(string),
			want: "",
		},
		{
			name: "Test nil value",
			val:  nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDereferencedValue(tt.val)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getDereferencedValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLoggerValue(t *testing.T) {
	cases := []struct {
		name     string
		field    fmap.Field
		val      any
		expected string
	}{
		{
			name: "hidden_tag_is_not_set",
			field: fmap.Get[struct {
				Test string
			}]()["Test"],
			val:      any(42),
			expected: "42",
		},
		{
			name: "hidden_tag_set_to_false",
			field: fmap.Get[struct {
				Test string `hidden:"false"`
			}]()["Test"],
			val:      any(1001),
			expected: "1001",
		},
		{
			name: "hidden_tag_set_to_true",
			field: fmap.Get[struct {
				Test int `hidden:"true"`
			}]()["Test"],
			val:      any(1234567890),
			expected: "**********",
		},
		{
			name: "hidden_tag_set_to_true_string_value",
			field: fmap.Get[struct {
				Test string `hidden:"true"`
			}]()["Test"],
			val:      any("mysecretvalue"),
			expected: "*************",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := getLoggerValue(c.field, c.val)
			if result != c.expected {
				t.Errorf("Expected '%s', got '%s'", c.expected, result)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		opts     []Option
		expected *Manager
	}{
		{
			name: "Empty Options",
			opts: []Option{},
			expected: &Manager{
				log:        &noopLogger{},
				registered: map[reflect.Type]map[string]fmap.Field{},
			},
		},
		{
			name: "With Driver Option",
			opts: []Option{
				WithDriver(&mockDriver{}),
			},
			expected: &Manager{
				log:        &noopLogger{},
				registered: map[reflect.Type]map[string]fmap.Field{},
				drivers: []Driver{
					&mockDriver{},
				},
			},
		},
		{
			name: "With Logger Option",
			opts: []Option{
				WithLogger(&mockLogger{}),
			},
			expected: &Manager{
				log:        &mockLogger{},
				registered: map[reflect.Type]map[string]fmap.Field{},
			},
		},
		{
			name: "With Multiple Different Options",
			opts: []Option{
				WithDriver(&mockDriver{}),
				WithLogger(&mockLogger{}),
			},
			expected: &Manager{
				log:        &mockLogger{},
				registered: map[reflect.Type]map[string]fmap.Field{},
				drivers: []Driver{
					&mockDriver{},
				},
			},
		},
		{
			name: "With Multiple Identical Options",
			opts: []Option{
				WithDriver(&mockDriver{}),
				WithDriver(&mockDriver{}),
			},
			expected: &Manager{
				log:        &noopLogger{},
				registered: map[reflect.Type]map[string]fmap.Field{},
				drivers: []Driver{
					&mockDriver{},
					&mockDriver{},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := New(test.opts...)
			assert.NoError(t, err)
			assert.Equal(t, test.expected, result)
		})
	}
}

type parseMockDriver struct {
	name  string
	err   error
	value any
}

func (md *parseMockDriver) GetName() string {
	return md.name
}

func (md *parseMockDriver) GetValue(field fmap.Field) (*Value, error) {
	return &Value{Source: "mock", Value: md.value}, md.err
}

type testLogger struct {
	ErrorLogged bool
	WarnLogged  bool
	DebugLogged bool
}

func (tl *testLogger) Error(_ string, _ ...Field) {
	tl.ErrorLogged = true
}

func (tl *testLogger) Debug(_ string, _ ...Field) {
	tl.DebugLogged = true
}

func (tl *testLogger) Warn(_ string, _ ...Field) {
	tl.WarnLogged = true
}
func (tl *testLogger) Info(_ string, _ ...Field) {
	tl.WarnLogged = true
}

func (tl *testLogger) With(_ ...Field) Logger {
	return tl
}

func TestManager_Parse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name              string
		Config            any
		Registered        bool
		Drivers           []Driver
		ExpectErrorLogged bool
		ExpectWarnLogged  bool
		ExpectDebugLogged bool
	}{
		{
			Name:              "not registered config",
			Config:            &struct{}{},
			Registered:        false,
			Drivers:           nil,
			ExpectErrorLogged: false,
			ExpectWarnLogged:  false,
			ExpectDebugLogged: false,
		},
		{
			Name:              "registered config, no drivers",
			Config:            &struct{}{},
			Registered:        true,
			Drivers:           nil,
			ExpectErrorLogged: false,
			ExpectWarnLogged:  false,
			ExpectDebugLogged: false,
		},
		{
			Name:       "registered config, drivers, one err",
			Config:     &struct{ Test string }{},
			Registered: true,
			Drivers: []Driver{
				&parseMockDriver{name: "d1", err: ErrValueNotFound},
				&parseMockDriver{name: "d2", err: ErrIncorrectTagSettings},
				&parseMockDriver{name: "d3", err: errors.New("random error")},
			},
			ExpectErrorLogged: true,
			ExpectWarnLogged:  true,
			ExpectDebugLogged: true,
		},
		{
			Name:       "registered config, drivers, values",
			Config:     &struct{ Test struct{ Test int } }{},
			Registered: true,
			Drivers: []Driver{
				&parseMockDriver{name: "d1", value: 1},
				&parseMockDriver{name: "d2", value: 3},
				&parseMockDriver{name: "d3", value: 3},
			},
			ExpectErrorLogged: false,
			ExpectWarnLogged:  false,
			ExpectDebugLogged: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			c := &Manager{
				drivers:    tc.Drivers,
				registered: make(map[reflect.Type]map[string]fmap.Field),
				log:        &testLogger{},
			}
			if tc.Registered {
				c.registered[reflect.TypeOf(tc.Config)] = fmap.GetFrom(tc.Config)
			}

			_ = c.Parse(tc.Config)

			logger := c.log.(*testLogger)

			assert.Equal(t, tc.ExpectWarnLogged, logger.WarnLogged)
			assert.Equal(t, tc.ExpectErrorLogged, logger.ErrorLogged)
			assert.Equal(t, tc.ExpectDebugLogged, logger.DebugLogged)
		})
	}
}
