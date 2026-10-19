# Nested structs and sub configs

## Nested structs

Fields can be grouped into structs. A field with struct type is parsed by its fields, every nested field has its own tags:

```go
type HTTP struct {
    Host string `yaml:"host" env:"HTTP_HOST" initial:"localhost"`
    Port int    `yaml:"port" env:"HTTP_PORT" initial:"8080"`
}

type Config struct {
    HTTP HTTP `yaml:"http"`
}
```

```yaml
http:
  host: example.com
  port: 9090
```

For the yaml driver the tag of the struct field is a key of the nested map, so `Host` is read from `http.host`. Names of environment variables are not built from the parent names: `Host` is read from `HTTP_HOST`, as it is written in its tag.

Types that are parsed from a single value — `time.Time`, `url.URL` and types with `UnmarshalText` — are not nested structs, see [Supported types](https://insei.github.io/tinyconf/2.2/features/types/index.md).

## Embedded structs

Fields of an embedded struct without the tag are on the level of the embedding struct, like in `encoding/json`:

```go
type Timeouts struct {
    Read  time.Duration `yaml:"read_timeout"`
    Write time.Duration `yaml:"write_timeout"`
}

type Config struct {
    HTTP struct {
        Timeouts
        Port int `yaml:"port"`
    } `yaml:"http"`
}
```

```yaml
http:
  port: 9090
  read_timeout: 5s
  write_timeout: 10s
```

- The yaml, json, toml, ini, properties, consul, vault and etcd drivers read `Read` from `http.read_timeout`, the embedded struct adds no level. `yaml:",inline"` works the same way.
- With the name in the tag, `` Timeouts `yaml:"timeouts"` ``, the embedded struct is a usual nested struct: `http.timeouts.read_timeout`.
- An embedded struct without the tag is expected, the manager does not [warn](https://insei.github.io/tinyconf/2.2/features/logging/#warnings-of-drivers) about it.
- [Auto keys](https://insei.github.io/tinyconf/2.2/drivers/env/#auto-keys) and [indexed keys](https://insei.github.io/tinyconf/2.2/drivers/env/#indexed-keys) of the env driver skip the name of the embedded struct too: `HTTP.Timeouts.Read` is `HTTP_READ` with auto keys, and `SERVERS_0_READ` for the struct in the slice.

## Structs stored by pointer

A struct stored by pointer is parsed the same way, by its fields:

```go
type TLS struct {
    Cert string `yaml:"cert" env:"TLS_CERT"`
    Key  string `yaml:"key" env:"TLS_KEY"`
}

type Config struct {
    TLS *TLS `yaml:"tls"`
}
```

The pointer makes the section **optional**:

- the struct is created when the first value for its fields is found in any driver;
- without values the pointer is not changed, so it stays `nil`;
- an existing struct is filled in place, it is not replaced with a new one.

```go
if err := config.Parse(context.Background(), c); err != nil {
    panic(err)
}
if c.TLS != nil {
    // TLS_CERT, TLS_KEY or the tls section of the file is set
}
```

[Required fields](https://insei.github.io/tinyconf/2.2/features/required/index.md) inside such struct are checked even if the pointer is `nil`: a required field makes the whole section mandatory.

Parsed as a single value

Pointers to pointers (`**Struct`) and recursive types (`Next *Node` inside `Node`) are not parsed by fields. They are single values: JSON in string values, a nested map in files.

## Sub configs

A part of the registered config can be parsed separately, by the type of the field. It allows a package to get its own config without knowing about the whole one:

```go
c := &Config{}
if err := config.Register(c); err != nil {
    panic(err)
}

// HTTP is a type of the field in the registered Config
http := &HTTP{}
if err := config.Parse(context.Background(), http); err != nil {
    panic(err)
}
```

The type can be a type of the struct stored by value or by pointer. Only the fields of the sub config are parsed, with the same tags and keys as in the whole config: `http.host` in the file, `HTTP_HOST` in the environment.

If several registered configs have a field with this type, the configs are checked in the registration order and the first found field is used.
