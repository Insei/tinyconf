package yaml

import (
	"io"
	"math"
	"reflect"
	"testing"

	"github.com/insei/fmap/v3"
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

	driver := yamlDriver{name: "yaml"}

	storages := make([]fmap.Storage, 3)
	storages[0], _ = fmap.Get[TestingFirstStruct]()
	storages[1], _ = fmap.Get[X]()
	storages[2], _ = fmap.Get[TestingSecondStruct]()

	tests := map[string]struct {
		in  []fmap.Storage
		out string
	}{
		"test map": {
			in: storages,
			out: `#network protocol http
#http:
	#
	#a:
	#authentication block
	#auth:
		#http authentication algorithm
		#alg:
		#http authentication issuer
		#issuer:
	#
	#b:
	#http protocol host
	#host:
	#http protocol port
	#port:
#description of y
#y:
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

func TestYamlDriver_GetName(t *testing.T) {
	d, _ := New("anystring.yaml")
	if d.GetName() != "yaml" {
		t.Fail()
	}
}

func TestNew(t *testing.T) {
	driver, err := New("anystring.yaml")
	assert.NoError(t, err)
	assert.NotNil(t, driver)
}