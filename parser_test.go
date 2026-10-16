package tinyconf

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

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

type pathMockDriver struct {
	values    map[string]any
	errs      map[string]error
	requested []string
}

func (md *pathMockDriver) GenDoc(registers ...*Registered) string {
	return "doc pathMockDriver"
}

func (md *pathMockDriver) GetName() string {
	return "pathMockDriver"
}

func (md *pathMockDriver) GetValue(field fmap.Field) (*Value, error) {
	path := field.GetStructPath()
	md.requested = append(md.requested, path)
	if err, ok := md.errs[path]; ok {
		return nil, err
	}
	if value, ok := md.values[path]; ok {
		return &Value{Source: "mock", Value: value}, nil
	}
	return nil, ErrValueNotFound
}

func newPathMockManager(driver *pathMockDriver) *Manager {
	return &Manager{
		drivers:    []Driver{driver},
		registered: make(map[reflect.Type]*Registered),
		log:        &testLogger{},
	}
}

func TestManager_Parse_UncomparableValues(t *testing.T) {
	type Config struct {
		List []any
		Map  map[string]any
	}
	c := newPathMockManager(&pathMockDriver{values: map[string]any{
		"List": []any{"a", "b"},
		"Map":  map[string]any{"a": 1},
	}})
	conf := &Config{}
	assert.NoError(t, c.Register(conf))
	assert.NotPanics(t, func() {
		assert.NoError(t, c.Parse(conf))
	})
	assert.Equal(t, []any{"a", "b"}, conf.List)
	assert.Equal(t, map[string]any{"a": 1}, conf.Map)
}

func TestManager_Parse_InvalidConfig(t *testing.T) {
	type Config struct {
		Test string
	}
	testCases := []struct {
		Name   string
		Config any
	}{
		{Name: "Nil", Config: nil},
		{Name: "Nil pointer of registered type", Config: (*Config)(nil)},
		{Name: "Non-pointer", Config: Config{}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			c := newPathMockManager(&pathMockDriver{values: map[string]any{"Test": "test"}})
			assert.NoError(t, c.Register(&Config{}))
			assert.NotPanics(t, func() {
				assert.Error(t, c.Parse(testCase.Config))
			})
		})
	}
}

func TestManager_Parse_DriverErrors(t *testing.T) {
	type Config struct {
		Host string
		Port int
	}
	driverErr := errors.New("random error")
	testCases := []struct {
		Name      string
		Err       error
		WantError bool
	}{
		{Name: "Driver error is returned", Err: driverErr, WantError: true},
		{Name: "Value not found is not an error", Err: ErrValueNotFound, WantError: false},
		{Name: "Incorrect tag settings is not an error", Err: ErrIncorrectTagSettings, WantError: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			c := newPathMockManager(&pathMockDriver{
				values: map[string]any{"Host": "localhost"},
				errs:   map[string]error{"Port": testCase.Err},
			})
			conf := &Config{}
			assert.NoError(t, c.Register(conf))
			err := c.Parse(conf)
			if testCase.WantError {
				assert.ErrorIs(t, err, driverErr)
				assert.Contains(t, err.Error(), "Port")
				assert.True(t, c.log.(*testLogger).ErrorLogged)
			} else {
				assert.NoError(t, err)
			}
			// other fields are parsed anyway
			assert.Equal(t, "localhost", conf.Host)
		})
	}
}

func TestManager_ParseSubConfig_SiblingPrefix(t *testing.T) {
	type HTTP struct {
		Port int
	}
	type HTTPS struct {
		Port int
	}
	type Config struct {
		HTTP  HTTP
		HTTPS HTTPS
	}
	testCases := []struct {
		Name   string
		Driver *pathMockDriver
	}{
		{
			Name: "Sibling with the same prefix has value",
			Driver: &pathMockDriver{
				values: map[string]any{"HTTP.Port": 1, "HTTPS.Port": 2},
			},
		},
		{
			Name: "Sibling with the same prefix has error",
			Driver: &pathMockDriver{
				values: map[string]any{"HTTP.Port": 1},
				errs:   map[string]error{"HTTPS.Port": errors.New("random error")},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			c := newPathMockManager(testCase.Driver)
			assert.NoError(t, c.Register(&Config{}))
			conf := &HTTP{}
			assert.NoError(t, c.Parse(conf))
			assert.Equal(t, 1, conf.Port)
		})
	}
}

func TestManager_ParseSubConfig_RepeatedName(t *testing.T) {
	type Inner struct {
		Token string
	}
	type Auth struct {
		Auth Inner
	}
	type Config struct {
		Auth Auth
	}
	c := newPathMockManager(&pathMockDriver{values: map[string]any{"Auth.Auth.Token": "token"}})
	assert.NoError(t, c.Register(&Config{}))
	conf := &Auth{}
	assert.NoError(t, c.Parse(conf))
	assert.Equal(t, "token", conf.Auth.Token)
}

func TestManager_ParseSubConfig_ZeroValue(t *testing.T) {
	type HTTP struct {
		Host string
		Port int
	}
	type Config struct {
		HTTP HTTP
	}
	c := newPathMockManager(&pathMockDriver{values: map[string]any{"HTTP.Host": "", "HTTP.Port": 0}})
	assert.NoError(t, c.Register(&Config{}))
	conf := &HTTP{Host: "default", Port: 8080}
	assert.NoError(t, c.Parse(conf))
	assert.Equal(t, "", conf.Host)
	assert.Equal(t, 0, conf.Port)
}

type parseLevel int

func (l *parseLevel) UnmarshalText(text []byte) error {
	*l = parseLevel(len(text))
	return nil
}

type parseTextStruct struct {
	Value string
	inner struct{ value string }
}

func (s *parseTextStruct) UnmarshalText(text []byte) error {
	s.Value = string(text)
	return nil
}

func TestManager_Parse_Types(t *testing.T) {
	type Nested struct {
		At      time.Time
		Timeout time.Duration
	}
	type Config struct {
		At       time.Time
		AtPtr    *time.Time
		Timeout  time.Duration
		Level    parseLevel
		Text     parseTextStruct
		Named    testNamedString
		NamedPtr *testNamedInt
		Hosts    []string
		Labels   map[string]int
		Nested   Nested
	}
	at := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	namedInt := testNamedInt(42)
	expected := Config{
		At:       at,
		AtPtr:    &at,
		Timeout:  5 * time.Second,
		Level:    parseLevel(4),
		Text:     parseTextStruct{Value: "text"},
		Named:    "named",
		NamedPtr: &namedInt,
		Hosts:    []string{"a", "b"},
		Labels:   map[string]int{"a": 1},
		Nested:   Nested{At: at, Timeout: time.Minute},
	}
	driver := &pathMockDriver{values: map[string]any{
		"At":             expected.At,
		"AtPtr":          expected.AtPtr,
		"Timeout":        expected.Timeout,
		"Level":          expected.Level,
		"Text":           expected.Text,
		"Named":          expected.Named,
		"NamedPtr":       expected.NamedPtr,
		"Hosts":          expected.Hosts,
		"Labels":         expected.Labels,
		"Nested.At":      expected.Nested.At,
		"Nested.Timeout": expected.Nested.Timeout,
	}}
	c := newPathMockManager(driver)
	conf := &Config{}
	assert.NoError(t, c.Register(conf))
	assert.NoError(t, c.Parse(conf))
	assert.Equal(t, expected, *conf)
	// fields of the types that are parsed from a single value are not requested from drivers
	assert.NotContains(t, driver.requested, "At.wall")
	assert.NotContains(t, driver.requested, "Text.Value")
	assert.NotContains(t, driver.requested, "Text.inner.value")
	assert.NotContains(t, driver.requested, "Nested")

	t.Run("sub config", func(t *testing.T) {
		sub := &Nested{}
		assert.NoError(t, c.Parse(sub))
		assert.Equal(t, expected.Nested, *sub)
	})
}

type testNamedString string
type testNamedInt int

type loaderMockDriver struct {
	pathMockDriver
	loaded  int
	loadErr error
}

func (md *loaderMockDriver) Load() error {
	md.loaded++
	return md.loadErr
}

func TestManager_Parse_Loader(t *testing.T) {
	type Config struct {
		Host string
		Port int
	}
	loadErr := errors.New("load error")
	failed := &loaderMockDriver{
		pathMockDriver: pathMockDriver{values: map[string]any{"Host": "failed"}},
		loadErr:        loadErr,
	}
	loaded := &loaderMockDriver{pathMockDriver: pathMockDriver{values: map[string]any{"Port": 80}}}
	c := &Manager{
		drivers:    []Driver{&pathMockDriver{values: map[string]any{"Host": "localhost"}}, loaded, failed},
		registered: make(map[reflect.Type]*Registered),
		log:        &testLogger{},
	}
	conf := &Config{}
	assert.NoError(t, c.Register(conf))

	err := c.Parse(conf)
	assert.ErrorIs(t, err, loadErr)
	assert.True(t, c.log.(*testLogger).ErrorLogged)
	// values of the failed driver are not used
	assert.Equal(t, Config{Host: "localhost", Port: 80}, *conf)
	assert.Empty(t, failed.requested)

	assert.Error(t, c.Parse(conf))
	// Load is called once per Parse
	assert.Equal(t, 2, loaded.loaded)
	assert.Equal(t, 2, failed.loaded)
}

type namedPathMockDriver struct {
	pathMockDriver
	name string
}

func (md *namedPathMockDriver) GetName() string {
	return md.name
}

func (md *namedPathMockDriver) GetValue(field fmap.Field) (*Value, error) {
	val, err := md.pathMockDriver.GetValue(field)
	if val != nil {
		val.Source = md.name + ":" + field.GetStructPath()
	}
	return val, err
}

func (md *namedPathMockDriver) GenDoc(registers ...*Registered) string {
	doc := ""
	for _, register := range registers {
		doc += reflect.TypeOf(register.Config).String() + ";"
	}
	return doc
}

func TestManager_Parse_Required(t *testing.T) {
	type HTTP struct {
		Host  string   `required:"true"`
		Port  int      `required:"true"`
		Hosts []string `required:"true"`
		Debug bool     `required:"false"`
		Name  string
	}
	type Config struct {
		HTTP  HTTP
		Token *string `required:"true"`
	}
	token := "token"
	testCases := []struct {
		Name    string
		Values  map[string]any
		Config  Config
		Missing []string
	}{
		{
			Name:    "Nothing is set",
			Missing: []string{"HTTP.Host", "HTTP.Port", "HTTP.Hosts", "Token"},
		},
		{
			Name:   "Values from driver",
			Values: map[string]any{"HTTP.Host": "localhost", "HTTP.Port": 80, "HTTP.Hosts": []string{"a"}, "Token": &token},
		},
		{
			Name:   "Zero values from driver",
			Values: map[string]any{"HTTP.Host": "", "HTTP.Port": 0, "HTTP.Hosts": []string{}, "Token": (*string)(nil)},
		},
		{
			Name:   "Default values in config",
			Config: Config{HTTP: HTTP{Host: "localhost", Port: 80, Hosts: []string{"a"}}, Token: &token},
		},
		{
			Name:    "Empty slice in config",
			Config:  Config{HTTP: HTTP{Host: "localhost", Port: 80, Hosts: []string{}}, Token: &token},
			Missing: []string{"HTTP.Hosts"},
		},
		{
			Name:    "Partially set",
			Values:  map[string]any{"HTTP.Host": "localhost"},
			Config:  Config{HTTP: HTTP{Hosts: []string{"a"}}},
			Missing: []string{"HTTP.Port", "Token"},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			c := newPathMockManager(&pathMockDriver{values: testCase.Values})
			conf := testCase.Config
			assert.NoError(t, c.Register(&conf))
			err := c.Parse(&conf)
			if len(testCase.Missing) == 0 {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrRequired)
				assert.Len(t, err.(parseErrors), len(testCase.Missing))
				for _, path := range testCase.Missing {
					assert.Contains(t, err.Error(), "field "+path+": ")
				}
			}

			t.Run("sub config", func(t *testing.T) {
				sub := testCase.Config.HTTP
				err := c.Parse(&sub)
				var missing []string
				for _, path := range testCase.Missing {
					if strings.HasPrefix(path, "HTTP.") {
						missing = append(missing, strings.TrimPrefix(path, "HTTP."))
					}
				}
				if len(missing) == 0 {
					assert.NoError(t, err)
					return
				}
				assert.ErrorIs(t, err, ErrRequired)
				assert.Len(t, err.(parseErrors), len(missing))
				for _, path := range missing {
					assert.Contains(t, err.Error(), "field "+path+": ")
				}
			})
		})
	}
}

func TestManager_Sources(t *testing.T) {
	type HTTP struct {
		Host string
		Port int
	}
	type Config struct {
		Name string
		HTTP HTTP
		Skip string
	}
	first := &namedPathMockDriver{name: "first", pathMockDriver: pathMockDriver{
		values: map[string]any{"HTTP.Port": 80, "HTTP.Host": "localhost", "Name": "first"},
		errs:   map[string]error{},
	}}
	second := &namedPathMockDriver{name: "second", pathMockDriver: pathMockDriver{
		values: map[string]any{"HTTP.Port": 8080},
	}}
	c := &Manager{
		drivers:    []Driver{first, second},
		registered: make(map[reflect.Type]*Registered),
		log:        &testLogger{},
	}
	conf := &Config{}
	assert.NoError(t, c.Register(conf))
	assert.Nil(t, c.Sources(conf))

	assert.NoError(t, c.Parse(conf))
	expected := []FieldSource{
		{Field: "Name", Driver: "first", Source: "first:Name"},
		{Field: "HTTP.Host", Driver: "first", Source: "first:HTTP.Host"},
		{Field: "HTTP.Port", Driver: "second", Source: "second:HTTP.Port"},
	}
	sources := c.Sources(conf)
	assert.Equal(t, expected, sources)

	// returned slice is a copy
	sources[0].Driver = "changed"
	assert.Equal(t, expected, c.Sources(conf))

	t.Run("sub config", func(t *testing.T) {
		sub := &HTTP{}
		assert.Nil(t, c.Sources(sub))
		assert.NoError(t, c.Parse(sub))
		assert.Equal(t, []FieldSource{
			{Field: "Host", Driver: "first", Source: "first:HTTP.Host"},
			{Field: "Port", Driver: "second", Source: "second:HTTP.Port"},
		}, c.Sources(sub))
		assert.Equal(t, expected, c.Sources(conf))
	})

	t.Run("sources are updated by the next parse", func(t *testing.T) {
		delete(second.values, "HTTP.Port")
		first.errs["Name"] = errors.New("random error")
		assert.Error(t, c.Parse(conf))
		assert.Equal(t, []FieldSource{
			{Field: "HTTP.Host", Driver: "first", Source: "first:HTTP.Host"},
			{Field: "HTTP.Port", Driver: "first", Source: "first:HTTP.Port"},
		}, c.Sources(conf))
	})

	t.Run("not parsed config", func(t *testing.T) {
		assert.Nil(t, c.Sources(&struct{}{}))
		assert.Nil(t, c.Sources(nil))
	})
}

func TestManager_RegistrationOrder(t *testing.T) {
	type Shared struct {
		Value string
	}
	type First struct {
		Shared Shared
	}
	type Second struct {
		Other  string
		Shared Shared
	}
	type Third struct {
		Nested struct {
			Shared Shared
		}
	}
	// map iteration order is random, so check it many times
	for i := 0; i < 50; i++ {
		driver := &namedPathMockDriver{name: "mock", pathMockDriver: pathMockDriver{values: map[string]any{
			"Shared.Value":        "shared",
			"Nested.Shared.Value": "nested",
		}}}
		c := &Manager{
			drivers:    []Driver{driver},
			registered: make(map[reflect.Type]*Registered),
			log:        &testLogger{},
		}
		assert.NoError(t, c.Register(&Third{}))
		assert.NoError(t, c.Register(&First{}))
		assert.NoError(t, c.Register(&Second{}))
		// repeated registration does not change the order
		assert.NoError(t, c.Register(&Third{}))

		// sub config is parsed from the first registered config with this type
		sub := &Shared{}
		assert.NoError(t, c.Parse(sub))
		assert.Equal(t, "nested", sub.Value)
		assert.Equal(t, []string{"Nested.Shared.Value"}, driver.requested)

		// drivers receive configs in the registration order
		assert.Equal(t, "*tinyconf.Third;*tinyconf.First;*tinyconf.Second;", c.GenDoc("mock"))
	}
}

func TestManager_Concurrency(t *testing.T) {
	type HTTP struct {
		Host string
	}
	type Config struct {
		HTTP HTTP
	}
	c, err := New(WithDriver(&staticMockDriver{}))
	assert.NoError(t, err)
	assert.NoError(t, c.Register(&Config{}))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				conf, sub := &Config{}, &HTTP{}
				assert.NoError(t, c.Register(&struct{ Other string }{}))
				assert.NoError(t, c.Register(conf))
				assert.NoError(t, c.Parse(conf))
				assert.NoError(t, c.Parse(sub))
				assert.Equal(t, "static", conf.HTTP.Host)
				assert.Equal(t, "static", sub.Host)
				assert.Len(t, c.Sources(conf), 1)
				assert.Equal(t, "doc", c.GenDoc("static"))
			}
		}()
	}
	wg.Wait()
}

// staticMockDriver has no state and can be used from many goroutines.
type staticMockDriver struct{}

func (md *staticMockDriver) GenDoc(...*Registered) string { return "doc" }
func (md *staticMockDriver) GetName() string              { return "static" }
func (md *staticMockDriver) GetValue(field fmap.Field) (*Value, error) {
	if field.GetType().Kind() != reflect.String {
		return nil, ErrValueNotFound
	}
	return &Value{Source: "static", Value: "static"}, nil
}

type watchMockDriver struct {
	mu       sync.Mutex
	value    string
	changes  chan struct{}
	watchErr error
	started  chan struct{}
}

func newWatchMockDriver(value string) *watchMockDriver {
	return &watchMockDriver{value: value, changes: make(chan struct{}), started: make(chan struct{}, 1)}
}

func (md *watchMockDriver) GenDoc(...*Registered) string { return "" }
func (md *watchMockDriver) GetName() string              { return "watch" }
func (md *watchMockDriver) GetValue(field fmap.Field) (*Value, error) {
	md.mu.Lock()
	defer md.mu.Unlock()
	if field.GetStructPath() != "Name" && field.GetStructPath() != "Sub.Name" {
		return nil, ErrValueNotFound
	}
	if md.value == "error" {
		return nil, errors.New("random error")
	}
	return &Value{Source: "watch", Value: md.value}, nil
}

func (md *watchMockDriver) set(value string) {
	md.mu.Lock()
	md.value = value
	md.mu.Unlock()
	md.changes <- struct{}{}
}

func (md *watchMockDriver) Watch(ctx context.Context, onChange func()) error {
	md.started <- struct{}{}
	if md.watchErr != nil {
		return md.watchErr
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-md.changes:
			onChange()
		}
	}
}
