# Supported types

All drivers support the same set of types. It is checked by the conformance tests in [`drivers/conformance_test.go`](https://github.com/Insei/tinyconf/blob/main/drivers/conformance_test.go): every type is parsed by every driver, and the results should be equal.

Drivers get values in two forms:

- **string values** — environment variables, command line flags and struct tags;
- **native values** — YAML, JSON and TOML, where lists and maps are a part of the format.

| Types                                                                            | String value                                                | Native value               |
| -------------------------------------------------------------------------------- | ----------------------------------------------------------- | -------------------------- |
| `string`, `bool`, integers, floats and named types based on them                 | `value`                                                     | scalar                     |
| `time.Duration`                                                                  | `1m30s`                                                     | `1m30s`                    |
| `time.Time`                                                                      | `2024-01-02T03:04:05Z`, `2024-01-02 03:04:05`, `2024-01-02` | the same or yaml timestamp |
| `url.URL`, `[]byte`, any `encoding.TextUnmarshaler` (`net.IP`, `uuid.UUID`, ...) | `value`                                                     | scalar                     |
| `[]T`, `[N]T`                                                                    | `a,b,c`                                                     | list or string             |
| `map[K]V`                                                                        | `a:1,b:2`                                                   | map or string              |
| `[]Struct`, `map[K]Struct`, nested slices and maps                               | JSON: `[{"host":"a","port":1}]`                             | native                     |
| pointers to all types above                                                      | the same                                                    | the same                   |
| `Struct`, `*Struct`                                                              | parsed by fields                                            | parsed by fields           |

## Slices and arrays

```go
type Config struct {
    Hosts []string `env:"HOSTS" yaml:"hosts"`
    Ports [2]int   `env:"PORTS" yaml:"ports"`
}
```

```bash
HOSTS=a.example.com,b.example.com
PORTS=80,443
```

```yaml
hosts: [a.example.com, b.example.com]
ports:
  - 80
  - 443
```

Items that contain the separator can be quoted with double or single quotes: `"a,b",c` is two items. The separator can be changed with the [`separator` tag](https://insei.github.io/tinyconf/2.2/features/tags/#common-tags). The number of items for an array should be equal to its length.

## Maps

```go
type Config struct {
    Labels map[string]string `env:"LABELS" yaml:"labels"`
    Limits map[string]int    `env:"LIMITS" yaml:"limits" separator:";" kvseparator:"="`
}
```

```bash
LABELS=env:prod,team:core
LIMITS=cpu=2;memory=512
```

```yaml
labels:
  env: prod
  team: core
limits: {cpu: 2, memory: 512}
```

## Structs in slices and maps

Nested values are set as JSON in string values and as is in YAML, JSON and TOML files:

```go
type Server struct {
    Host string `json:"host" yaml:"host"`
    Port int    `json:"port" yaml:"port"`
}

type Config struct {
    Servers []Server `env:"SERVERS" yaml:"servers"`
}
```

```bash
SERVERS='[{"host":"a","port":1},{"host":"b","port":2}]'
```

```yaml
servers:
  - host: a
    port: 1
  - host: b
    port: 2
```

String values of slices, arrays, maps and structs are parsed as JSON if they start with `[` or `{`. Keys of such structs are taken from the `json` tag for string values and JSON files, from the `yaml` tag for YAML files and from the `toml` tag for TOML files, the field name is used if the tag is not set (case-insensitive).

Fields with struct type

A field with struct type, stored by value or by pointer, is not a single value: it is parsed by its fields, and every field has its own tags. See [Nested structs and sub configs](https://insei.github.io/tinyconf/2.2/features/structs/index.md).

## Secrets

`tinyconf.Secret` is a string that is not printed. `hidden:"true"` masks the value in the logs of tinyconf, but the application can print or log the whole config itself; the `Secret` type masks the value there too:

```go
type Config struct {
    Database struct {
        User     string          `env:"DB_USER"`
        Password tinyconf.Secret `env:"DB_PASSWORD"`
    }
}

log.Printf("config: %+v", conf)            // {Database:{User:app Password:******}}
db.Connect(conf.Database.User, conf.Database.Password.Value())
```

- `String`, `GoString` and `MarshalText` return `******` for a not empty value, so `fmt`, `encoding/json`, yaml, zap and slog print the mask. The length of the value is not shown; an empty value is shown as empty, so it is visible that the secret is not set.
- The value is parsed like a usual string by all drivers, slices and pointers of secrets work too.
- `Value()` or `string(secret)` returns the value.
- [`GenDoc`](https://insei.github.io/tinyconf/2.2/features/gendoc/index.md) shows `******` as the value of the field.

## Several drivers

If the value of a slice or map is provided by several drivers, the last driver replaces the whole value. With the `merge:"true"` tag the values are merged:

```go
type Config struct {
    Hosts  []string          `initial:"default" yaml:"hosts" env:"HOSTS" merge:"true"`
    Labels map[string]string `initial:"team:core" yaml:"labels" env:"LABELS" merge:"true"`
}
```

```bash
# config.yaml: hosts: [from-file], labels: {env: dev}
HOSTS=from-env LABELS=env:prod
# Hosts:  [default from-file from-env]
# Labels: map[env:prod team:core]
```

- Slices are appended in the order of the drivers, pointers to slices too.
- Maps get the keys of the next drivers, the same key is overridden by the next driver.
- The values of the drivers in one `Parse` call are merged: the first driver with the value replaces the value of the struct, so the next `Parse` does not duplicate the items.
- Arrays, `[]byte` and other types are replaced.
- [`Sources`](https://insei.github.io/tinyconf/2.2/features/sources/index.md) reports the last driver, the [log](https://insei.github.io/tinyconf/2.2/features/logging/index.md) has every step.
