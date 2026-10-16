# Drivers

A driver is a source of values. Every driver reads its own struct tag and knows how to find the value for the field in its source.

| Driver | Package | Struct tag | Source |
|---|---|---|---|
| [tag](tag.md) | `drivers/tag` | any, `initial` for example | default values from struct tags |
| [yaml](yaml.md) | `drivers/yaml` | `yaml` | YAML files |
| [json](json.md) | `drivers/json` | `json` | JSON files |
| [toml](toml.md) | `drivers/toml` | `toml` | TOML files |
| [ini](ini.md) | `drivers/ini` | `ini` | INI files |
| [properties](properties.md) | `drivers/properties` | `properties` | `.properties` files of Java and Spring |
| [env](env.md) | `drivers/env` | `env` | environment variables and `.env` files |
| [flag](flag.md) | `drivers/flag` | `flag` | command line flags |
| [consul](consul.md) | `drivers/consul` | `consul` | Consul KV: a key per field, or a document for yaml, json and toml |
| [vault](vault.md) | `drivers/vault` | `vault` | secrets from the KV secrets engine of HashiCorp Vault |
| [etcd](etcd.md) | `drivers/etcd` | `etcd` | etcd: a key per field, or a document for yaml, json and toml |

Drivers are added to the [manager](../features/manager.md) with `tinyconf.WithDriver`, the next driver overrides values of the previous one. All drivers support the same [set of types](../features/types.md).

Need another file format? See [Custom formats](format.md): a decoder function is enough. Another source? See [Custom drivers](custom.md).
