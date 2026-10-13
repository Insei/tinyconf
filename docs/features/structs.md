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

Types that are parsed from a single value — `time.Time`, `url.URL` and types with `UnmarshalText` — are not nested structs, see [Supported types](types.md).

## Sub configs

A part of the registered config can be parsed separately, by the type of the field. It allows a package to get its own config without knowing about the whole one:

```go
c := &Config{}
if err := config.Register(c); err != nil {
    panic(err)
}

// HTTP is a type of the field in the registered Config
http := &HTTP{}
if err := config.Parse(http); err != nil {
    panic(err)
}
```

Only the fields of the sub config are parsed, with the same tags and keys as in the whole config: `http.host` in the file, `HTTP_HOST` in the environment.

If several registered configs have a field with this type, the field of one of them is used, so keep the types of sub configs unique.
