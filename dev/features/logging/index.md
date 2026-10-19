# Logging

The manager logs what happens with every field during `Parse`.

| Level   | Message                                                                                                                                                                                       |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Debug` | `initial`: the value of the field before drivers; `override`: the value is overridden by the driver; `skip`: the driver has no value for the field                                            |
| `Warn`  | the field has no tag of the driver; the driver has found the value in an unexpected place                                                                                                     |
| `Error` | `access denied`: the source denies access to the values, `tinyconf.ErrAccessDenied`; `failed`, `failed to load`, `failed to watch`: the driver failed to get, to parse or to watch the values |

Messages have the fields `config`, `driver`, `field`, `key` and `details` or `value`. Values of the fields with `hidden:"true"` are masked.

## History of a field

On the debug level the log has the whole history of every field: grep the log by the name of the field to see where its value came from.

- At the start of every `Parse` every field is logged with its initial value: the value in the struct before drivers, usually zero.
- Every driver that has changed the value of the field is logged with `override`. A driver that has the same value as the field already has is not logged.
- After every [reload](https://insei.github.io/tinyconf/dev/features/reloading/index.md) only the fields with a new value are logged, with the driver that has set it. Unchanged fields are not logged.

```text
[DEBUG] initial: {config: *main.Config} {field: HTTP.Port} {value: 0}
[DEBUG] override: {config: *main.Config} {driver: tag} {field: HTTP.Port} {key: initial} {value: 8080}
[DEBUG] override: {config: *main.Config} {driver: yaml} {field: HTTP.Port} {key: http.port} {value: 9000}
[DEBUG] override: {config: *main.Config} {driver: env} {field: HTTP.Port} {key: HTTP_PORT} {value: 9090}
...
[DEBUG] override: {config: *main.Config} {driver: env} {field: HTTP.Port} {key: HTTP_PORT} {value: 9091}
```

`field` is the struct path of the field in the registered config, `key` is the place of the value in the source of the driver:

| Driver                            | `key`                                                                                                                                              |
| --------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| tag                               | name of the tag, `initial`                                                                                                                         |
| yaml, json, toml, ini, properties | path by the tags, `http.port`                                                                                                                      |
| env                               | name of the variable, with the prefix; `HOSTS_*` for the value from [indexed keys](https://insei.github.io/tinyconf/dev/drivers/env/#indexed-keys) |
| flag                              | name of the flag, `-http-port`                                                                                                                     |
| consul                            | the key, `app/http/port`                                                                                                                           |
| vault                             | path by the tags, `database.password`                                                                                                              |
| etcd                              | the key, `app/http/port`                                                                                                                           |

If a field has lost its value in all drivers after a reload, it gets its initial value, and it is logged without `driver` and `key`.

## Warnings of drivers

A driver can attach a warning to the value. The value is still used, and the warning is logged with the `Warn` level.

The yaml, json, toml, ini, properties, vault and format drivers warn when a parent of the field has no tag of the driver. The path to the value is built from the tags, so without the tag of the parent the value is read from another key:

```go
type Config struct {
    HTTP struct { // no yaml tag
        Host string `yaml:"host"`
    }
}
```

```text
[WARN] warning: {config: *main.Config} {driver: yaml} {field: HTTP.Host} {details: parent fields HTTP have no 'yaml' tag, the value is read from the 'host' key}
```

Add the tag to the parent, `yaml:"http"`, to read the value from `http.host`.

## Logger from the config

The level, the format or the output of the logger are often a part of the config, but the config is parsed by the manager that already needs the logger. `WithDeferredLogger` breaks this cycle: the manager keeps the log records in memory, and `SetLogger` writes them to the logger created from the parsed config.

```go
config, _ := tinyconf.New(
    tinyconf.WithDriver(tagDriver),
    tinyconf.WithDriver(yamlDriver),
    tinyconf.WithDeferredLogger(), // the records are kept until SetLogger
)
conf := &Config{}
_ = config.Register(conf)
err := config.Parse(ctx, conf)

log := newLogger(conf.Logging)                   // the logger of the application from its config
config.SetLogger(logger.NewSlogLogger(log))      // the kept records are written to it
if err != nil {
    log.Error("config is not parsed", "error", err)
    os.Exit(1)
}

// the logger follows the changes of the config without restart
_ = tinyconf.Subscribe(ctx, config, func(_, next *Logging) {
    config.SetLogger(logger.NewSlogLogger(newLogger(*next)))
})
```

- The records of the first `Parse` get to the logger of the application in its format, its level filters them.
- `SetLogger` is safe for concurrent use with `Parse`, reloads and other goroutines. The records written during `SetLogger` wait for the kept ones, so nothing is lost and the order is kept.
- Up to 1000 records are kept, the oldest are dropped, and the logger gets a warning with the count of the dropped records.
- `SetLogger(nil)` disables logging, the kept records are dropped.
- `SetLogger` replaces the logger set by `WithLogger` too, without `WithDeferredLogger` nothing is kept.

## Bundled loggers

```go
import "github.com/insei/tinyconf/v2/logger"

// prints to stdout, levels: FATAL, ERROR, WARN, INFO, DEBUG, TRACE
tinyconf.WithLogger(logger.NewFmtLogger(logger.INFO))

// wraps *slog.Logger, Go 1.21 or newer
tinyconf.WithLogger(logger.NewSlogLogger(slog.Default()))
```

The adapter of [zap](https://github.com/uber-go/zap) is a separate module, so zap is not a dependency of tinyconf for the applications with other loggers:

```bash
go get github.com/insei/tinyconf/logger/zaplogger
```

```go
import "github.com/insei/tinyconf/logger/zaplogger"

// wraps *zap.Logger
tinyconf.WithLogger(zaplogger.New(zapLogger))
```

Migration

`logger.NewZapLogger(l)` of the previous versions is `zaplogger.New(l)` from the `github.com/insei/tinyconf/logger/zaplogger` module.

## Custom logger

Implement the `tinyconf.Logger` interface:

```go
type Logger interface {
    Debug(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    With(fields ...Field) Logger
}
```

`tinyconf.Field` is a pair of `Key` and `Value`.
