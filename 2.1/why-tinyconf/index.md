# Why tinyconf?

**tinyconf is for Go services built from independent packages, whose config comes from several sources and changes without restart.** Every package declares its own config struct, gets its values and its updates, and knows nothing about the config of the whole application. Where a value came from is always known.

## Who it is for

- **Modular services.** An HTTP server, a database client, a queue consumer, a cache — each package has its own config struct and works with it alone. Adding a package does not touch a central config file of the code.
- **Services that change config at runtime.** A ConfigMap in Kubernetes, keys in Consul or etcd, a rotated secret in Vault: the change reaches only the packages whose part has changed, without restart and without locks in the code that reads the config.
- **Services with many sources.** Defaults, a file, the environment, flags and a remote store at the same time, with one explicit priority order — and an answer to "why is this value here?" when something goes wrong in production.
- **Teams that do not want to maintain config templates by hand.** `.env` examples, YAML templates, flags help and the commands for Consul, Vault and etcd are generated from the same structs.

## What you get

### Every package owns its part of the config

The application registers the whole config once. A package parses, reads and subscribes to its own struct type — no string keys, no `Unmarshal` of a subtree, no knowledge of where its part lives in the whole config.

```go
// in the http package
tinyconf.Current[HTTP](config).Port // the current version: an atomic read, no allocations

tinyconf.Subscribe(ctx, config, func(prev, next *HTTP) {
    server.Restart(next) // called only when HTTP has changed
})
```

Other libraries give one callback for any change of the whole config (viper), or a callback per provider (koanf), and leave it to you to find out what has changed and who should know. See [Sub configs](https://insei.github.io/tinyconf/2.1/features/structs/#sub-configs) and [Reloading](https://insei.github.io/tinyconf/2.1/features/reloading/index.md).

### Reloads that are safe by design

A reload builds a new version of the config from the defaults. The version that your code holds is never modified, so readers need no locks and never see a half-applied config. A broken file or an unavailable store keeps the last good config; subscribers are called only with a config without errors, and only for their changed part. `ReloadStatus()` reports the last error and the drivers that stopped watching, for health checks. Files are watched by the events of the file system, Consul and etcd notify about changes, Vault is polled.

### One struct, one chain of drivers

The config is your struct, every field says where its value lives:

```go
Port int `initial:"8080" yaml:"port" env:"HTTP_PORT" flag:"http-port" required:"true" doc:"port of the server"`
```

Drivers are applied in the order of addition, the next one overrides the previous one field by field. Eleven drivers out of the box — defaults from tags, YAML, JSON, TOML, INI, properties, environment, flags, Consul, Vault, etcd — and [any file format](https://insei.github.io/tinyconf/2.1/drivers/format/index.md) by a decoder. `time.Duration`, `url.URL`, `TextUnmarshaler`, slices, maps, structs in lists and pointers behave the same in every driver: a conformance test checks it.

### You always know where a value came from

`Sources(conf)` returns the driver and the key of the final value of every field. The debug log shows the initial value of every field, every override and every change after reloads. Secrets are masked everywhere: `hidden:"true"` in the logs and templates, the `tinyconf.Secret` type also in `fmt`, JSON and the logs of the application. See [Value sources](https://insei.github.io/tinyconf/2.1/features/sources/index.md) and [Logging](https://insei.github.io/tinyconf/2.1/features/logging/index.md).

### Templates of the config from the code

`GenDoc` renders the registered configs with the current values and the `doc` descriptions in the format of every driver: a YAML or TOML file, a `.env` file, flags help, `consul kv put`, `vault kv put` and `etcdctl put` commands. See [Documentation generation](https://insei.github.io/tinyconf/2.1/features/gendoc/index.md).

### Light, fast and tested

- No SDKs: Consul, Vault and etcd are used through their HTTP APIs, zap is a separate module. The core depends only on the YAML and TOML parsers, fsnotify and two small libraries of the author, fmap and cast.
- `Current` takes tens of nanoseconds without allocations, `Parse` of a typical config takes microseconds, so the config can be read on every request.
- 100% test coverage, race detector, fuzz tests of the parsers, end-to-end tests with real Consul, Vault and etcd, Go 1.19 and the latest Go in CI, a benchmark diff in every pull request.

### Ready for AI agents

The repository has a [skill for agents](https://insei.github.io/tinyconf/2.1/ai-agents/index.md) whose programs are built in CI, and the documentation is published as `llms.txt` and `llms-full.txt` for every version, so an agent writes the code for the current API, not for the one it remembers.

## When another library fits better

- You only read environment variables into a struct — [caarlos0/env](https://github.com/caarlos0/env) or [sethvargo/go-envconfig](https://github.com/sethvargo/go-envconfig) have no dependencies at all.
- You work with config as dynamic keys rather than a struct, or need many remote providers (S3, AWS Parameter Store and AppConfig, Azure Key Vault, NATS) out of the box — pick [koanf](https://github.com/knadh/koanf).
- You are deep in the cobra / pflag ecosystem — [viper](https://github.com/spf13/viper) integrates with them directly.
- You need Go older than 1.19.

## Feature matrix

Checked against the current documentation of the libraries on 2026-10-16.

|                                    | tinyconf                                                                                                             | [viper](https://github.com/spf13/viper)             | [koanf](https://github.com/knadh/koanf)              | [cleanenv](https://github.com/ilyakaznacheev/cleanenv) | [aconfig](https://github.com/cristalhq/aconfig) | [confita](https://github.com/heetch/confita) | [caarlos0/env](https://github.com/caarlos0/env) |
| ---------------------------------- | -------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------- | ---------------------------------------------------- | ------------------------------------------------------ | ----------------------------------------------- | -------------------------------------------- | ----------------------------------------------- |
| **Model**                          | struct + tags                                                                                                        | key-value + `Unmarshal`                             | key-value + `Unmarshal`                              | struct + tags                                          | struct + tags                                   | struct + tags                                | struct + tags                                   |
| **Defaults in tags**               | ✓ (tag driver)                                                                                                       | ✗ (`SetDefault`)                                    | ✗ (provider)                                         | ✓                                                      | ✓                                               | ✗ (pre-filled struct)                        | ✓                                               |
| **File formats**                   | YAML, JSON, TOML, INI, properties + [any by a decoder](https://insei.github.io/tinyconf/2.1/drivers/format/index.md) | JSON, TOML, YAML, INI, envfile, properties + codecs | many, each a module                                  | YAML, JSON, TOML, EDN, .env                            | JSON + modules                                  | JSON, YAML, TOML                             | ✗                                               |
| **Environment / flags**            | ✓ / ✓                                                                                                                | ✓ / pflag                                           | ✓ / modules                                          | ✓ / ✗                                                  | ✓ / ✓                                           | ✓ / ✓                                        | ✓ / ✗                                           |
| **Remote stores**                  | Consul, Vault, etcd                                                                                                  | Consul, etcd, Firestore, NATS (separate module)     | Consul, etcd, Vault, S3, AWS, Azure, NATS… (modules) | ✗                                                      | ✗                                               | Consul, etcd, Vault, SSM (in core)           | ✗                                               |
| **Required fields**                | ✓                                                                                                                    | ✗                                                   | ✗                                                    | ✓                                                      | ✓                                               | ✓                                            | ✓                                               |
| **Reloading**                      | files by events, Consul and etcd by notifications, Vault by polling                                                  | files by events, remote                             | some providers                                       | manual, env only                                       | ✗                                               | ✗                                            | ✗                                               |
| **Change subscriptions per part**  | ✓                                                                                                                    | ✗ (one global callback)                             | ✗ (one callback per provider)                        | ✗                                                      | ✗                                               | ✗                                            | ✗                                               |
| **Old config untouched on reload** | ✓ (new version)                                                                                                      | ✗ (in place)                                        | ✓ (fresh instance by hand)                           | ✗ (in place)                                           | n/a                                             | n/a                                          | n/a                                             |
| **Where the value came from**      | ✓ driver and key                                                                                                     | ✗                                                   | ✗                                                    | ✗                                                      | ✗                                               | ✗                                            | partial (default or not)                        |
| **Masking secrets**                | ✓ logs, docs, `Secret` type                                                                                          | ✗                                                   | ✗                                                    | ✗                                                      | ✗                                               | ✗                                            | ✗                                               |
| **Generated docs**                 | every driver                                                                                                         | write config back                                   | write config back                                    | env usage                                              | flags usage                                     | flags usage                                  | external tool                                   |
| **Sub configs**                    | by type                                                                                                              | by key                                              | by key                                               | by type                                                | by type                                         | by type                                      | by type                                         |
| **Concurrent use**                 | ✓                                                                                                                    | ✗                                                   | ✗ during load                                        | n/a                                                    | n/a                                             | n/a                                          | n/a                                             |

## Known limitations

- **Fewer remote stores than koanf.** Consul, Vault and etcd only; others need a [custom driver](https://insei.github.io/tinyconf/2.1/drivers/custom/index.md).
- **Vault: token authentication only.** Kubernetes, AppRole and cloud login are delegated to Vault Agent; changes are detected by polling, because Vault does not notify.
- **No HCL out of the box.** The [format driver](https://insei.github.io/tinyconf/2.1/drivers/format/index.md) accepts any decoder, but the conversion of HCL to a map is up to you.
- **Own flag parser.** It ignores unknown flags, so it lives next to cobra or any CLI framework, but it is not a pflag integration.
- **Subscriptions are package functions,** `tinyconf.Subscribe(ctx, config, ...)`, to keep Go 1.19 support; methods with type parameters need Go 1.27.
- **A younger and smaller project** than viper and koanf.

## Migrating

From tinyconf v1: see [Upgrading to v2](https://insei.github.io/tinyconf/2.1/upgrading/index.md).

| From                                     | To tinyconf                                                                                          |
| ---------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| viper `SetDefault("http.port", 8080)`    | `initial:"8080"` with the [tag driver](https://insei.github.io/tinyconf/2.1/drivers/tag/index.md)    |
| viper `AutomaticEnv()`                   | [`env.WithAutoKeys()`](https://insei.github.io/tinyconf/2.1/drivers/env/#auto-keys)                  |
| viper `WatchConfig()` + `OnConfigChange` | [`tinyconf.Subscribe`](https://insei.github.io/tinyconf/2.1/features/reloading/index.md) per package |
| viper `Sub("http")`                      | `config.Parse(context.Background(), &HTTP{})` or `tinyconf.Current[HTTP](config)`                    |
| cleanenv `env-default`, `env-required`   | `initial:"..."` and `required:"true"`                                                                |
| cleanenv `env-description` + `Usage`     | `doc:"..."` + [`GenDoc("env")`](https://insei.github.io/tinyconf/2.1/features/gendoc/index.md)       |
| caarlos0/env `envDefault`, `required`    | `initial:"..."` and `required:"true"`                                                                |
