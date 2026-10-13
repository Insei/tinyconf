# Struct tags

Everything about a field is described by its struct tags: where the value comes from and how it is documented.

```go
type Config struct {
    Token string `yaml:"token" env:"TOKEN" initial:"dev" doc:"API token" hidden:"true"`
    Port  int    `yaml:"port" env:"PORT" initial:"8080" doc:"port to listen"`
}
```

## Driver tags

Every driver reads its own tag, the value of the tag is the name of the value in the source of the driver.

| Driver | Tag | Value |
|---|---|---|
| [tag](../drivers/tag.md) | any, set in `tag.New` | the default value itself |
| [yaml](../drivers/yaml.md) | `yaml` | key in the file |
| [env](../drivers/env.md) | `env` | name of the environment variable |

A field without the tag of the driver is skipped by this driver.

## Common tags

| Tag | Description |
|---|---|
| `doc:"text"` | description of the field for [`GenDoc`](gendoc.md) |
| `hidden:"true"` | the value is masked in logs and is not shown in `GenDoc`, use it for secrets |

## Types of the fields

Fields can be strings, bools, integers, floats and pointers to these types. Fields with struct type are parsed by their fields, see [Nested structs and sub configs](structs.md).
