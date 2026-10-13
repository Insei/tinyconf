# Logging

The manager logs what happens with every field during `Parse`.

| Level | Message |
|---|---|
| `Debug` | the driver has no value for the field, the value is overridden by the driver |
| `Warn` | the field has no tag of the driver |
| `Error` | the driver failed to get or to parse the value |

Every message has the fields `config`, `driver`, `field` and `details` or `value`. Values of the fields with `hidden:"true"` are masked.

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
