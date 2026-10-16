# Use cases

Recipes for typical tasks. Every recipe is a small, complete piece: take the one that matches your application and combine it with others.

| I want to… | Recipe |
|---|---|
| keep defaults next to the fields | [Defaults in code](#defaults-in-code) |
| configure a service only by environment | [Environment only](#environment-only) |
| have a config file and override it by environment | [File with environment overrides](#file-with-environment-overrides) |
| have different files for dev, stage and prod | [Files per environment](#files-per-environment) |
| ship defaults inside the binary | [Embedded defaults](#embedded-defaults) |
| keep secrets out of the files and logs | [Secrets](#secrets) |
| run in Docker or Kubernetes | [Containers](#containers) |
| build a command line tool | [Command line tools](#command-line-tools) |
| set lists and maps from environment | [Lists and maps in environment](#lists-and-maps-in-environment) |
| have a section that can be absent | [Optional sections](#optional-sections) |
| give every package its own config | [Config per package](#config-per-package) |
| stop at start if something is missing | [Fail fast](#fail-fast) |
| apply changes without restart | [Reload without restart](#reload-without-restart) |
| generate an example of the config | [Config template](#config-template) |
| find out where a value came from | [Debugging](#debugging) |
| test the code that uses the config | [Tests](#tests) |
| read values from my own storage | [Custom source](#custom-source) |

All recipes use this helper to keep the examples short:

```go
func must[T any](value T, err error) T {
    if err != nil {
        panic(err)
    }
    return value
}
```

## Defaults in code

Default values live in the tags, right next to the fields. The [tag driver](drivers/tag.md) goes first, so every other driver can override it.

```go
type Config struct {
    Host    string        `initial:"localhost"`
    Port    int           `initial:"8080"`
    Timeout time.Duration `initial:"30s"`
    Hosts   []string      `initial:"a,b"`
}

config := must(tinyconf.New(
    tinyconf.WithDriver(must(tag.New("initial"))),
))
```

A value that is computed at runtime can be set in the struct before `Parse`: the field keeps it, if no driver has a value.

```go
c := &Config{Host: must(os.Hostname())}
```

## Environment only

A twelve-factor service without config files. With the [auto keys](drivers/env.md#auto-keys) the `env` tags are not needed, with the [prefix](drivers/env.md#prefix) the variables of the service do not clash with others.

```go
type Config struct {
    HTTP struct {
        Host     string `initial:"0.0.0.0"`
        Port     int    `initial:"8080"`
        MaxConns int    `initial:"100"`
    }
    DatabaseURL string `required:"true" hidden:"true"`
}

config := must(tinyconf.New(
    tinyconf.WithDriver(must(tag.New("initial"))),
    tinyconf.WithDriver(must(env.New(env.WithPrefix("APP_"), env.WithAutoKeys()))),
))
```

```bash
APP_HTTP_PORT=9090
APP_HTTP_MAX_CONNS=500
APP_DATABASE_URL=postgres://db/app
```

## File with environment overrides

The most common setup: defaults, then a file, then the environment on top.

```go
type Config struct {
    HTTP struct {
        Host string `initial:"localhost" yaml:"host" env:"HTTP_HOST"`
        Port int    `initial:"8080" yaml:"port" env:"HTTP_PORT"`
    } `yaml:"http"`
    LogLevel string `initial:"info" yaml:"log_level" env:"LOG_LEVEL"`
}

config := must(tinyconf.New(
    tinyconf.WithDriver(must(tag.New("initial"))),
    tinyconf.WithDriver(must(yaml.New("config.yaml", yaml.WithOptional()))),
    tinyconf.WithDriver(must(env.New())),
))
```

`yaml.WithOptional()` lets the application start without the file at all. Prefer JSON or TOML? The [json](drivers/json.md) and [toml](drivers/toml.md) drivers have the same options, only the tag is different.

## Files per environment

One base file and a small file with differences for every environment. Files are [merged](drivers/yaml.md#several-files): the next file overrides only the keys it has.

```go
stage := os.Getenv("APP_ENV") // dev, stage, prod

yamlDriver := must(yaml.New("config.yaml",
    yaml.WithFiles(
        "config."+stage+".yaml",
        "config.local.yaml", // personal overrides, not in the repository
    ),
    yaml.WithOptional(),
))
```

=== "config.yaml"

    ```yaml
    http:
      host: 0.0.0.0
      port: 8080
    log_level: info
    ```

=== "config.prod.yaml"

    ```yaml
    log_level: warn
    ```

=== "config.local.yaml"

    ```yaml
    http:
      port: 9090
    log_level: debug
    ```

## Embedded defaults

Defaults as a readable file that is compiled into the binary, with an optional external file on top. The binary works without any files around it.

```go
//go:embed defaults.yaml
var defaults embed.FS

yamlDriver := must(yaml.New("",
    yaml.WithFS(defaults, "defaults.yaml"),
    yaml.WithFiles("/etc/app/config.yaml"),
    yaml.WithOptional(),
))
```

## Secrets

Three things protect a secret: it is not printed, it is not stored in the file, and the application does not start without it.

```go
type Config struct {
    Database struct {
        Host     string `yaml:"host" env:"DB_HOST"`
        Password string `yaml:"password" env:"DB_PASSWORD" hidden:"true" required:"true"`
    } `yaml:"database"`
}
```

* `hidden:"true"` masks the value in [logs](features/logging.md) and removes it from [`GenDoc`](features/gendoc.md).
* `required:"true"` makes `Parse` [fail](features/required.md) without the value.

Where the value comes from:

=== "Environment variable"

    ```bash
    DB_PASSWORD=secret
    ```

=== "Secret file"

    ```go
    env.New(env.WithSecretFiles())
    ```

    ```bash
    # the value is read from the file
    DB_PASSWORD_FILE=/run/secrets/db_password
    ```

=== "Reference in the config file"

    ```go
    yaml.New("config.yaml", yaml.WithEnvExpansion())
    ```

    ```yaml
    database:
      host: db.internal
      password: ${DB_PASSWORD}
    ```

## Containers

A typical Kubernetes deployment: the config file is mounted from a ConfigMap, secrets are mounted as files, a few values are overridden by variables of the pod.

```go
config := must(tinyconf.New(
    tinyconf.WithDriver(must(tag.New("initial"))),
    tinyconf.WithDriver(must(yaml.New("/etc/app/config.yaml",
        yaml.WithOptional(),
        yaml.WithWatchInterval(10*time.Second),
    ))),
    tinyconf.WithDriver(must(env.New(
        env.WithPrefix("APP_"),
        env.WithSecretFiles(), // APP_DB_PASSWORD_FILE=/run/secrets/db_password
        env.WithIndexedKeys(), // APP_HOSTS_0, APP_HOSTS_1
        env.WithFiles(),       // no .env files in the container
    ))),
))
```

```yaml
env:
  - name: APP_DB_PASSWORD_FILE
    value: /run/secrets/db_password
  - name: APP_HOSTS_0
    value: a.example.com
  - name: APP_HOSTS_1
    value: b.example.com
```

A ConfigMap volume is updated by Kubernetes without restart of the pod, so [`Watch`](#reload-without-restart) picks up the changes of the file.

## Command line tools

Flags have the highest priority, the file and the environment provide the rest.

```go
type Config struct {
    Output  string   `flag:"output" env:"TOOL_OUTPUT" initial:"out" doc:"output directory"`
    Include []string `flag:"include" doc:"patterns of files to include"`
    Verbose bool     `flag:"verbose" doc:"print debug messages"`
    Help    bool     `flag:"help" doc:"show this help"`
}

config := must(tinyconf.New(
    tinyconf.WithDriver(must(tag.New("initial"))),
    tinyconf.WithDriver(must(env.New())),
    tinyconf.WithDriver(must(flag.New())),
))

c := &Config{}
if err := config.Register(c); err != nil {
    panic(err)
}
if err := config.Parse(c); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(2)
}
if c.Help {
    fmt.Print(config.GenDoc("flag"))
    return
}
```

```bash
tool --output=build --include='*.go' --include='*.md' --verbose
```

The driver [ignores unknown flags](drivers/flag.md#unknown-flags), so it works next to any CLI framework that owns the sub commands and positional arguments.

## Lists and maps in environment

Three forms, from the shortest to the most flexible. All of them fill the same fields:

```go
type Server struct {
    Host string `json:"host" env:"HOST"`
    Port int    `json:"port" env:"PORT"`
}

type Config struct {
    Hosts   []string          `env:"HOSTS"`
    Labels  map[string]string `env:"LABELS"`
    Servers []Server          `env:"SERVERS"`
}
```

=== "Separators"

    ```bash
    HOSTS=a.example.com,b.example.com
    LABELS=env:prod,team:core
    ```

=== "JSON"

    ```bash
    HOSTS='["a.example.com","b.example.com"]'
    LABELS='{"env":"prod","team":"core"}'
    SERVERS='[{"host":"a","port":1},{"host":"b","port":2}]'
    ```

=== "Indexed keys"

    ```bash
    HOSTS_0=a.example.com
    HOSTS_1=b.example.com
    LABELS_env=prod
    LABELS_team=core
    SERVERS_0_HOST=a
    SERVERS_0_PORT=1
    SERVERS_1_HOST=b
    SERVERS_1_PORT=2
    ```

The indexed form requires [`env.WithIndexedKeys()`](drivers/env.md#indexed-keys). A value with the separator inside can be quoted, `"a,b",c`, or the separator can be changed with the [`separator` tag](features/tags.md#common-tags).

## Optional sections

A feature that is turned on by the presence of its settings. The section is a [struct stored by pointer](features/structs.md#structs-stored-by-pointer): it stays `nil` until some driver has a value for one of its fields.

```go
type TLS struct {
    Cert string `yaml:"cert" env:"TLS_CERT"`
    Key  string `yaml:"key" env:"TLS_KEY"`
}

type Metrics struct {
    Addr string `yaml:"addr" env:"METRICS_ADDR"`
}

type Config struct {
    TLS     *TLS     `yaml:"tls"`
    Metrics *Metrics `yaml:"metrics"`
}
```

```go
if c.Metrics != nil {
    go serveMetrics(c.Metrics.Addr)
}
if c.TLS != nil {
    // TLS is enabled, but it can be set partially: check the fields
    if c.TLS.Cert == "" || c.TLS.Key == "" {
        return errors.New("both tls.cert and tls.key should be set")
    }
}
```

Do not use `required:"true"` inside an optional section: a required field is checked even when the section is `nil`, so it makes the whole section mandatory.

## Config per package

Every package declares the struct it needs and does not know about the whole config. The application registers the root config once and passes the manager around.

```go
// package storage
type Config struct {
    DSN      string `yaml:"dsn" env:"STORAGE_DSN" required:"true" hidden:"true"`
    PoolSize int    `yaml:"pool_size" env:"STORAGE_POOL_SIZE" initial:"10"`
}

func New(config *tinyconf.Manager) (*Storage, error) {
    c := &Config{}
    if err := config.Parse(c); err != nil {
        return nil, err
    }
    return connect(c)
}
```

```go
// package main
type Config struct {
    HTTP    server.Config  `yaml:"http"`
    Storage storage.Config `yaml:"storage"`
}

root := &Config{}
if err := config.Register(root); err != nil {
    panic(err)
}
store := must(storage.New(config)) // parses only the storage section
```

The package gets a [sub config](features/structs.md#sub-configs): its own fields with the keys of the whole config, `storage.dsn` in the file.

## Fail fast

Check everything at start and report all problems in one message.

```go
if err := config.Parse(c); err != nil {
    if errors.Is(err, tinyconf.ErrRequired) {
        fmt.Fprintln(os.Stderr, "some required settings are not set")
    }
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
}
```

```text
env driver: field HTTP.Port: failed to parse ... "abc": invalid syntax; field Database.Password: required value is not set
```

`Parse` does not stop on the first error: incorrect values of all fields and all missing [required fields](features/required.md) are in the same error.

## Reload without restart

The current config is stored in an atomic pointer, [`Watch`](features/reloading.md) replaces it after every change of the files.

```go
var current atomic.Pointer[Config]

c := &Config{}
if err := config.Register(c); err != nil {
    panic(err)
}
if err := config.Parse(c); err != nil {
    panic(err)
}
current.Store(c)

go func() {
    err := config.Watch(ctx, c, func(conf any, err error) {
        if err != nil {
            log.Println("config is not reloaded:", err)
            return
        }
        fresh := conf.(*Config)
        if old := current.Swap(fresh); old.LogLevel != fresh.LogLevel {
            setLogLevel(fresh.LogLevel)
        }
    })
    if err != nil {
        log.Println("config watching is stopped:", err)
    }
}()
```

Readers call `current.Load()` and always see a consistent config: the old one is never modified.

## Config template

A flag that prints the example of the config and exits. The template is always up to date, because it is generated from the same structs.

```go
c := &Config{}
if err := config.Register(c); err != nil {
    panic(err)
}
if len(os.Args) > 1 && os.Args[1] == "-doc" {
    // fills the defaults and the values of this environment,
    // missing required values are not a problem for the template
    _ = config.Parse(c)
    fmt.Println(config.GenDoc("yaml"))
    fmt.Println(config.GenDoc("env"))
    return
}
```

`GenDoc` shows the current values of the registered config, so after `Parse` the template has the defaults and everything that is set in this environment — it is also a "show effective config" command. Without `Parse` the template has empty values. Fields with `hidden:"true"` are never shown. See [Documentation generation](features/gendoc.md).

## Debugging

"Why is the port 9090?" Two tools answer it.

[`Sources`](features/sources.md) shows the driver that has set the final value:

```go
for _, source := range config.Sources(c) {
    log.Printf("%s is set by %s driver from %s", source.Field, source.Driver, source.Source)
}
```

```text
HTTP.Host is set by env driver from HTTP_HOST
HTTP.Port is set by yaml driver from http.port
```

The [logger](features/logging.md) on the debug level shows the [history](features/logging.md#history-of-a-field) of every field: the initial value and every driver that has overridden it, then every change after reloads.

```go
tinyconf.WithLogger(logger.NewFmtLogger(logger.DEBUG))
```

## Tests

Tests should not depend on the files and environment of the machine. Every driver can be fed from memory:

```go
func newTestConfig(t *testing.T, yamlDoc string, args ...string) *Config {
    config := must(tinyconf.New(
        tinyconf.WithDriver(must(tag.New("initial"))),
        tinyconf.WithDriver(must(yaml.New("", yaml.WithData("test", []byte(yamlDoc))))),
        tinyconf.WithDriver(must(env.New(env.WithFiles()))), // no .env files
        tinyconf.WithDriver(must(flag.New(flag.WithArgs(args)))),
    ))
    c := &Config{}
    if err := config.Register(c); err != nil {
        t.Fatal(err)
    }
    if err := config.Parse(c); err != nil {
        t.Fatal(err)
    }
    return c
}

func TestPort(t *testing.T) {
    t.Setenv("HTTP_PORT", "7070")
    c := newTestConfig(t, "http:\n  host: test\n")
    // c.HTTP.Host == "test", c.HTTP.Port == 7070
}
```

To test with a `.env` file without changing the environment of the test process, use `env.WithFiles(path)` together with [`env.WithFilesIsolation()`](drivers/env.md#isolation).

## Custom source

Values from Vault, Consul, a database or any other storage: write a [custom driver](drivers/custom.md). `Load` reads the storage once per `Parse`, `GetValue` converts the values with the `convert` package, so the driver supports the same types as the bundled ones.

```go
type kvDriver struct {
    client *kv.Client
    values map[string]string
}

func (d *kvDriver) GetName() string                       { return "kv" }
func (d *kvDriver) GenDoc(...*tinyconf.Registered) string { return "" }

func (d *kvDriver) Load() (err error) {
    d.values, err = d.client.FetchAll()
    return err
}

func (d *kvDriver) GetValue(field fmap.Field) (*tinyconf.Value, error) {
    key, ok := field.GetTag().Lookup("kv")
    if !ok {
        return nil, tinyconf.ErrIncorrectTagSettings
    }
    raw, ok := d.values[key]
    if !ok {
        return nil, tinyconf.ErrValueNotFound
    }
    value, err := convert.StringToField(raw, field)
    if err != nil {
        return nil, err
    }
    return &tinyconf.Value{Source: key, Value: value}, nil
}
```

Add the `Watch` method, and the driver takes part in [reloading](features/reloading.md).
