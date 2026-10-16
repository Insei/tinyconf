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

The returned value should have exactly the type of the field. To support [the same types](../features/types.md) as the bundled drivers, use the `convert` package:

* `convert.StringToField(s, field)` — for sources with string values;
* `convert.TreeToField(v, field, tag)` — for values decoded from formats like YAML or JSON.

The returned `tinyconf.Value` has three fields:

| Field | Description |
|---|---|
| `Value` | the value with the type of the field |
| `Source` | the key of the value in the source, `HTTP_PORT` or `http.port` for example; it is returned by [`Manager.Sources`](../features/sources.md) and logged as `key` |
| `Warning` | optional message for the user, the manager logs it with the `Warn` level; use it when the value is found, but probably not where the user expects |

Two errors have a special meaning, all other errors are returned from `Parse`:

| Error | Meaning |
|---|---|
| `tinyconf.ErrIncorrectTagSettings` | the field has no settings for this driver, it is skipped |
| `tinyconf.ErrValueNotFound` | the source has no value for the field, the value of the previous driver is kept |

Wrap `tinyconf.ErrAccessDenied` into the error if the source denies access: the token has no permission to read the values. It is still an error of `Parse`, but the manager logs it as `access denied`, so the problem with permissions is not lost among network errors, and the application can check it with `errors.Is`. Return it from `Load`, `GetValue` and `Watch`; `Watch` should stop on it, retries do not fix the permissions.

## Loader

`GetValue` is called for every field. If the source should be read once — a file, a response of the remote service — implement the optional `tinyconf.Loader` interface:

```go
type Loader interface {
    Load() error
}
```

`Load` is called once at the start of every `Parse` call, before any `GetValue` call. If it returns an error, the driver is skipped in this `Parse` call and the error is returned from `Parse`.

```go
func (d *kvDriver) Load() error {
    values, err := d.client.FetchAll()
    if err != nil {
        return err
    }
    d.values = values
    return nil
}
```

## Watcher

A driver can report changes of its source for [`Manager.Watch`](../features/reloading.md) by implementing the optional `tinyconf.Watcher` interface:

```go
type Watcher interface {
    Watch(ctx context.Context, onChange func()) error
}
```

`Watch` should block until the context is done and call `onChange` after every change of the source. The manager parses the config again after the call, so `Load` and `GetValue` should return the new values.

```go
func (d *kvDriver) Watch(ctx context.Context, onChange func()) error {
    events := d.client.Subscribe(ctx)
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-events:
            onChange()
        }
    }
}
```

An error returned before the context is done stops `Manager.Watch`, it returns this error.

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
    value, err := convert.StringToField(raw, field)
    if err != nil {
        return nil, fmt.Errorf("failed to parse %s: %w", key, err)
    }
    return &tinyconf.Value{Source: key, Value: value}, nil
}
```

```go
type Config struct {
    Port  int      `kv:"port"`
    Hosts []string `kv:"hosts"`
}

config, err := tinyconf.New(tinyconf.WithDriver(&kvDriver{values: map[string]string{
    "port":  "8080",
    "hosts": "a,b",
}}))
```
