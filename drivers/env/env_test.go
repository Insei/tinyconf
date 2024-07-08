package env

import (
	"github.com/insei/fmap/v2"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func Test_envDriver_GetName(t *testing.T) {
	d, _ := New()
	if d.GetName() != "env" {
		t.Fail()
	}
}

func Test_envDriver_GetValue(t *testing.T) {
	tests := map[string]struct {
		setup         func()
		field         fmap.Field
		wantErr       bool
		expectedValue any
	}{
		"TagNotSet": {
			setup: func() {},
			field: fmap.Get[struct {
				Test string
			}]()["Test"],
			wantErr: true,
		},
		"EnvNotSet": {
			setup: func() {
				os.Clearenv()
			},
			field: fmap.Get[struct {
				Test string `env:"TEST"`
			}]()["Test"],
			wantErr: true,
		},
		"EnvSetCorrectly": {
			setup: func() {
				os.Setenv("TEST", "value")
			},
			field: fmap.Get[struct {
				Test string `env:"TEST"`
			}]()["Test"],
			wantErr:       false,
			expectedValue: "value",
		},
		"InvalidEnvValue": {
			setup: func() {
				os.Setenv("TEST", "value")
			},
			field: fmap.Get[struct {
				Test int `env:"TEST"`
			}]()["Test"],
			wantErr: true,
		},
	}

	d, _ := New()

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			tc.setup()
			val, err := d.GetValue(tc.field)
			if (err != nil) != tc.wantErr {
				t.Errorf("GetValue() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if err == nil && tc.expectedValue != nil {
				assert.Equal(t, tc.expectedValue, val.Value)
			}
		})
	}
}

func TestNew(t *testing.T) {
	driver, err := New()
	assert.NoError(t, err)
	assert.NotNil(t, driver)
}
