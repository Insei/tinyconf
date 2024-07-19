package yaml

import (
	"io"
	"math"
	"reflect"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/insei/tinyconf"
	"github.com/stretchr/testify/assert"
)

type mockReaderCloser struct {
	n    int
	data []byte
}

func (m *mockReaderCloser) Read(p []byte) (int, error) {
	n := 0
	if m.n == 0 {
		n = copy(p, m.data)
	} else {
		n = copy(p, m.data[m.n:])
	}
	if m.n != m.n+n {
		m.n += n
		return n, nil
	}
	return 0, io.EOF
}

func (m *mockReaderCloser) Close() error {
	return nil
}

func TestYamlDriver_GetName(t *testing.T) {
	d, _ := New("anystring.yaml")
	if d.GetName() != "yaml" {
		t.Fail()
	}
}

func Test_convertValToType(t *testing.T) {
	tests := []struct {
		name     string
		getField func() fmap.Field
		val      any
		want     any
		wantErr  bool
	}{
		{
			name: "Convert string to int",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int
				}]()
				return storage.MustFind("Test")
			},
			val:     "123",
			want:    123,
			wantErr: false,
		},
		{
			name: "Incompatible types",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int
				}]()
				return storage.MustFind("Test")
			},
			val:     "test",
			want:    nil,
			wantErr: true,
		},
		{
			name: "Already same types",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test string
				}]()
				return storage.MustFind("Test")
			},
			val:     "test",
			want:    "test",
			wantErr: false,
		},
		{
			name: "convertible type",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int
				}]()
				return storage.MustFind("Test")
			},
			val:     float64(123),
			want:    123,
			wantErr: false,
		},
		{
			name: "non convertible type",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int32
				}]()
				return storage.MustFind("Test")
			},
			val:     int64(math.MaxInt64),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertValToType(tt.getField(), tt.val)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertValToType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertValToType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestYamlDriver_GetMapValue(t *testing.T) {
	tests := []struct {
		name          string
		yamlMap       map[string]any
		getField      func() fmap.Field
		expectedValue any
		wantErr       bool
	}{
		{
			name:    "NonExistingYamlTag",
			yamlMap: nil,
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int
				}]()
				return storage.MustFind("Test")
			},
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "NonExistingValueInNonExistingYaml",
			yamlMap: nil,
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int `yaml:"nonexisting"`
				}]()
				return storage.MustFind("Test")
			},
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "NonExistingFieldInExistingYaml",
			yamlMap: map[string]any{"existent": "value"},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int `yaml:"nonexistent"`
				}]()
				return storage.MustFind("Test")
			},
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "ExistingNonEmptyField",
			yamlMap: map[string]any{"existent": "value"},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int `yaml:"existent"`
				}]()
				return storage.MustFind("Test")
			},
			expectedValue: "value",
			wantErr:       false,
		},
		{
			name:    "NestedExistingNonEmptyField",
			yamlMap: map[string]any{"existent": map[string]any{"nested": "value"}},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test struct {
						Nested string `yaml:"nested"`
					} `yaml:"existent"`
				}]()
				return storage.MustFind("Test.Nested")
			},
			expectedValue: "value",
			wantErr:       false,
		},
		{
			name:    "NestedNonExistingField",
			yamlMap: map[string]any{"existent": map[string]any{"nested": "value"}},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test struct {
						Nonested string `yaml:"nonested"`
					} `yaml:"existent"`
				}]()
				return storage.MustFind("Test.Nonested")
			},
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "NestedNonExistingFieldWithEmptyMap",
			yamlMap: map[string]any{"existent": "value"},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test struct {
						Nonested string `yaml:"nonesting"`
					} `yaml:"existent"`
				}]()
				return storage.MustFind("Test.Nonested")
			},
			expectedValue: nil,
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getMapValue(tt.getField(), tt.yamlMap)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedValue, got)
		})
	}
}

func TestYamlDriver_GenDoc(t *testing.T) {
	type TestingFirstStruct struct {
		HTTP struct {
			A    uint `yaml:"a"`
			B    int  `yaml:"b"`
			C    uint
			Host string `yaml:"host" doc:"http protocol host"`
			Port string `yaml:"port" doc:"http protocol port"`
		} `yaml:"http" doc:"network protocol http"`
	}
	type X struct {
		Y int `yaml:"y" doc:"description of y"`
	}
	type TestingSecondStruct struct {
		HTTP struct {
			Auth struct {
				Alg    string `yaml:"alg" doc:"http authentication algorithm"`
				Issuer string `yaml:"issuer" doc:"http authentication issuer"`
			} `yaml:"auth" doc:"authentication block"`
		} `yaml:"http" doc:"network protocol http"`
	}

	storages := make([]fmap.Storage, 3)
	storages[0], _ = fmap.Get[TestingFirstStruct]()
	storages[1], _ = fmap.Get[X]()
	storages[2], _ = fmap.Get[TestingSecondStruct]()

	configs := make([]any, 3)
	configs[0] = TestingFirstStruct{
		HTTP: struct {
			A    uint `yaml:"a"`
			B    int  `yaml:"b"`
			C    uint
			Host string `yaml:"host" doc:"http protocol host"`
			Port string `yaml:"port" doc:"http protocol port"`
		}{
			A:    35,
			B:    -35,
			C:    42,
			Host: "192.168.236.42",
			Port: "8888",
		},
	}
	configs[1] = X{Y: 100}
	configs[2] = TestingSecondStruct{
		HTTP: struct {
			Auth struct {
				Alg    string `yaml:"alg" doc:"http authentication algorithm"`
				Issuer string `yaml:"issuer" doc:"http authentication issuer"`
			} `yaml:"auth" doc:"authentication block"`
		}{
			Auth: struct {
				Alg    string `yaml:"alg" doc:"http authentication algorithm"`
				Issuer string `yaml:"issuer" doc:"http authentication issuer"`
			}{
				Alg:    "SHA256",
				Issuer: "Application",
			},
		},
	}

	driver := yamlDriver{name: "yaml"}

	tests := map[string]struct {
		in  []tinyconf.Registered
		out string
	}{
		"test map": {
			in: []tinyconf.Registered{
				{Storage: storages[0], Config: configs[0]},
				{Storage: storages[1], Config: configs[1]},
				{Storage: storages[2], Config: configs[2]},
			},
			out: `#network protocol http
#http: 
	#
	#a: 35
	#authentication block
	#auth: 
		#http authentication algorithm
		#alg: SHA256
		#http authentication issuer
		#issuer: Application
	#
	#b: -35
	#http protocol host
	#host: 192.168.236.42
	#http protocol port
	#port: 8888
#description of y
#y: 100
`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			out := driver.GenDoc(tc.in...)
			assert.Equal(t, tc.out, out)
		})
	}
}

// go test ./drivers/yaml -bench . -benchmem
func BenchmarkYamlDriver_GenDoc(b *testing.B) {
	type TestingFirstStruct struct {
		HTTP struct {
			A    uint `yaml:"a"`
			B    int  `yaml:"b"`
			C    uint
			Host string `yaml:"host" doc:"http protocol host"`
			Port string `yaml:"port" doc:"http protocol port"`
		} `yaml:"http" doc:"network protocol http"`
	}
	type X struct {
		Y int `yaml:"y" doc:"description of y"`
	}
	type TestingSecondStruct struct {
		HTTP struct {
			Auth struct {
				Alg    string `yaml:"alg" doc:"http authentication algorithm"`
				Issuer string `yaml:"issuer" doc:"http authentication issuer"`
			} `yaml:"auth" doc:"authentication block"`
		} `yaml:"http" doc:"network protocol http"`
	}

	storages := make([]fmap.Storage, 3)
	storages[0], _ = fmap.Get[TestingFirstStruct]()
	storages[1], _ = fmap.Get[X]()
	storages[2], _ = fmap.Get[TestingSecondStruct]()

	configs := make([]any, 3)
	configs[0] = TestingFirstStruct{
		HTTP: struct {
			A    uint `yaml:"a"`
			B    int  `yaml:"b"`
			C    uint
			Host string `yaml:"host" doc:"http protocol host"`
			Port string `yaml:"port" doc:"http protocol port"`
		}{
			A:    35,
			B:    -35,
			C:    42,
			Host: "192.168.236.42",
			Port: "8888",
		},
	}
	configs[1] = X{Y: 100}
	configs[2] = TestingSecondStruct{
		HTTP: struct {
			Auth struct {
				Alg    string `yaml:"alg" doc:"http authentication algorithm"`
				Issuer string `yaml:"issuer" doc:"http authentication issuer"`
			} `yaml:"auth" doc:"authentication block"`
		}{
			Auth: struct {
				Alg    string `yaml:"alg" doc:"http authentication algorithm"`
				Issuer string `yaml:"issuer" doc:"http authentication issuer"`
			}{
				Alg:    "SHA256",
				Issuer: "Application",
			},
		},
	}

	driver := yamlDriver{name: "yaml"}

	benchData := map[string]struct {
		registers []tinyconf.Registered
	}{
		"with 3 registers": {
			registers: []tinyconf.Registered{
				{Storage: storages[0], Config: configs[0]},
				{Storage: storages[1], Config: configs[1]},
				{Storage: storages[2], Config: configs[2]}},
		},
	}

	for benchName, data := range benchData {
		b.ResetTimer()
		b.Run(benchName, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				driver.GenDoc(data.registers...)
			}
		})
	}
}

func TestNew(t *testing.T) {
	driver, err := New("anystring.yaml")
	assert.NoError(t, err)
	assert.NotNil(t, driver)
}
