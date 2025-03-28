package tinyconf

import (
	"errors"
	"reflect"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
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
		getField func() fmap.Field
		val      any
		expected string
	}{
		{
			name: "hidden_tag_is_not_set",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test string
				}]()
				return storage.MustFind("Test")
			},
			val:      any(42),
			expected: "42",
		},
		{
			name: "hidden_tag_set_to_false",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test string `hidden:"false"`
				}]()
				return storage.MustFind("Test")
			},
			val:      any(1001),
			expected: "1001",
		},
		{
			name: "hidden_tag_set_to_true",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test string `hidden:"true"`
				}]()
				return storage.MustFind("Test")
			},
			val:      any(1234567890),
			expected: "**********",
		},
		{
			name: "hidden_tag_set_to_true_string_value",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test string `hidden:"true"`
				}]()
				return storage.MustFind("Test")
			},
			val:      any("mysecretvalue"),
			expected: "*************",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := getLoggerValue(c.getField(), c.val)
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
				registered: map[reflect.Type]*Registered{},
			},
		},
		{
			name: "With Driver Option",
			opts: []Option{
				WithDriver(&mockDriver{}),
			},
			expected: &Manager{
				log:        &noopLogger{},
				registered: map[reflect.Type]*Registered{},
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
				registered: map[reflect.Type]*Registered{},
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
				registered: map[reflect.Type]*Registered{},
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
				registered: map[reflect.Type]*Registered{},
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

func (md *parseMockDriver) GenDoc(registers ...*Registered) string {
	return "doc parseMockDriver"
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

func TestManager_ParseSubConfig(t *testing.T) {
	type FieldConfig struct {
		Test  string
		Test2 string
	}
	type Config struct {
		Field FieldConfig
	}
	testCases := []struct {
		Name                string
		Drivers             []Driver
		expectedFieldValues any
		config              *FieldConfig
	}{
		{
			Name: "Simple value",
			Drivers: []Driver{
				&parseMockDriver{
					value: "test",
				},
			},
			expectedFieldValues: "test",
			config:              &FieldConfig{},
		},
		{
			Name: "Driver no value, no override existing fields values",
			Drivers: []Driver{
				&parseMockDriver{
					value: "",
					err:   ErrValueNotFound,
				},
			},
			expectedFieldValues: "test123",
			config: &FieldConfig{
				Test:  "test123",
				Test2: "test123",
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			c := &Manager{
				drivers:    testCase.Drivers,
				registered: make(map[reflect.Type]*Registered),
				log:        &testLogger{},
			}
			conf := &Config{Field: FieldConfig{}}
			assert.NoError(t, c.Register(conf))
			assert.NoError(t, c.Parse(testCase.config))
			assert.Equal(t, testCase.expectedFieldValues, testCase.config.Test)
			assert.Equal(t, testCase.expectedFieldValues, testCase.config.Test2)
		})
	}
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
			Name:              "not Registered config",
			Config:            &struct{}{},
			Registered:        false,
			Drivers:           nil,
			ExpectErrorLogged: false,
			ExpectWarnLogged:  false,
			ExpectDebugLogged: false,
		},
		{
			Name:              "Registered config, no drivers",
			Config:            &struct{}{},
			Registered:        true,
			Drivers:           nil,
			ExpectErrorLogged: false,
			ExpectWarnLogged:  false,
			ExpectDebugLogged: false,
		},
		{
			Name:       "Registered config, drivers, one err",
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
			Name:       "Registered config, drivers, values",
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
		{
			Name:       "Registered config, drivers, values",
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
				registered: make(map[reflect.Type]*Registered),
				log:        &testLogger{},
			}

			assert.NoError(t, c.Register(tc.Config))

			_ = c.Parse(tc.Config)

			logger := c.log.(*testLogger)

			assert.Equal(t, tc.ExpectWarnLogged, logger.WarnLogged)
			assert.Equal(t, tc.ExpectErrorLogged, logger.ErrorLogged)
			assert.Equal(t, tc.ExpectDebugLogged, logger.DebugLogged)
		})
	}
}

func TestManager_GenDoc(t *testing.T) {
	type fields struct {
		drivers    []Driver
		log        Logger
		registered map[reflect.Type]*Registered
	}
	type args struct {
		driverName string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
	}{
		{
			name: "success",
			fields: fields{
				drivers: []Driver{
					&parseMockDriver{name: "mockDriver", err: nil},
					&parseMockDriver{name: "newMockDriver", err: nil},
					&mockDriver{},
				},
				log:        nil,
				registered: nil,
			},
			args: args{
				driverName: "mockDriver",
			},
			want: "doc mockDriver",
		},
		{
			name: "not found driver",
			fields: fields{
				drivers: []Driver{
					&parseMockDriver{name: "mockDriver", err: nil},
					&parseMockDriver{name: "newMockDriver", err: nil},
					&mockDriver{},
				},
				log:        nil,
				registered: nil,
			},
			args: args{
				driverName: "testDriver",
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Manager{
				drivers:    tt.fields.drivers,
				log:        tt.fields.log,
				registered: tt.fields.registered,
			}
			assert.Equalf(t, tt.want, c.GenDoc(tt.args.driverName), "GenDoc(%v)", tt.args.driverName)
		})
	}
}
