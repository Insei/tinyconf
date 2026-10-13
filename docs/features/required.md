# Required fields

A field with the `required:"true"` tag should have a value. Otherwise `Parse` returns `tinyconf.ErrRequired`.

```go
type Config struct {
    DSN      string `env:"DSN" required:"true"`
    Password string `env:"PASSWORD" yaml:"password" hidden:"true" required:"true"`
    Port     int    `env:"PORT" initial:"8080"`
}
```

```go
err := config.Parse(c)
if errors.Is(err, tinyconf.ErrRequired) {
    // field DSN: required value is not set; field Password: required value is not set
    log.Fatal(err)
}
```

## What is a value

The field passes the check if **any** of these is true:

* some driver has set its value, even if the value is zero: `PORT=0` is a value;
* the field has a non-zero value in the config that was passed to `Parse`.

So a required field without sources can still be filled in the code before `Parse`:

```go
c := &Config{DSN: "postgres://localhost/dev"}
err := config.Parse(c) // DSN is not required to be in the environment
```

## All at once

`Parse` checks all required fields and reports all missing ones in one error, together with the [errors of drivers](manager.md#errors). The application can show the full list of what should be set in one start, not one variable per attempt.

!!! tip "Defaults and required"
    A field with a default value in the [tag driver](../drivers/tag.md) always has a value, so `required` makes sense only for the fields without defaults: secrets, addresses of external services.

## Sub configs

When a [sub config](structs.md#sub-configs) is parsed, only its own required fields are checked.
