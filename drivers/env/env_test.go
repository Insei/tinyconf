package env

import (
	"os"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/insei/tinyconf"
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

	storages := make([]fmap.Storage, 3)
	storages[0], _ = fmap.Get[TestingFirstStruct]()
	storages[1], _ = fmap.Get[TestingSecondStruct]()
	storages[2], _ = fmap.Get[TestingThirdStruct]()

	configs := make([]any, 3)
	configs[0] = TestingFirstStruct{
		Service: struct {
			Name string `env:"SERVICE_NAME" doc:"service name"`
		}{
			Name: "Service",
		},
		HTTP: struct {
			Auth struct {
				Alg    string `env:"HTTP_AUTH_ALG"`
				Issuer string `env:"HTTP_AUTH_ISSUER" doc:""`
			}
		}{
			Auth: struct {
				Alg    string `env:"HTTP_AUTH_ALG"`
				Issuer string `env:"HTTP_AUTH_ISSUER" doc:""`
			}{
				Alg:    "SHA256",
				Issuer: "Application",
			},
		},
	}
	configs[1] = TestingSecondStruct{
		Service: struct {
			ServiceName string `env:"SERVICE_NAME" doc:"service name 27"`
		}{
			ServiceName: "Senders",
		},
		Something: 200,
	}
	configs[2] = TestingThirdStruct{
		HTTP: struct {
			Host string `env:"HTTP_HOST" doc:"http protocol host"`
			Port string `env:"HTTP_PORT" doc:"http protocol port"`
		}{
			Host: "localhost",
			Port: "8080",
		},
	}

	driver := envDriver{name: "env"}

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
			out: `#service name
#SERVICE_NAME=Service

#
#HTTP_AUTH_ALG=SHA256
#
#HTTP_AUTH_ISSUER=Application
#http protocol host
#HTTP_HOST=localhost
#http protocol port
#HTTP_PORT=8080

#something
#SOMETHING=200

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

// go test ./drivers/env -bench . -benchmem
func BenchmarkEnvDriver_GenDoc(b *testing.B) {
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

	storages := make([]fmap.Storage, 3)
	storages[0], _ = fmap.Get[TestingFirstStruct]()
	storages[1], _ = fmap.Get[TestingSecondStruct]()
	storages[2], _ = fmap.Get[TestingThirdStruct]()

	configs := make([]any, 3)
	configs[0] = TestingFirstStruct{
		Service: struct {
			Name string `env:"SERVICE_NAME" doc:"service name"`
		}{
			Name: "Service",
		},
		HTTP: struct {
			Auth struct {
				Alg    string `env:"HTTP_AUTH_ALG"`
				Issuer string `env:"HTTP_AUTH_ISSUER" doc:""`
			}
		}{
			Auth: struct {
				Alg    string `env:"HTTP_AUTH_ALG"`
				Issuer string `env:"HTTP_AUTH_ISSUER" doc:""`
			}{
				Alg:    "SHA256",
				Issuer: "Application",
			},
		},
	}
	configs[1] = TestingSecondStruct{
		Service: struct {
			ServiceName string `env:"SERVICE_NAME" doc:"service name 27"`
		}{
			ServiceName: "Senders",
		},
		Something: 200,
	}
	configs[2] = TestingThirdStruct{
		HTTP: struct {
			Host string `env:"HTTP_HOST" doc:"http protocol host"`
			Port string `env:"HTTP_PORT" doc:"http protocol port"`
		}{
			Host: "localhost",
			Port: "8080",
		},
	}

	driver := envDriver{name: "env"}

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
	tests := map[string]struct {
		envVarName  string
		expectedVal string
	}{
		"EnvSet": {
			"ENV_VAR",
			"true",
		},
		"EnvNotSet": {
			"ENV_NOT_SET",
			"",
		},
		"EnvCommented": {
			"COMMENTED",
			"",
		},
		"EnvEmpty": {
			"",
			"",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			driver, err := New()
			envVar := os.Getenv(test.envVarName)

			assert.NoError(t, err)
			assert.NotNil(t, driver)
			assert.Equal(t, test.expectedVal, envVar)
		})
	}
}
