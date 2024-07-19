package tag

import (
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

func TestDefaultTagDriver_GetValue(t *testing.T) {
	tests := []struct {
		name          string
		getField      func() fmap.Field
		driver        *defaultTagDriver
		expectedValue interface{}
		wantErr       bool
	}{
		{
			name: "value successfully retrieved",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test string `default:"test"`
				}]()
				return storage.MustFind("Test")
			},
			driver: &defaultTagDriver{
				tag:  "default",
				name: "tag",
			},
			expectedValue: "test",
			wantErr:       false,
		},
		{
			name: "missing tag",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test string
				}]()
				return storage.MustFind("Test")
			},
			driver: &defaultTagDriver{
				tag:  "default",
				name: "tag",
			},
			wantErr: true,
		},
		{
			name: "empty tag",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test string `default:""`
				}]()
				return storage.MustFind("Test")
			},
			driver: &defaultTagDriver{
				tag:  "default",
				name: "tag",
			},
			wantErr: true,
		},
		{
			name: "incorrect tag value",
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int `default:"string"`
				}]()
				return storage.MustFind("Test")
			},
			driver: &defaultTagDriver{
				tag:  "default",
				name: "tag",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := tt.driver.GetValue(tt.getField())
			if (err != nil) != tt.wantErr {
				t.Errorf("GetValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && (val.Source != tt.driver.tag || val.Value != tt.expectedValue) {
				t.Errorf("GetValue() value = %v, want %v", val, tt.expectedValue)
			}
		})
	}
}

func TestDefaultTagDriver_GetName(t *testing.T) {
	tests := []struct {
		name       string
		driverName string
		want       string
	}{
		{
			name:       "Empty name",
			driverName: "",
			want:       "",
		},
		{
			name:       "Non-empty name",
			driverName: "testName",
			want:       "testName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := defaultTagDriver{
				name: tt.driverName,
			}

			if got := d.GetName(); got != tt.want {
				t.Errorf("defaultTagDriver.GetName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "tag",
		},
		{
			name:     "non empty string",
			input:    "MyTag",
			expected: "tag",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			driver, err := New(tt.input)
			assert.Nil(t, err)
			assert.Equal(t, tt.expected, driver.GetName())
		})
	}

}
