package tinyconf_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/env"
	"github.com/insei/tinyconf/drivers/flag"
	"github.com/insei/tinyconf/drivers/tag"
	"github.com/insei/tinyconf/drivers/yaml"
)

type HTTP struct {
	Host string `initial:"localhost" yaml:"host" env:"EXAMPLE_HTTP_HOST" flag:"http-host" doc:"host of the server"`
	Port int    `initial:"8080" yaml:"port" env:"EXAMPLE_HTTP_PORT" flag:"http-port" doc:"port of the server"`
}

type Config struct {
	Name     string          `initial:"app" yaml:"name" doc:"name of the application"`
	HTTP     HTTP            `yaml:"http"`
	Password tinyconf.Secret `yaml:"password" env:"EXAMPLE_PASSWORD"`
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// The config is filled by the chain of drivers, the next driver overrides the values of the previous one.
func Example() {
	os.Setenv("EXAMPLE_HTTP_HOST", "example.com")
	defer os.Unsetenv("EXAMPLE_HTTP_HOST")

	config := must(tinyconf.New(
		tinyconf.WithDriver(must(tag.New("initial"))),
		tinyconf.WithDriver(must(yaml.New("", yaml.WithData("config.yaml", []byte("http:\n  port: 9090\n"))))),
		tinyconf.WithDriver(must(env.New(env.WithFiles()))),
		tinyconf.WithDriver(must(flag.New(flag.WithArgs([]string{"-http-port", "9091"})))),
	))
	conf := &Config{}
	if err := config.Register(conf); err != nil {
		panic(err)
	}
	if err := config.Parse(context.Background(), conf); err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", *conf)
	// Output: {Name:app HTTP:{Host:example.com Port:9091} Password:}
}

// Sources tells which driver and which key gave the final value of every field.
func ExampleManager_Sources() {
	config := must(tinyconf.New(
		tinyconf.WithDriver(must(tag.New("initial"))),
		tinyconf.WithDriver(must(yaml.New("", yaml.WithData("config.yaml", []byte("http:\n  port: 9090\n"))))),
	))
	conf := &Config{}
	if err := config.Register(conf); err != nil {
		panic(err)
	}
	if err := config.Parse(context.Background(), conf); err != nil {
		panic(err)
	}
	for _, source := range config.Sources(conf) {
		fmt.Printf("%s: %s driver, %s\n", source.Field, source.Driver, source.Source)
	}
	// Output:
	// Name: tag driver, initial
	// HTTP.Host: tag driver, initial
	// HTTP.Port: yaml driver, http.port
}

// GenDoc generates the template of the config in the format of the driver, with the current values.
func ExampleManager_GenDoc() {
	config := must(tinyconf.New(
		tinyconf.WithDriver(must(tag.New("initial"))),
		tinyconf.WithDriver(must(env.New(env.WithFiles()))),
	))
	conf := &Config{}
	if err := config.Register(conf); err != nil {
		panic(err)
	}
	if err := config.Parse(context.Background(), conf); err != nil {
		panic(err)
	}
	fmt.Print(config.GenDoc("env"))
	// Output:
	// #host of the server
	// #EXAMPLE_HTTP_HOST=localhost
	// #port of the server
	// #EXAMPLE_HTTP_PORT=8080
	// #
	// #EXAMPLE_PASSWORD=
}

// A package parses its own part of the config, it knows nothing about the whole config.
func ExampleManager_Parse_subConfig() {
	config := must(tinyconf.New(tinyconf.WithDriver(must(tag.New("initial")))))
	if err := config.Register(&Config{}); err != nil {
		panic(err)
	}

	http := &HTTP{}
	if err := config.Parse(context.Background(), http); err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", *http)
	// Output: {Host:localhost Port:8080}
}

// A package subscribes to its part of the config and gets only its changes.
func ExampleSubscribe() {
	dir, err := os.MkdirTemp("", "example")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "config.yaml")
	if err = os.WriteFile(file, []byte("http:\n  port: 9090\n"), 0o600); err != nil {
		panic(err)
	}

	config := must(tinyconf.New(
		tinyconf.WithDriver(must(tag.New("initial"))),
		// the file is watched by the events of the file system and checked by polling
		tinyconf.WithDriver(must(yaml.New(file, yaml.WithWatchInterval(10*time.Millisecond)))),
	))
	conf := &Config{}
	if err = config.Register(conf); err != nil {
		panic(err)
	}
	if err = config.Parse(context.Background(), conf); err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan struct{})
	err = tinyconf.Subscribe(ctx, config, func(prev, next *HTTP) {
		fmt.Printf("port: %d -> %d\n", prev.Port, next.Port)
		close(changed)
	})
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile(file, []byte("http:\n  port: 9091\n"), 0o600); err != nil {
		panic(err)
	}
	<-changed
	fmt.Println("current port:", tinyconf.Current[HTTP](config).Port)
	// Output:
	// port: 9090 -> 9091
	// current port: 9091
}

// Secret is masked when the application prints the config.
func ExampleSecret() {
	conf := Config{Name: "app", Password: "password"}
	fmt.Printf("%+v\n", conf)
	fmt.Println(conf.Password.Value())
	// Output:
	// {Name:app HTTP:{Host: Port:0} Password:******}
	// password
}
