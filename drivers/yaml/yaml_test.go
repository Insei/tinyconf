package yaml

import (
	"github.com/insei/fmap/v2"
	"io"
	"math"
	"reflect"
	"testing"

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

func TestYamlDriver_GetMapValue(t *testing.T) {
	tests := []struct {
		name          string
		yamlMap       map[string]any
		field         fmap.Field
		expectedValue any
		wantErr       bool
	}{
		{
			name:    "NonExistingYamlTag",
			yamlMap: nil,
			field: fmap.Get[struct {
				Test int
			}]()["Test"],
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "NonExistingValueInNonExistingYaml",
			yamlMap: nil,
			field: fmap.Get[struct {
				Test int `yaml:"nonexisting"`
			}]()["Test"],
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "NonExistingFieldInExistingYaml",
			yamlMap: map[string]any{"existent": "value"},
			field: fmap.Get[struct {
				Test int `yaml:"nonexistent"`
			}]()["Test"],
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "ExistingNonEmptyField",
			yamlMap: map[string]any{"existent": "value"},
			field: fmap.Get[struct {
				Test int `yaml:"existent"`
			}]()["Test"],
			expectedValue: "value",
			wantErr:       false,
		},
		{
			name:    "NestedExistingNonEmptyField",
			yamlMap: map[string]any{"existent": map[string]any{"nested": "value"}},
			field: fmap.Get[struct {
				Test struct {
					Nested string `yaml:"nested"`
				} `yaml:"existent"`
			}]()["Test.Nested"],
			expectedValue: "value",
			wantErr:       false,
		},
		{
			name:    "NestedNonExistingField",
			yamlMap: map[string]any{"existent": map[string]any{"nested": "value"}},
			field: fmap.Get[struct {
				Test struct {
					Nonested string `yaml:"nonesting"`
				} `yaml:"existent"`
			}]()["Test.Nonested"],
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "NestedNonExistingFieldWithEmptyMap",
			yamlMap: map[string]any{"existent": "value"},
			field: fmap.Get[struct {
				Test struct {
					Nonested string `yaml:"nonesting"`
				} `yaml:"existent"`
			}]()["Test.Nonested"],
			expectedValue: nil,
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getMapValue(tt.field, tt.yamlMap)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedValue, got)
		})
	}
}

func Test_convertValToType(t *testing.T) {
	tests := []struct {
		name    string
		field   fmap.Field
		val     any
		want    any
		wantErr bool
	}{
		{
			name: "Convert string to int",
			field: fmap.Get[struct {
				Test int
			}]()["Test"],
			val:     "123",
			want:    123,
			wantErr: false,
		},
		{
			name: "Incompatible types",
			field: fmap.Get[struct {
				Test int
			}]()["Test"],
			val:     "test",
			want:    nil,
			wantErr: true,
		},
		{
			name: "Already same types",
			field: fmap.Get[struct {
				Test string
			}]()["Test"],
			val:     "test",
			want:    "test",
			wantErr: false,
		},
		{
			name: "convertible type",
			field: fmap.Get[struct {
				Test int
			}]()["Test"],
			val:     float64(123),
			want:    123,
			wantErr: false,
		},
		{
			name: "non convertible type",
			field: fmap.Get[struct {
				Test int32
			}]()["Test"],
			val:     int64(math.MaxInt64),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertValToType(tt.field, tt.val)
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

func Test_envDriver_GetName(t *testing.T) {
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
