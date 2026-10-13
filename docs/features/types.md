# Supported types

All drivers support the same set of types. It is checked by the conformance tests in [`drivers/conformance_test.go`](https://github.com/Insei/tinyconf/blob/main/drivers/conformance_test.go): every type is parsed by every driver, and the results should be equal.

Drivers get values in two forms:

* **string values** — environment variables and struct tags;
* **native values** — YAML, where lists and maps are a part of the format.

| Types | String value | Native value |
|---|---|---|
| `string`, `bool`, integers, floats and named types based on them | `value` | scalar |
| `time.Duration` | `1m30s` | `1m30s` |
| `time.Time` | `2024-01-02T03:04:05Z`, `2024-01-02 03:04:05`, `2024-01-02` | the same or yaml timestamp |
| `url.URL`, `[]byte`, any `encoding.TextUnmarshaler` (`net.IP`, `uuid.UUID`, ...) | `value` | scalar |
| `[]T`, `[N]T` | `a,b,c` | list or string |
| `map[K]V` | `a:1,b:2` | map or string |
| structs, `[]Struct`, `map[K]Struct`, nested slices and maps | JSON: `[{"host":"a","port":1}]` | native |
| pointers to all types above | the same | the same |

## Slices and arrays

```go
type Config struct {
    Hosts []string `env:"HOSTS" yaml:"hosts"`
    Ports [2]int   `env:"PORTS" yaml:"ports"`
}
```

=== "env"

    ```bash
    HOSTS=a.example.com,b.example.com
    PORTS=80,443
    ```

=== "yaml"

    ```yaml
    hosts: [a.example.com, b.example.com]
    ports:
      - 80
      - 443
    ```

Items that contain the separator can be quoted with double or single quotes: `"a,b",c` is two items. The separator can be changed with the [`separator` tag](tags.md#common-tags). The number of items for an array should be equal to its length.

## Maps

```go
type Config struct {
    Labels map[string]string `env:"LABELS" yaml:"labels"`
    Limits map[string]int    `env:"LIMITS" yaml:"limits" separator:";" kvseparator:"="`
}
```

=== "env"

    ```bash
    LABELS=env:prod,team:core
    LIMITS=cpu=2;memory=512
    ```

=== "yaml"

    ```yaml
    labels:
      env: prod
      team: core
    limits: {cpu: 2, memory: 512}
    ```

## Structs in slices and maps

Nested values are set as JSON in string values and as is in YAML:

```go
type Server struct {
    Host string `json:"host" yaml:"host"`
    Port int    `json:"port" yaml:"port"`
}

type Config struct {
    Servers []Server `env:"SERVERS" yaml:"servers"`
}
```

=== "env"

    ```bash
    SERVERS='[{"host":"a","port":1},{"host":"b","port":2}]'
    ```

=== "yaml"

    ```yaml
    servers:
      - host: a
        port: 1
      - host: b
        port: 2
    ```

String values of slices, arrays, maps and structs are parsed as JSON if they start with `[` or `{`. Keys of such structs are taken from the `json` tag for string values and from the `yaml` tag for YAML files, the field name is used if the tag is not set (case-insensitive).

!!! note "Fields with struct type"
    A field with struct type is not a single value: it is parsed by its fields, and every field has its own tags. See [Nested structs and sub configs](structs.md).

## Several drivers

If the value of a slice or map is provided by several drivers, the last driver replaces the whole value, items are not merged.
