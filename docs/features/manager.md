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

| Option | Description |
|---|---|
| `WithDriver(driver)` | adds the driver, can be used many times |
| `WithLogger(logger)` | sets the logger, see [Logging](logging.md) |

## Priority of drivers

Drivers are applied in the order of addition. The next driver overrides values of the previous one, so **the last driver has the highest priority**.

A typical order is: defaults from tags, then files, then environment variables.

A driver overrides the value only if it has a value for the field. A field that is absent in the file keeps the value from the previous driver, and a field without values in all drivers keeps the value that was in the struct before `Parse`.

## Register

```go
c := &Config{}
err := config.Register(c)
```

`Register` takes a pointer to struct. The registered config is used by `Parse` and by [`GenDoc`](gendoc.md).

## Parse

```go
err := config.Parse(c)
```

`Parse` fills the registered config with values from drivers. The config can be a registered one or a field of the registered config with struct type, see [sub configs](structs.md#sub-configs). `tinyconf.ErrNotRegisteredConfig` is returned for all other configs.

## Other methods

| Method | Description |
|---|---|
| `GenDoc(driverName)` | generates a template of all registered configs, see [Documentation generation](gendoc.md) |
| `Sources(conf)` | returns which driver has set the value of every field, see [Value sources](sources.md) |
| `Watch(ctx, conf, onChange)` | parses the config to the new copy after every change of the sources, see [Reloading](reloading.md) |

## Errors

`Parse` does not stop on the first error: all fields are parsed, and all errors are returned as one error. It can be checked with `errors.Is` and `errors.As`.

```go
if err := config.Parse(c); err != nil {
    // env driver: field HTTP.Port: failed to parse env value from key HTTP_PORT ...
    log.Fatal(err)
}
```

Fields that were parsed without errors are filled even if `Parse` returns an error.

Missing [required fields](required.md) are reported in the same error and can be checked with `errors.Is(err, tinyconf.ErrRequired)`.

!!! note "Not an error"
    A field without a value in the driver and a field without the tag of the driver are not errors, the manager just goes to the next driver.

## Concurrency

All methods of the manager are safe for concurrent use: configs can be registered and parsed from several goroutines. The config struct itself is not protected, do not read it while it is being parsed by another goroutine.

Configs are documented by `GenDoc` and checked for [sub configs](structs.md#sub-configs) in the registration order.
