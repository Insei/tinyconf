package flag_test

import (
	"context"
	"fmt"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/flag"
)

// Flags are read from the arguments, unknown flags are ignored, so the driver works next to any CLI framework.
func ExampleNew() {
	type Config struct {
		Port    int      `flag:"port" doc:"port of the server"`
		Hosts   []string `flag:"host" doc:"allowed hosts"`
		Verbose bool     `flag:"verbose" doc:"print debug messages"`
	}
	driver, err := flag.New(flag.WithArgs([]string{"serve", "--port=9090", "-host", "a", "-host", "b", "-verbose", "--unknown"}))
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
	if err = config.Parse(context.Background(), conf); err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", *conf)
	// Output: {Port:9090 Hosts:[a b] Verbose:true}
}
