package env

import (
	"os"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
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
		getField      func() fmap.Field
		wantErr       bool
		expectedValue any
	}{
		"TagNotSet": {
			setup: func() {},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test string
				}]()
				return storage.MustFind("Test")
			},
			wantErr: true,
		},
		"EnvNotSet": {
			setup: func() {
				os.Clearenv()
			},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test string `env:"TEST"`
				}]()
				return storage.MustFind("Test")
			},
			wantErr: true,
		},
		"EnvSetCorrectly": {
			setup: func() {
				os.Setenv("TEST", "value")
			},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test string `env:"TEST"`
				}]()
				return storage.MustFind("Test")
			},
			wantErr:       false,
			expectedValue: "value",
		},
		"InvalidEnvValue": {
			setup: func() {
				os.Setenv("TEST", "value")
			},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int `env:"TEST"`
				}]()
				return storage.MustFind("Test")
			},
			wantErr: true,
		},
	}

	d, _ := New()

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			tc.setup()
			val, err := d.GetValue(tc.getField())
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

func TestEnvDriver_GenDoc(t *testing.T) {
	type TestingFirstStruct struct {
		Service struct {
			Name string `env:"SERVICE_NAME" doc:"service name"`
		}
		HTTP struct {
			Auth struct {
				Alg    string `env:"HTTP_AUTH_ALG"`
				Issuer string `env:"HTTP_AUTH_ISSUER" doc:""`
			}
		}
	}
	type TestingSecondStruct struct {
		Service struct {
			ServiceName string `env:"SERVICE_NAME" doc:"service name 27"`
		}
		Something uint `env:"SOMETHING" doc:"something"`
	}
	type TestingThirdStruct struct {
		HTTP struct {
			Host string `env:"HTTP_HOST" doc:"http protocol host"`
			Port string `env:"HTTP_PORT" doc:"http protocol port"`
		}
	}

	driver := envDriver{name: "env"}

	storages := make([]fmap.Storage, 3)
	storages[0], _ = fmap.Get[TestingFirstStruct]()
	storages[1], _ = fmap.Get[TestingSecondStruct]()
	storages[2], _ = fmap.Get[TestingThirdStruct]()

	tests := map[string]struct {
		in  []fmap.Storage
		out string
	}{
		"test map": {
			in: storages,
			out: `#service name
#SERVICE_NAME=

#
#HTTP_AUTH_ALG=
#
#HTTP_AUTH_ISSUER=
#http protocol host
#HTTP_HOST=
#http protocol port
#HTTP_PORT=

#something
#SOMETHING=

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

func TestNew(t *testing.T) {
	driver, err := New()
	assert.NoError(t, err)
	assert.NotNil(t, driver)
}
