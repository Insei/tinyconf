package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/env"
	"github.com/insei/tinyconf/drivers/tag"
	"github.com/insei/tinyconf/drivers/yaml"
	"github.com/insei/tinyconf/logger"
)

const (
	UsageDocFlag = "Show documentation in selected format. Supported formats: env, yaml.\nExample: go run ./example/main.go -doc yaml"
)

type Application struct {
	Name string `env:"APPLICATION_NAME" yaml:"name" doc:"application name"`
}
type HTTP struct {
	Host string `env:"HTTP_HOST" yaml:"host" doc:"http host"`
	Port int    `env:"HTTP_PORT" yaml:"port" doc:"http port"`
}
type HTTPAuth struct {
	Auth struct {
		Issuer   string `env:"HTTP_AUTH_ISSUER" yaml:"issuer" doc:"http authentication issuer"`
		Audience string `env:"HTTP_AUTH_AUDIENCE" yaml:"audience" doc:"http authentication audience"`
	} `yaml:"auth"`
}
type HTTPAuthSign struct {
	Middleware struct {
		Sign string `env:"HTTP_AUTH_SIGN" yaml:"sign" doc:"http middleware sign"`
		Key  string `env:"HTTP_AUTH_KEY" yaml:"key" doc:"http middleware key"`
	} `yaml:"auth"`
}

type sharedConfig struct {
	HTTP HTTP `yaml:"http"`
}
type sharedAuthConfig struct {
	HTTP HTTPAuth `yaml:"http"`
}
type sharedApplicationConfig struct {
	HTTP Application `yaml:"application"`
}
type sharedAuthSignConfig struct {
	HTTP HTTPAuthSign `yaml:"http"`
}

func main() {
	driverName := flag.String("doc", "", UsageDocFlag)
	flag.Parse()

	yamlDriver, err := yaml.New("config.yaml")
	if err != nil {
		return
	}
	envDriver, err := env.New()
	if err != nil {
		return
	}
	tagDriver, err := tag.New("initial")
	if err != nil {
		return
	}

	config, err := tinyconf.New(
		tinyconf.WithLogger(logger.NewFmtLogger(logger.TRACE)),
		tinyconf.WithDriver(tagDriver),
		tinyconf.WithDriver(yamlDriver),
		tinyconf.WithDriver(envDriver),
	)
	if err != nil {
		return
	}

	c1 := &sharedAuthSignConfig{}
	if err = config.Register(c1); err != nil {
		panic(err)
	}

	c2 := &sharedConfig{}
	if err = config.Register(c2); err != nil {
		panic(err)
	}

	c3 := &sharedAuthConfig{}
	if err = config.Register(c3); err != nil {
		panic(err)
	}

	c4 := &sharedApplicationConfig{}
	if err = config.Register(c4); err != nil {
		panic(err)
	}

	if *driverName == "" {
		if err = config.Parse(c1); err != nil {
			panic(err)
		}
		if err = config.Parse(c2); err != nil {
			panic(err)
		}
		if err = config.Parse(c3); err != nil {
			panic(err)
		}
		if err = config.Parse(c4); err != nil {
			panic(err)
		}
	}

	if strings.Contains("env yaml", *driverName) {
		doc := config.GenDoc(*driverName)
		fmt.Println(doc)
		os.Exit(0)
	}

	flag.Usage()
	os.Exit(1)
}
