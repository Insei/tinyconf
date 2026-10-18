//go:build go1.21

package tinyconf_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/insei/tinyconf/v2"
	"github.com/insei/tinyconf/v2/drivers/tag"
	"github.com/insei/tinyconf/v2/logger"
)

type Logging struct {
	Level string `initial:"debug" doc:"log level: debug, info, warn, error"`
}

type AppConfig struct {
	Logging Logging
	Port    int `initial:"8080"`
}

// newLogger creates the logger of the application from its config, the time is removed for the stable output.
func newLogger(conf Logging) *slog.Logger {
	var level slog.Level
	_ = level.UnmarshalText([]byte(conf.Level))
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return attr
		},
	}))
}

// The logger of the application is configured by the config itself: the records of the parsing are kept
// until the logger is created from the parsed config.
func ExampleWithDeferredLogger() {
	config := must(tinyconf.New(
		tinyconf.WithDriver(must(tag.New("initial"))),
		tinyconf.WithDeferredLogger(),
	))
	conf := &AppConfig{}
	if err := config.Register(conf); err != nil {
		panic(err)
	}
	err := config.Parse(context.Background(), conf)

	// the kept records are written to the logger, the next records are written directly
	config.SetLogger(logger.NewSlogLogger(newLogger(conf.Logging)))
	if err != nil {
		panic(err)
	}
	fmt.Println("port:", conf.Port)
	// Output:
	// level=DEBUG msg=initial config=*tinyconf_test.AppConfig field=Logging.Level value=""
	// level=DEBUG msg=initial config=*tinyconf_test.AppConfig field=Port value=0
	// level=DEBUG msg=override config=*tinyconf_test.AppConfig driver=tag field=Logging.Level key=initial value=debug
	// level=DEBUG msg=override config=*tinyconf_test.AppConfig driver=tag field=Port key=initial value=8080
	// port: 8080
}
