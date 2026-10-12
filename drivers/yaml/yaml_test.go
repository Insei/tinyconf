package yaml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/insei/fmap/v3"
	"github.com/insei/tinyconf"
	"github.com/stretchr/testify/assert"
)

func TestYamlDriver_GetName(t *testing.T) {
	d, _ := New("anystring.yaml")
	if d.GetName() != "yaml" {
		t.Fail()
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
		in  []*tinyconf.Registered
		out string
	}{
		"test map": {
			in: []*tinyconf.Registered{
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
	driver, err := New("anystring.yaml")
	assert.NoError(t, err)
	assert.NotNil(t, driver)
}

func TestYamlDriver_GenDoc_Hidden(t *testing.T) {
	conf := &struct {
		Password string `yaml:"password" doc:"password" hidden:"true"`
	}{Password: "secret"}
	storage, err := fmap.GetFrom(conf)
	assert.NoError(t, err)

	driver := &yamlDriver{name: "yaml"}
	doc := driver.GenDoc(&tinyconf.Registered{Storage: storage, Config: conf})
	assert.Equal(t, "#password\n#password: \n", doc)
}

func writeFile(t *testing.T, name, content string) string {
	filePath := filepath.Join(t.TempDir(), name)
	assert.NoError(t, os.WriteFile(filePath, []byte(content), 0o600))
	return filePath
}

func TestYamlDriver_Sources(t *testing.T) {
	type config struct {
		HTTP struct {
			Host string `yaml:"host"`
			Port int    `yaml:"port"`
		} `yaml:"http"`
		Hosts []string `yaml:"hosts"`
		Name  string   `yaml:"name"`
	}
	base := "http:\n  host: localhost\n  port: 80\nhosts: [a, b]\nname: base\n"
	override := "http:\n  port: 8080\nhosts: [c]\n"
	tests := []struct {
		name    string
		driver  func(t *testing.T) (tinyconf.Driver, error)
		want    func(c *config)
		wantErr bool
	}{
		{
			name: "single file",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "base.yaml", base))
			},
			want: func(c *config) {
				c.HTTP.Host, c.HTTP.Port, c.Hosts, c.Name = "localhost", 80, []string{"a", "b"}, "base"
			},
		},
		{
			name: "next file overrides previous, nested maps are merged",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "base.yaml", base), WithFiles(writeFile(t, "override.yaml", override)))
			},
			want: func(c *config) {
				c.HTTP.Host, c.HTTP.Port, c.Hosts, c.Name = "localhost", 8080, []string{"c"}, "base"
			},
		},
		{
			name: "absent file",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(filepath.Join(t.TempDir(), "absent.yaml"))
			},
			want:    func(c *config) {},
			wantErr: true,
		},
		{
			name: "absent optional file",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "base.yaml", base), WithFiles(filepath.Join(t.TempDir(), "absent.yaml")), WithOptional())
			},
			want: func(c *config) {
				c.HTTP.Host, c.HTTP.Port, c.Hosts, c.Name = "localhost", 80, []string{"a", "b"}, "base"
			},
		},
		{
			name: "all optional files are absent",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(filepath.Join(t.TempDir(), "absent.yaml"), WithOptional())
			},
			want: func(c *config) {},
		},
		{
			name: "incorrect optional file",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "bad.yaml", "key: value:"), WithOptional())
			},
			want:    func(c *config) {},
			wantErr: true,
		},
		{
			name: "empty file",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New(writeFile(t, "empty.yaml", ""))
			},
			want: func(c *config) {},
		},
		{
			name: "fs and data",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				fsys := fstest.MapFS{"conf/base.yaml": {Data: []byte(base)}}
				return New("", WithFS(fsys, "conf/base.yaml"), WithData("override", []byte(override)))
			},
			want: func(c *config) {
				c.HTTP.Host, c.HTTP.Port, c.Hosts, c.Name = "localhost", 8080, []string{"c"}, "base"
			},
		},
		{
			name: "env expansion",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				t.Setenv("TINYCONF_YAML_HOST", "example.com")
				content := "http:\n  host: ${TINYCONF_YAML_HOST}\n  port: ${TINYCONF_YAML_PORT:-443}\nname: $${NAME}\n"
				return New("", WithData("config", []byte(content)), WithEnvExpansion())
			},
			want: func(c *config) {
				c.HTTP.Host, c.HTTP.Port, c.Name = "example.com", 443, "${NAME}"
			},
		},
		{
			name: "without env expansion",
			driver: func(t *testing.T) (tinyconf.Driver, error) {
				return New("", WithData("config", []byte("name: ${TINYCONF_YAML_HOST}\n")))
			},
			want: func(c *config) {
				c.Name = "${TINYCONF_YAML_HOST}"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver, err := tt.driver(t)
			assert.NoError(t, err)
			manager, err := tinyconf.New(tinyconf.WithDriver(driver))
			assert.NoError(t, err)

			conf, want := &config{}, &config{}
			tt.want(want)
			assert.NoError(t, manager.Register(conf))
			err = manager.Parse(conf)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, want, conf)
		})
	}
}

func TestYamlDriver_Reload(t *testing.T) {
	type config struct {
		Name string `yaml:"name"`
	}
	filePath := writeFile(t, "config.yaml", "name: first\n")
	driver, err := New(filePath)
	assert.NoError(t, err)
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)

	conf := &config{}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))
	assert.Equal(t, "first", conf.Name)

	// file is read again on every Parse
	assert.NoError(t, os.WriteFile(filePath, []byte("name: second\n"), 0o600))
	assert.NoError(t, manager.Parse(conf))
	assert.Equal(t, "second", conf.Name)
}

func TestYamlDriver_GenDoc_Types(t *testing.T) {
	type server struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	}
	conf := &struct {
		Hosts   []string       `yaml:"hosts" doc:"hosts"`
		Labels  map[string]int `yaml:"labels"`
		Servers []server       `yaml:"servers"`
		Timeout time.Duration  `yaml:"timeout"`
		At      time.Time      `yaml:"at"`
		Ptr     *int           `yaml:"ptr"`
		Any     any            `yaml:"any"`
	}{
		Hosts:   []string{"a", "b"},
		Labels:  map[string]int{"b": 2, "a": 1},
		Servers: []server{{Host: "a", Port: 1}},
		Timeout: 90 * time.Second,
		At:      time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	storage, err := fmap.GetFrom(conf)
	assert.NoError(t, err)

	driver := &yamlDriver{name: "yaml"}
	doc := driver.GenDoc(&tinyconf.Registered{Storage: storage, Config: conf})
	assert.Equal(t, "#hosts\n#hosts: [\"a\",\"b\"]\n"+
		"#\n#labels: {\"a\":1,\"b\":2}\n"+
		"#\n#servers: [{\"host\":\"a\",\"port\":1}]\n"+
		"#\n#timeout: 1m30s\n"+
		"#\n#at: 2024-01-02T03:04:05Z\n"+
		"#\n#ptr: \n"+
		"#\n#any: \n", doc)

	t.Run("doc can be parsed back", func(t *testing.T) {
		// uncomment lines with values
		var content string
		for _, line := range strings.Split(doc, "\n") {
			if strings.Contains(line, ": ") {
				content += strings.TrimPrefix(line, "#") + "\n"
			}
		}
		driver, err := New("", WithData("doc", []byte(content)))
		assert.NoError(t, err)
		manager, err := tinyconf.New(tinyconf.WithDriver(driver))
		assert.NoError(t, err)
		parsed := &struct {
			Hosts   []string       `yaml:"hosts" doc:"hosts"`
			Labels  map[string]int `yaml:"labels"`
			Servers []server       `yaml:"servers"`
			Timeout time.Duration  `yaml:"timeout"`
			At      time.Time      `yaml:"at"`
			Ptr     *int           `yaml:"ptr"`
			Any     any            `yaml:"any"`
		}{}
		assert.NoError(t, manager.Register(parsed))
		assert.NoError(t, manager.Parse(parsed))
		assert.Equal(t, conf, parsed)
	})
}
