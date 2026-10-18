# Why tinyconf?

Go has many config libraries: big key-value stores, struct-tag loaders, environment-only parsers. tinyconf occupies a specific niche: **one config struct, filled by a chain of drivers, that tells you where every value came from and lets every package subscribe to its own part**. This page lays out where it fits, what it gives up, and how it compares to the closest alternatives.

## The 30-second pitch

- **The config is your struct.** Every field says where its value lives: `initial:"8080" yaml:"port" env:"HTTP_PORT" flag:"http-port"`. No key-value store in between, no `Unmarshal` step.
- **Drivers are a chain with priorities:** defaults from tags, files, remote stores, environment, flags — the next driver overrides the previous one, field by field.
- **Every value has a history.** [`Sources`](features/sources.md) tells which driver and which key gave the final value; the debug [log](features/logging.md#history-of-a-field) shows every override and every change after reloads. Secrets are masked everywhere.
- **Packages own their parts.** A package parses or [subscribes](features/reloading.md) to its sub config by type — `tinyconf.Subscribe(ctx, config, func(prev, next *HTTP) {...})` — and is called only when its part changes.
- **Documentation from the same structs:** a YAML template, a `.env` file, flags help, `consul kv put` / `vault kv put` / `etcdctl put` commands — in the format of every driver.
- **Remote stores without heavy dependencies:** Consul, Vault and etcd talk to their HTTP APIs directly; changes in Consul and etcd arrive right away, files are watched by the events of the file system.

## When to pick tinyconf

- Your service has several sources of config — defaults, a file, the environment, flags, a remote store — and you want one explicit priority order.
- You want to answer "why is this value here?" from the logs, without guessing.
- Your packages are independent: each one knows its own config struct, not the whole config.
- You want config changes applied without restart, with the old config never modified under your readers.
- You want the template of the config generated, not maintained by hand.

## When **not** to pick tinyconf

- You only read environment variables into a struct — [caarlos0/env](https://github.com/caarlos0/env) or [sethvargo/go-envconfig](https://github.com/sethvargo/go-envconfig) have no dependencies at all.
- You work with config as dynamic keys rather than a struct, or need many remote providers (S3, AWS Parameter Store and AppConfig, Azure Key Vault, NATS) out of the box — pick [koanf](https://github.com/knadh/koanf).
- You are deep in the cobra / pflag ecosystem — [viper](https://github.com/spf13/viper) integrates with them directly.
- You need Go older than 1.19.

## Feature matrix

Checked against the current documentation of the libraries on 2026-10-16.

|  | tinyconf | [viper](https://github.com/spf13/viper) | [koanf](https://github.com/knadh/koanf) | [cleanenv](https://github.com/ilyakaznacheev/cleanenv) | [aconfig](https://github.com/cristalhq/aconfig) | [confita](https://github.com/heetch/confita) | [caarlos0/env](https://github.com/caarlos0/env) |
|---|---|---|---|---|---|---|---|
| **Model** | struct + tags | key-value + `Unmarshal` | key-value + `Unmarshal` | struct + tags | struct + tags | struct + tags | struct + tags |
| **Defaults in tags** | ✓ (tag driver) | ✗ (`SetDefault`) | ✗ (provider) | ✓ | ✓ | ✗ (pre-filled struct) | ✓ |
| **File formats** | YAML, JSON, TOML, INI, properties + [any by a decoder](drivers/format.md) | JSON, TOML, YAML, INI, envfile, properties + codecs | many, each a module | YAML, JSON, TOML, EDN, .env | JSON + modules | JSON, YAML, TOML | ✗ |
| **Environment / flags** | ✓ / ✓ | ✓ / pflag | ✓ / modules | ✓ / ✗ | ✓ / ✓ | ✓ / ✓ | ✓ / ✗ |
| **Remote stores** | Consul, Vault, etcd | Consul, etcd, Firestore, NATS (separate module) | Consul, etcd, Vault, S3, AWS, Azure, NATS… (modules) | ✗ | ✗ | Consul, etcd, Vault, SSM (in core) | ✗ |
| **Required fields** | ✓ | ✗ | ✗ | ✓ | ✓ | ✓ | ✓ |
| **Reloading** | files by events, Consul and etcd by notifications, Vault by polling | files by events, remote | some providers | manual, env only | ✗ | ✗ | ✗ |
| **Change subscriptions per part** | ✓ | ✗ (one global callback) | ✗ (one callback per provider) | ✗ | ✗ | ✗ | ✗ |
| **Old config untouched on reload** | ✓ (new version) | ✗ (in place) | ✓ (fresh instance by hand) | ✗ (in place) | n/a | n/a | n/a |
| **Where the value came from** | ✓ driver and key | ✗ | ✗ | ✗ | ✗ | ✗ | partial (default or not) |
| **Masking secrets** | ✓ logs and docs | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| **Generated docs** | every driver | write config back | write config back | env usage | flags usage | flags usage | external tool |
| **Sub configs** | by type | by key | by key | by type | by type | by type | by type |
| **Concurrent use** | ✓ | ✗ | ✗ during load | n/a | n/a | n/a | n/a |

## Strengths

- **Provenance is built in.** No other library from the table reports which source gave the final value of every field. With several drivers it is the difference between reading the logs and guessing. See [Value sources](features/sources.md) and [Logging](features/logging.md).
- **Reloading that is safe to read.** A reload builds a new version from the defaults, the old one is never modified, so readers need no locks. Subscribers are called only for their part and only with a config without errors; a broken file keeps the last good config. See [Reloading](features/reloading.md).
- **The same types in every driver.** `time.Duration`, `url.URL`, `TextUnmarshaler`, slices, maps, structs in lists and pointers behave the same in a file, an environment variable, a flag and a remote store — the conformance tests check it for the tag, yaml, json, toml, env and flag drivers, the other drivers share their conversion.
- **Remote stores without the weight.** Consul, Vault and etcd drivers use their HTTP APIs: no SDKs, no gRPC. They handle the real-world cases — ACL that hides keys, expired tokens, compacted history, restarts of the store, failover between etcd endpoints — and are tested against real servers in Docker in CI.
- **Errors you can act on.** `Parse` collects all errors of all fields at once; `ErrAccessDenied` separates permission problems from network ones; `required:"true"` fails fast at the start.
- **100% test coverage**, race detector and the minimal supported Go version in CI.

## Weaknesses

- **Fewer remote stores than koanf.** Consul, Vault and etcd only; others need a [custom driver](drivers/custom.md).
- **Vault: token authentication only.** Kubernetes, AppRole and cloud login are delegated to Vault Agent; changes are detected by polling, because Vault does not notify.
- **No HCL out of the box.** The [format driver](drivers/format.md) accepts any decoder, but the conversion of HCL to a map is up to you.
- **Own flag parser.** It ignores unknown flags, so it lives next to cobra or any CLI framework, but it is not a pflag integration.
- **Subscriptions are package functions,** `tinyconf.Subscribe(ctx, config, ...)`, to keep Go 1.19 support; methods with type parameters need Go 1.27.
- **zap is a dependency of the core** because of the bundled logger adapter, even if you use another logger.
- **A younger and smaller project** than viper and koanf.

## Migrating

| From | To tinyconf |
|---|---|
| viper `SetDefault("http.port", 8080)` | `initial:"8080"` with the [tag driver](drivers/tag.md) |
| viper `AutomaticEnv()` | [`env.WithAutoKeys()`](drivers/env.md#auto-keys) |
| viper `WatchConfig()` + `OnConfigChange` | [`tinyconf.Subscribe`](features/reloading.md) per package |
| viper `Sub("http")` | `config.Parse(context.Background(), &HTTP{})` or `tinyconf.Current[HTTP](config)` |
| cleanenv `env-default`, `env-required` | `initial:"..."` and `required:"true"` |
| cleanenv `env-description` + `Usage` | `doc:"..."` + [`GenDoc("env")`](features/gendoc.md) |
| caarlos0/env `envDefault`, `required` | `initial:"..."` and `required:"true"` |
