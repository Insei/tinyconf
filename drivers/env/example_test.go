package env_test

import (
	"fmt"
	"os"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/env"
)

// With the prefix, auto keys and indexed keys the variables are found without the env tags of every field.
func ExampleNew() {
	os.Setenv("APP_HTTP_MAX_CONNS", "100")
	os.Setenv("APP_SERVERS_0_HOST", "a.example.com")
	os.Setenv("APP_SERVERS_1_HOST", "b.example.com")
	defer func() {
		for _, name := range []string{"APP_HTTP_MAX_CONNS", "APP_SERVERS_0_HOST", "APP_SERVERS_1_HOST"} {
			os.Unsetenv(name)
		}
	}()

	type Server struct {
		Host string
	}
	type Config struct {
		HTTP struct {
			MaxConns int
		}
		Servers []Server
	}
	driver, err := env.New(env.WithPrefix("APP_"), env.WithAutoKeys(), env.WithIndexedKeys(), env.WithFiles())
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
	// Output: {HTTP:{MaxConns:100} Servers:[{Host:a.example.com} {Host:b.example.com}]}
}
