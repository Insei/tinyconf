// The example service: the config is read from the defaults in the tags, config.yaml, environment variables and flags,
// the packages subscribe to their parts of the config and get the changes of config.yaml without restart.
//
//	go run .                                 # run, change config.yaml while it is running, stop by Ctrl+C
//	DB_PASSWORD=secret go run . -http-port 9090
//	go run . -doc yaml                       # print the template of the config: yaml, env, flag
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/insei/tinyconf/v2"
	"github.com/insei/tinyconf/v2/drivers/env"
	"github.com/insei/tinyconf/v2/drivers/flag"
	"github.com/insei/tinyconf/v2/drivers/tag"
	"github.com/insei/tinyconf/v2/drivers/yaml"
	"github.com/insei/tinyconf/v2/logger"
)

// HTTP is the config of the HTTP server package.
type HTTP struct {
	Host    string        `initial:"localhost" yaml:"host" env:"HTTP_HOST" flag:"http-host" doc:"host of the HTTP server"`
	Port    int           `initial:"8080" yaml:"port" env:"HTTP_PORT" flag:"http-port" doc:"port of the HTTP server"`
	Timeout time.Duration `initial:"30s" yaml:"timeout" env:"HTTP_TIMEOUT" doc:"timeout of the requests"`
}

// Database is the config of the database package.
type Database struct {
	Host     string          `initial:"localhost" yaml:"host" env:"DB_HOST" doc:"host of the database"`
	User     string          `initial:"app" yaml:"user" env:"DB_USER" doc:"user of the database"`
	Password tinyconf.Secret `yaml:"password" env:"DB_PASSWORD" required:"true" doc:"password of the database"`
}

// Logging is the config of the logging package.
type Logging struct {
	Level string `initial:"info" yaml:"level" env:"LOG_LEVEL" flag:"log-level" doc:"log level: debug, info, warn, error"`
}

// Config is the config of the whole application, the packages know only their parts.
type Config struct {
	HTTP     HTTP     `yaml:"http"`
	Database Database `yaml:"database"`
	Logging  Logging  `yaml:"logging"`
	// the hosts from the defaults, the file and the environment are merged
	Hosts []string `initial:"localhost" yaml:"hosts" env:"HOSTS" merge:"true" doc:"allowed hosts"`
	// the flag of the application itself is a field of the config too
	Doc string `flag:"doc" doc:"print the template of the config and exit: yaml, env, flag"`
}

func must[T any](value T, err error) T {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return value
}

func main() {
	config := must(tinyconf.New(
		tinyconf.WithLogger(logger.NewFmtLogger(logger.ERROR)),
		// the next driver overrides the values of the previous one
		tinyconf.WithDriver(must(tag.New("initial"))),
		tinyconf.WithDriver(must(yaml.New("config.yaml", yaml.WithOptional()))),
		tinyconf.WithDriver(must(env.New(env.WithFiles()))),
		tinyconf.WithDriver(must(flag.New())),
	))
	conf := &Config{}
	if err := config.Register(conf); err != nil {
		panic(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	parseCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := config.Parse(parseCtx, conf)
	if conf.Doc != "" {
		fmt.Print(config.GenDoc(conf.Doc))
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "config is not parsed:", err)
		os.Exit(1)
	}

	// the password is masked when the config is printed
	fmt.Printf("config: %+v\n", *conf)
	for _, source := range config.Sources(conf) {
		fmt.Printf("%s is set by %s driver from %s\n", source.Field, source.Driver, source.Source)
	}

	// the packages subscribe to their parts and get only their changes
	must(0, tinyconf.Subscribe(ctx, config, func(prev, next *HTTP) {
		fmt.Printf("http config is changed: %+v -> %+v\n", *prev, *next)
	}))
	must(0, tinyconf.Subscribe(ctx, config, func(prev, next *Logging) {
		fmt.Printf("log level is changed: %s -> %s\n", prev.Level, next.Level)
	}))
	fmt.Println("change config.yaml to see the changes, stop by Ctrl+C")
	<-ctx.Done()

	// the current version is available anywhere
	fmt.Println("the last http config:", *tinyconf.Current[HTTP](config))
}
