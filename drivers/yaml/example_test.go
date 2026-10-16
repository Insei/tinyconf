package yaml_test

import (
	"fmt"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/yaml"
)

// Several sources are merged in the order of addition: the next source overrides the keys of the previous one.
func ExampleNew() {
	type Config struct {
		HTTP struct {
			Host string `yaml:"host"`
			Port int    `yaml:"port"`
		} `yaml:"http"`
		Hosts []string `yaml:"hosts"`
	}
	driver, err := yaml.New("",
		yaml.WithData("defaults.yaml", []byte("http:\n  host: localhost\n  port: 80\nhosts: [a, b]\n")),
		yaml.WithData("config.local.yaml", []byte("http:\n  port: 9090\n")),
	)
	if err != nil {
		panic(err)
	}
	config, err := tinyconf.New(tinyconf.WithDriver(driver))
	if err != nil {
		panic(err)
	}
	conf := &Config{}
	if err = config.Register(conf); err != nil {
		panic(err)
	}
	if err = config.Parse(conf); err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", *conf)
	// Output: {HTTP:{Host:localhost Port:9090} Hosts:[a b]}
}
