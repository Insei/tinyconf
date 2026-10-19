# Drivers

A driver is a source of values. Every driver reads its own struct tag and knows how to find the value for the field in its source.

| Driver                                                                         | Package              | Struct tag                 | Source                                                            |
| ------------------------------------------------------------------------------ | -------------------- | -------------------------- | ----------------------------------------------------------------- |
| [tag](https://insei.github.io/tinyconf/2.2/drivers/tag/index.md)               | `drivers/tag`        | any, `initial` for example | default values from struct tags                                   |
| [yaml](https://insei.github.io/tinyconf/2.2/drivers/yaml/index.md)             | `drivers/yaml`       | `yaml`                     | YAML files                                                        |
| [json](https://insei.github.io/tinyconf/2.2/drivers/json/index.md)             | `drivers/json`       | `json`                     | JSON files                                                        |
| [toml](https://insei.github.io/tinyconf/2.2/drivers/toml/index.md)             | `drivers/toml`       | `toml`                     | TOML files                                                        |
| [ini](https://insei.github.io/tinyconf/2.2/drivers/ini/index.md)               | `drivers/ini`        | `ini`                      | INI files                                                         |
| [properties](https://insei.github.io/tinyconf/2.2/drivers/properties/index.md) | `drivers/properties` | `properties`               | `.properties` files of Java and Spring                            |
| [env](https://insei.github.io/tinyconf/2.2/drivers/env/index.md)               | `drivers/env`        | `env`                      | environment variables and `.env` files                            |
| [flag](https://insei.github.io/tinyconf/2.2/drivers/flag/index.md)             | `drivers/flag`       | `flag`                     | command line flags                                                |
| [consul](https://insei.github.io/tinyconf/2.2/drivers/consul/index.md)         | `drivers/consul`     | `consul`                   | Consul KV: a key per field, or a document for yaml, json and toml |
| [vault](https://insei.github.io/tinyconf/2.2/drivers/vault/index.md)           | `drivers/vault`      | `vault`                    | secrets from the KV secrets engine of HashiCorp Vault             |
| [etcd](https://insei.github.io/tinyconf/2.2/drivers/etcd/index.md)             | `drivers/etcd`       | `etcd`                     | etcd: a key per field, or a document for yaml, json and toml      |

Drivers are added to the [manager](https://insei.github.io/tinyconf/2.2/features/manager/index.md) with `tinyconf.WithDriver`, the next driver overrides values of the previous one. All drivers support the same [set of types](https://insei.github.io/tinyconf/2.2/features/types/index.md).

Need another file format? See [Custom formats](https://insei.github.io/tinyconf/2.2/drivers/format/index.md): a decoder function is enough. Another source? See [Custom drivers](https://insei.github.io/tinyconf/2.2/drivers/custom/index.md).
