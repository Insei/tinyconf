package env

import (
	"os"
	"path"
	"testing"
	"time"

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
		in  []*tinyconf.Registered
		out string
	}{
		"test map": {
			in: []*tinyconf.Registered{
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
		registers []*tinyconf.Registered
	}{
		"with 3 registers": {
			registers: []*tinyconf.Registered{
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

func Test_setENVsFromPath(t *testing.T) {
	content := "\n" +
		"# comment\n" +
		"  # indented comment\n" +
		"TINYCONF_TEST_PLAIN=value\n" +
		"\n" +
		"TINYCONF_TEST_DSN=postgres://host/db?sslmode=disable\n" +
		"TINYCONF_TEST_DOUBLE_QUOTED=\"quoted value\"\n" +
		"TINYCONF_TEST_SINGLE_QUOTED='quoted=value'\n" +
		"TINYCONF_TEST_SPACED = spaced \n" +
		"TINYCONF_TEST_EMPTY=\n" +
		"TINYCONF_TEST_EXISTING=from file\n"
	expected := map[string]string{
		"TINYCONF_TEST_PLAIN":         "value",
		"TINYCONF_TEST_DSN":           "postgres://host/db?sslmode=disable",
		"TINYCONF_TEST_DOUBLE_QUOTED": "quoted value",
		"TINYCONF_TEST_SINGLE_QUOTED": "quoted=value",
		"TINYCONF_TEST_SPACED":        "spaced",
		"TINYCONF_TEST_EMPTY":         "",
		"TINYCONF_TEST_EXISTING":      "from env",
	}
	t.Setenv("TINYCONF_TEST_EXISTING", "from env")
	t.Cleanup(func() {
		for key := range expected {
			os.Unsetenv(key)
		}
	})

	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(path.Join(dir, ".env"), []byte(content), 0o600))
	assert.NotPanics(t, func() {
		setENVsFromPath(dir)
	})

	for key, value := range expected {
		envVal, ok := os.LookupEnv(key)
		assert.True(t, ok, key)
		assert.Equal(t, value, envVal, key)
	}
}

func TestEnvDriver_GenDoc_Hidden(t *testing.T) {
	conf := &struct {
		Password string `env:"APP_PASSWORD" doc:"password" hidden:"true"`
	}{Password: "secret"}
	storage, err := fmap.GetFrom(conf)
	assert.NoError(t, err)

	driver := envDriver{name: "env"}
	doc := driver.GenDoc(&tinyconf.Registered{Storage: storage, Config: conf})
	assert.Equal(t, "#password\n#APP_PASSWORD=\n\n", doc)
}

func Test_autoKey(t *testing.T) {
	tests := map[string]string{
		"Port":             "PORT",
		"HTTP.Port":        "HTTP_PORT",
		"HTTP.MaxConns":    "HTTP_MAX_CONNS",
		"HTTPPort":         "HTTP_PORT",
		"Auth.JWTSecret":   "AUTH_JWT_SECRET",
		"DB.Pool2.MaxSize": "DB_POOL2_MAX_SIZE",
		"lower.camelCase":  "LOWER_CAMEL_CASE",
	}
	for structPath, want := range tests {
		assert.Equal(t, want, autoKey(structPath), structPath)
	}
}

func TestEnvDriver_Options(t *testing.T) {
	type config struct {
		HTTP struct {
			Host     string `env:"HTTP_HOST"`
			MaxConns int
			Excluded string `env:"-"`
		}
		Hosts []string `env:"HOSTS"`
		Token string   `env:"TOKEN" hidden:"true" doc:"token"`
	}
	newManager := func(t *testing.T, opts ...Option) *tinyconf.Manager {
		driver, err := New(append([]Option{WithFiles()}, opts...)...)
		assert.NoError(t, err)
		manager, err := tinyconf.New(tinyconf.WithDriver(driver))
		assert.NoError(t, err)
		assert.NoError(t, manager.Register(&config{}))
		return manager
	}

	t.Run("prefix", func(t *testing.T) {
		t.Setenv("HTTP_HOST", "without prefix")
		t.Setenv("TINYCONF_OPT_HTTP_HOST", "localhost")
		t.Setenv("TINYCONF_OPT_HOSTS", "a,b")
		manager := newManager(t, WithPrefix("TINYCONF_OPT_"))
		conf := &config{}
		assert.NoError(t, manager.Parse(conf))
		assert.Equal(t, "localhost", conf.HTTP.Host)
		assert.Equal(t, []string{"a", "b"}, conf.Hosts)
		assert.Equal(t, 0, conf.HTTP.MaxConns)

		conf.Token = "secret"
		assert.NoError(t, manager.Register(conf))
		assert.Equal(t, "#\n#TINYCONF_OPT_HTTP_HOST=localhost\n\n#\n#TINYCONF_OPT_HOSTS=a,b\n\n#token\n#TINYCONF_OPT_TOKEN=\n\n", manager.GenDoc("env"))
	})

	t.Run("auto keys", func(t *testing.T) {
		t.Setenv("TINYCONF_OPT_HTTP_MAX_CONNS", "10")
		t.Setenv("TINYCONF_OPT_HTTP_EXCLUDED", "excluded")
		t.Setenv("TINYCONF_OPT_HTTP_HOST", "localhost")
		manager := newManager(t, WithPrefix("TINYCONF_OPT_"), WithAutoKeys())
		conf := &config{}
		assert.NoError(t, manager.Parse(conf))
		assert.Equal(t, 10, conf.HTTP.MaxConns)
		assert.Equal(t, "localhost", conf.HTTP.Host)
		assert.Equal(t, "", conf.HTTP.Excluded)

		assert.NoError(t, manager.Register(conf))
		doc := manager.GenDoc("env")
		assert.Contains(t, doc, "#TINYCONF_OPT_HTTP_MAX_CONNS=10\n")
		assert.NotContains(t, doc, "EXCLUDED")
	})

	t.Run("without auto keys", func(t *testing.T) {
		t.Setenv("HTTP_MAX_CONNS", "10")
		manager := newManager(t)
		conf := &config{}
		assert.NoError(t, manager.Parse(conf))
		assert.Equal(t, 0, conf.HTTP.MaxConns)
	})

	t.Run("secret files", func(t *testing.T) {
		secretPath := path.Join(t.TempDir(), "token")
		assert.NoError(t, os.WriteFile(secretPath, []byte("from file\n"), 0o600))
		t.Setenv("TOKEN_FILE", secretPath)

		conf := &config{}
		assert.NoError(t, newManager(t).Parse(conf))
		assert.Equal(t, "", conf.Token)

		manager := newManager(t, WithSecretFiles())
		assert.NoError(t, manager.Parse(conf))
		assert.Equal(t, "from file", conf.Token)

		// variable has priority over file
		t.Setenv("TOKEN", "from env")
		assert.NoError(t, manager.Parse(conf))
		assert.Equal(t, "from env", conf.Token)
	})

	t.Run("absent secret file", func(t *testing.T) {
		t.Setenv("TOKEN_FILE", path.Join(t.TempDir(), "absent"))
		conf := &config{}
		assert.Error(t, newManager(t, WithSecretFiles()).Parse(conf))
	})
}

func TestEnvDriver_Files(t *testing.T) {
	type config struct {
		First  string `env:"TINYCONF_FILES_FIRST"`
		Second string `env:"TINYCONF_FILES_SECOND"`
		Env    string `env:"TINYCONF_FILES_ENV"`
		Auto   string
	}
	keys := []string{"TINYCONF_FILES_FIRST", "TINYCONF_FILES_SECOND", "AUTO"}
	unset := func() {
		for _, key := range keys {
			os.Unsetenv(key)
		}
	}
	unset()
	t.Cleanup(unset)
	t.Setenv("TINYCONF_FILES_ENV", "from env")

	dir := t.TempDir()
	first, second := path.Join(dir, "first.env"), path.Join(dir, "second.env")
	assert.NoError(t, os.WriteFile(first, []byte("TINYCONF_FILES_FIRST=first\nTINYCONF_FILES_ENV=first\n"), 0o600))
	assert.NoError(t, os.WriteFile(second, []byte("TINYCONF_FILES_FIRST=second\nTINYCONF_FILES_SECOND=second\nAUTO=auto\n"), 0o600))

	parse := func(t *testing.T, opts ...Option) config {
		driver, err := New(opts...)
		assert.NoError(t, err)
		manager, err := tinyconf.New(tinyconf.WithDriver(driver))
		assert.NoError(t, err)
		conf := &config{}
		assert.NoError(t, manager.Register(conf))
		assert.NoError(t, manager.Parse(conf))
		return *conf
	}

	t.Run("isolation", func(t *testing.T) {
		conf := parse(t, WithFiles(first, second), WithFilesIsolation(), WithAutoKeys())
		assert.Equal(t, config{First: "first", Second: "second", Env: "from env", Auto: "auto"}, conf)
		for _, key := range keys {
			_, exist := os.LookupEnv(key)
			assert.False(t, exist, key)
		}
	})

	t.Run("without files", func(t *testing.T) {
		assert.Equal(t, config{Env: "from env"}, parse(t, WithFiles()))
	})

	t.Run("export to the process environment", func(t *testing.T) {
		conf := parse(t, WithFiles(first, second))
		assert.Equal(t, config{First: "first", Second: "second", Env: "from env"}, conf)
		assert.Equal(t, "first", os.Getenv("TINYCONF_FILES_FIRST"))
		assert.Equal(t, "second", os.Getenv("TINYCONF_FILES_SECOND"))
	})

	t.Run("absent file", func(t *testing.T) {
		_, err := New(WithFiles(path.Join(dir, "absent.env")))
		assert.Error(t, err)
	})
}

func TestEnvDriver_GenDoc_Types(t *testing.T) {
	conf := &struct {
		Hosts   []string       `env:"HOSTS" doc:"hosts"`
		Paths   []string       `env:"PATHS" separator:":"`
		Labels  map[string]int `env:"LABELS"`
		Matrix  [][]int        `env:"MATRIX"`
		Timeout time.Duration  `env:"TIMEOUT"`
		At      time.Time      `env:"AT"`
		Ptr     *int           `env:"PTR"`
	}{
		Hosts:   []string{"a", "b,c"},
		Paths:   []string{"/bin", "/usr/bin"},
		Labels:  map[string]int{"b": 2, "a": 1},
		Matrix:  [][]int{{1}, {2, 3}},
		Timeout: 90 * time.Second,
		At:      time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	storage, err := fmap.GetFrom(conf)
	assert.NoError(t, err)

	driver := envDriver{name: "env"}
	doc := driver.GenDoc(&tinyconf.Registered{Storage: storage, Config: conf})
	assert.Equal(t, "#hosts\n#HOSTS=a,\"b,c\"\n\n"+
		"#\n#PATHS=/bin:/usr/bin\n\n"+
		"#\n#LABELS=a:1,b:2\n\n"+
		"#\n#MATRIX=[[1],[2,3]]\n\n"+
		"#\n#TIMEOUT=1m30s\n\n"+
		"#\n#AT=2024-01-02T03:04:05Z\n\n"+
		"#\n#PTR=\n\n", doc)
}
