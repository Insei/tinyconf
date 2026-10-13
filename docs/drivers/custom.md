# Custom drivers

A driver is any type that implements the `tinyconf.Driver` interface:

```go
type Driver interface {
    GenDoc(...*Registered) string
    GetName() string
    GetValue(field fmap.Field) (*Value, error)
}
```

| Method | Description |
|---|---|
| `GetName` | name of the driver, it is used in logs, errors and as an argument of `Manager.GenDoc` |
| `GetValue` | returns the value for the field, it is called for every field of the config |
| `GenDoc` | returns the documentation of the registered configs in the format of the driver, can return an empty string |

## GetValue

The field is described by [`fmap.Field`](https://pkg.go.dev/github.com/insei/fmap/v3#Field): `GetTag()` returns its struct tags, `GetStructPath()` — its path in the config, `HTTP.Port` for example.

The returned value should have exactly the type of the field. The bundled drivers convert strings with [`cast.ToReflect`](https://pkg.go.dev/github.com/insei/cast#ToReflect) from the `github.com/insei/cast` package, use it to support the same types.

Two errors have a special meaning, all other errors are returned from `Parse`:

| Error | Meaning |
|---|---|
| `tinyconf.ErrIncorrectTagSettings` | the field has no settings for this driver, it is skipped |
| `tinyconf.ErrValueNotFound` | the source has no value for the field, the value of the previous driver is kept |

## Example

A driver that reads values from a map, the name of the value is taken from the `kv` tag:

```go
type kvDriver struct {
    values map[string]string
}

func (d *kvDriver) GetName() string { return "kv" }

func (d *kvDriver) GenDoc(...*tinyconf.Registered) string { return "" }

func (d *kvDriver) GetValue(field fmap.Field) (*tinyconf.Value, error) {
    key, ok := field.GetTag().Lookup("kv")
    if !ok {
        return nil, tinyconf.ErrIncorrectTagSettings
    }
    raw, ok := d.values[key]
    if !ok {
        return nil, tinyconf.ErrValueNotFound
    }
    value, err := cast.ToReflect(raw, field.GetType())
    if err != nil {
        return nil, fmt.Errorf("failed to parse %s: %w", key, err)
    }
    return &tinyconf.Value{Source: "kv", Value: value}, nil
}
```

```go
type Config struct {
    Port  int  `kv:"port"`
    Debug bool `kv:"debug"`
}

config, err := tinyconf.New(tinyconf.WithDriver(&kvDriver{values: map[string]string{
    "port":  "8080",
    "debug": "true",
}}))
```
