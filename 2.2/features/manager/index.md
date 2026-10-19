# Manager

`tinyconf.Manager` holds the list of drivers and the registered configs. It is created once at the start of the application.

## Setup

```go
config, err := tinyconf.New(
    tinyconf.WithLogger(logger.NewFmtLogger(logger.INFO)),
    tinyconf.WithDriver(tagDriver),
    tinyconf.WithDriver(yamlDriver),
    tinyconf.WithDriver(envDriver),
)
```

| Option                 | Description                                                                                                                                                               |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `WithDriver(driver)`   | adds the driver, can be used many times                                                                                                                                   |
| `WithLogger(logger)`   | sets the logger, see [Logging](https://insei.github.io/tinyconf/2.2/features/logging/index.md)                                                                            |
| `WithDeferredLogger()` | keeps the log records until the logger is set by `SetLogger`, see [Logger from the config](https://insei.github.io/tinyconf/2.2/features/logging/#logger-from-the-config) |

## Priority of drivers

Drivers are applied in the order of addition. The next driver overrides values of the previous one, so **the last driver has the highest priority**.

A typical order is: defaults from tags, then files, then environment variables.

A driver overrides the value only if it has a value for the field. A field that is absent in the file keeps the value from the previous driver, and a field without values in all drivers keeps the value that was in the struct before `Parse`.

## Register

```go
c := &Config{}
err := config.Register(c)
```

`Register` takes a pointer to struct. The registered config is used by `Parse` and by [`GenDoc`](https://insei.github.io/tinyconf/2.2/features/gendoc/index.md).

## Parse

```go
err := config.Parse(context.Background(), c)
```

`Parse` fills the registered config with values from drivers. The config can be a registered one or a field of the registered config with struct type, see [sub configs](https://insei.github.io/tinyconf/2.2/features/structs/#sub-configs). `tinyconf.ErrNotRegisteredConfig` is returned for all other configs.

The context is used by the drivers that read their sources by the network — [consul](https://insei.github.io/tinyconf/2.2/drivers/consul/index.md), [vault](https://insei.github.io/tinyconf/2.2/drivers/vault/index.md), [etcd](https://insei.github.io/tinyconf/2.2/drivers/etcd/index.md) and the remote documents: their requests are stopped when the context is done. A start of the application in Kubernetes is a typical case:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
if err := config.Parse(ctx, c); err != nil {
    log.Fatal(err) // errors.Is(err, context.DeadlineExceeded) if the storage is not available in time
}
```

Migration

`Parse(conf)` of the previous versions is `Parse(context.Background(), conf)`, and `ParseContext(ctx, conf)` is `Parse(ctx, conf)`.

Every request of the drivers is limited to 30 seconds anyway. The reloads use the context of watching, so the requests are stopped when the last subscription is canceled.

## Other methods

| Method                                 | Description                                                                                                                                               |
| -------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GenDoc(driverName)`                   | generates a template of all registered configs, see [Documentation generation](https://insei.github.io/tinyconf/2.2/features/gendoc/index.md)             |
| `Sources(conf)`                        | returns which driver has set the value of every field, see [Value sources](https://insei.github.io/tinyconf/2.2/features/sources/index.md)                |
| `tinyconf.Subscribe(ctx, m, onChange)` | calls `onChange` after the changes of the config or the sub config, see [Reloading](https://insei.github.io/tinyconf/2.2/features/reloading/index.md)     |
| `tinyconf.Current[T](m)`               | returns the current version of the config or the sub config, see [Reloading](https://insei.github.io/tinyconf/2.2/features/reloading/index.md)            |
| `ReloadStatus()`                       | state of watching and reloading for the health checks, see [Health checks](https://insei.github.io/tinyconf/2.2/features/reloading/#health-checks)        |
| `SetLogger(logger)`                    | replaces the logger, safe for concurrent use, see [Logger from the config](https://insei.github.io/tinyconf/2.2/features/logging/#logger-from-the-config) |

## Errors

`Parse` does not stop on the first error: all fields are parsed, and all errors are returned as one error. It can be checked with `errors.Is` and `errors.As`.

```go
if err := config.Parse(context.Background(), c); err != nil {
    // env driver: field HTTP.Port: failed to parse env value from key HTTP_PORT ...
    log.Fatal(err)
}
```

Fields that were parsed without errors are filled even if `Parse` returns an error.

Missing [required fields](https://insei.github.io/tinyconf/2.2/features/required/index.md) are reported in the same error and can be checked with `errors.Is(err, tinyconf.ErrRequired)`.

Not an error

A field without a value in the driver and a field without the tag of the driver are not errors, the manager just goes to the next driver.

## Concurrency

All methods of the manager are safe for concurrent use: configs can be registered and parsed from several goroutines. The config struct itself is not protected, do not read it while it is being parsed by another goroutine.

Configs are documented by `GenDoc` and checked for [sub configs](https://insei.github.io/tinyconf/2.2/features/structs/#sub-configs) in the registration order.
