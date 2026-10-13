# Struct tags

Everything about a field is described by its struct tags: where the value comes from, how it is documented and how it is parsed.

```go
type Config struct {
    Token string   `yaml:"token" env:"TOKEN" initial:"dev" doc:"API token" hidden:"true"`
    Hosts []string `yaml:"hosts" env:"HOSTS" separator:";"`
}
```

## Driver tags

Every driver reads its own tag, the value of the tag is the name of the value in the source of the driver.

| Driver | Tag | Value |
|---|---|---|
| [tag](../drivers/tag.md) | any, set in `tag.New` | the default value itself |
| [yaml](../drivers/yaml.md) | `yaml` | key in the file |
| [json](../drivers/json.md) | `json` | key in the file |
| [toml](../drivers/toml.md) | `toml` | key in the file |
| [env](../drivers/env.md) | `env` | name of the environment variable |
| [flag](../drivers/flag.md) | `flag` | name of the command line flag |

A field without the tag of the driver is skipped by this driver.

## Common tags

| Tag | Description |
|---|---|
| `doc:"text"` | description of the field for [`GenDoc`](gendoc.md) |
| `hidden:"true"` | the value is masked in logs and is not shown in `GenDoc`, use it for secrets |
| `required:"true"` | `Parse` returns an error if the field has no value, see [Required fields](required.md) |
| `separator:";"` | separator of slice, array and map items in string values, `,` by default |
| `kvseparator:"="` | separator of map key and value in string values, `:` by default |

See [Supported types](types.md) for the syntax of string values.
