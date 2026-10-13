# Drivers

A driver is a source of values. Every driver reads its own struct tag and knows how to find the value for the field in its source.

| Driver | Package | Struct tag | Source |
|---|---|---|---|
| [tag](tag.md) | `drivers/tag` | any, `initial` for example | default values from struct tags |
| [yaml](yaml.md) | `drivers/yaml` | `yaml` | YAML files |
| [json](json.md) | `drivers/json` | `json` | JSON files |
| [env](env.md) | `drivers/env` | `env` | environment variables and `.env` files |
| [flag](flag.md) | `drivers/flag` | `flag` | command line flags |

Drivers are added to the [manager](../features/manager.md) with `tinyconf.WithDriver`, the next driver overrides values of the previous one. All drivers support the same [set of types](../features/types.md).

Need another source? See [Custom drivers](custom.md).
