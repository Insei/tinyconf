package drivers_test

import (
	"testing"
	"time"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/env"
	"github.com/insei/tinyconf/drivers/tag"
	"github.com/insei/tinyconf/drivers/yaml"
)

type benchServer struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type benchConfig struct {
	Name string `initial:"app" yaml:"name" env:"BENCH_NAME"`
	HTTP struct {
		Host    string        `initial:"localhost" yaml:"host" env:"BENCH_HTTP_HOST"`
		Port    int           `initial:"8080" yaml:"port" env:"BENCH_HTTP_PORT"`
		Timeout time.Duration `initial:"30s" yaml:"timeout" env:"BENCH_HTTP_TIMEOUT"`
	} `yaml:"http"`
	Hosts   []string          `initial:"a,b" yaml:"hosts" env:"BENCH_HOSTS"`
	Labels  map[string]string `yaml:"labels" env:"BENCH_LABELS"`
	Servers []benchServer     `yaml:"servers"`
	Secret  tinyconf.Secret   `yaml:"secret" env:"BENCH_SECRET"`
}

const benchYAML = `name: bench
http:
  port: 9090
  timeout: 1m
hosts: [c, d]
labels: {env: prod, team: core}
servers:
  - {host: a, port: 1}
  - {host: b, port: 2}
secret: password
`

// BenchmarkParse_Drivers parses the config by the usual chain of the drivers: tags, a yaml document and environment.
func BenchmarkParse_Drivers(b *testing.B) {
	b.Setenv("BENCH_HTTP_HOST", "example.com")
	b.Setenv("BENCH_LABELS", "env:stage")
	config, err := tinyconf.New(
		tinyconf.WithDriver(must(tag.New("initial"))),
		tinyconf.WithDriver(must(yaml.New("", yaml.WithData("bench.yaml", []byte(benchYAML))))),
		tinyconf.WithDriver(must(env.New(env.WithFiles()))),
	)
	if err != nil {
		b.Fatal(err)
	}
	conf := &benchConfig{}
	if err = config.Register(conf); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err = config.Parse(conf); err != nil {
			b.Fatal(err)
		}
	}
}

func must(driver tinyconf.Driver, err error) tinyconf.Driver {
	if err != nil {
		panic(err)
	}
	return driver
}
