# Logging

The manager logs what happens with every field during `Parse`.

| Level | Message |
|---|---|
| `Debug` | the driver has no value for the field, the value is overridden by the driver |
| `Warn` | the field has no tag of the driver; the driver has found the value in an unexpected place |
| `Error` | the driver failed to get or to parse the value |

Every message has the fields `config`, `driver`, `field` and `details` or `value`. Values of the fields with `hidden:"true"` are masked.

## Warnings of drivers

A driver can attach a warning to the value. The value is still used, and the warning is logged with the `Warn` level.

The yaml, json and toml drivers warn when a parent of the field has no tag of the driver. The path to the value is built from the tags, so without the tag of the parent the value is read from another key:

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

## Bundled loggers

```go
import "github.com/insei/tinyconf/logger"

// prints to stdout, levels: FATAL, ERROR, WARN, INFO, DEBUG, TRACE
tinyconf.WithLogger(logger.NewFmtLogger(logger.INFO))

// wraps *zap.Logger
tinyconf.WithLogger(logger.NewZapLogger(zapLogger))
```

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
