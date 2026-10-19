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

| Driver                                                             | Tag                   | Value                            |
| ------------------------------------------------------------------ | --------------------- | -------------------------------- |
| [tag](https://insei.github.io/tinyconf/2.2/drivers/tag/index.md)   | any, set in `tag.New` | the default value itself         |
| [yaml](https://insei.github.io/tinyconf/2.2/drivers/yaml/index.md) | `yaml`                | key in the file                  |
| [json](https://insei.github.io/tinyconf/2.2/drivers/json/index.md) | `json`                | key in the file                  |
| [toml](https://insei.github.io/tinyconf/2.2/drivers/toml/index.md) | `toml`                | key in the file                  |
| [env](https://insei.github.io/tinyconf/2.2/drivers/env/index.md)   | `env`                 | name of the environment variable |
| [flag](https://insei.github.io/tinyconf/2.2/drivers/flag/index.md) | `flag`                | name of the command line flag    |

A field without the tag of the driver is skipped by this driver.

## Common tags

| Tag               | Description                                                                                                                                                              |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `doc:"text"`      | description of the field for [`GenDoc`](https://insei.github.io/tinyconf/2.2/features/gendoc/index.md)                                                                   |
| `merge:"true"`    | slices and maps from several drivers are merged instead of replaced, see [Several drivers](https://insei.github.io/tinyconf/2.2/features/types/#several-drivers)         |
| `hidden:"true"`   | the value is masked in logs and is not shown in `GenDoc`, use it for secrets; see also the [`Secret`](https://insei.github.io/tinyconf/2.2/features/types/#secrets) type |
| `required:"true"` | `Parse` returns an error if the field has no value, see [Required fields](https://insei.github.io/tinyconf/2.2/features/required/index.md)                               |
| `separator:";"`   | separator of slice, array and map items in string values, `,` by default                                                                                                 |
| `kvseparator:"="` | separator of map key and value in string values, `:` by default                                                                                                          |

See [Supported types](https://insei.github.io/tinyconf/2.2/features/types/index.md) for the syntax of string values.
