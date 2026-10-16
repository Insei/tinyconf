[![codecov](https://codecov.io/gh/Insei/tinyconf/graph/badge.svg?token=TSOJ1SWF9D)](https://codecov.io/gh/Insei/tinyconf)
[![build](https://github.com/insei/tinyconf/actions/workflows/go.yml/badge.svg)](https://github.com/Insei/tinyconf/actions/workflows/go.yml)
[![Goreport](https://goreportcard.com/badge/github.com/insei/tinyconf)](https://goreportcard.com/report/github.com/insei/tinyconf)
[![GoDoc](https://godoc.org/github.com/insei/tinyconf?status.svg)](https://godoc.org/github.com/insei/tinyconf)
# tinyconf
tinyconf - a simple and universal library for parsing configs.

Documentation: [insei.github.io/tinyconf](https://insei.github.io/tinyconf/) — get started, [why tinyconf](https://insei.github.io/tinyconf/why-tinyconf/), [use cases](https://insei.github.io/tinyconf/use-cases/), features and drivers.

# Installation
Install via go get. Note that Go 1.19 or newer is required.
```sh
go get github.com/insei/tinyconf@latest
```

# Description
tinyconf fills config structs with values from drivers. When initializing `tinyconf.Manager`, you can add a logger to it,
as well as drivers. Drivers are applied in the order of addition, the next driver overrides values of the previous one,
so the last driver has the highest priority. You can also add your own driver.

`tinyconf.Manager` has methods:

| Method | Description |
|---|---|
| `Register(conf any) error` | registers the config, `conf` should be a pointer to struct |
| `Parse(conf any) error` | fills the registered config, or its field with struct type (sub config), with values from drivers |
| `ParseContext(ctx, conf any) error` | `Parse` with the context for the requests of the drivers to the remote storages |
| `GenDoc(driverName string) string` | generates documentation of all registered configs in the driver format |
| `Sources(conf any) []FieldSource` | returns which driver has set the value of every field in the last `Parse` call |
| `tinyconf.Subscribe(ctx, m, onChange) error` | calls `onChange(prev, next *T)` after the changes of the config or the sub config `T` |
| `tinyconf.Current[T](m) *T` | returns the current version of the config or the sub config `T` |
| `ReloadStatus() ReloadStatus` | state of watching and reloading for the health checks: the last error, stopped drivers |

`Parse` parses all fields even if some of them have errors, all errors are returned as one error that can be checked
with `errors.Is` and `errors.As`.

# Drivers

| Driver | Package | Struct tag | Source |
|---|---|---|---|
| tag | `drivers/tag` | any, `initial` for example | default values from struct tags |
| yaml | `drivers/yaml` | `yaml` | YAML files |
| json | `drivers/json` | `json` | JSON files |
| toml | `drivers/toml` | `toml` | TOML files |
| ini | `drivers/ini` | `ini` | INI files |
| properties | `drivers/properties` | `properties` | `.properties` files of Java and Spring |
| env | `drivers/env` | `env` | environment variables and `.env` files |
| flag | `drivers/flag` | `flag` | command line flags |
| consul | `drivers/consul` | `consul` | Consul KV: a key per field, or a document for yaml, json and toml |
| vault | `drivers/vault` | `vault` | secrets from the KV secrets engine of HashiCorp Vault |
| etcd | `drivers/etcd` | `etcd` | etcd: a key per field, or a document for yaml, json and toml |

Common struct tags:

| Tag | Description |
|---|---|
| `doc:"text"` | field description for `GenDoc` |
| `merge:"true"` | slices and maps from several drivers are merged instead of replaced |
| `hidden:"true"` | value is masked in logs and is not shown in `GenDoc`; `tinyconf.Secret` type masks the value in `fmt`, JSON, zap and slog of the application too |
| `required:"true"` | `Parse` returns `tinyconf.ErrRequired` if the field has no value from drivers and has zero value in the config |
| `separator:";"` | separator of slice, array and map items in string values, `,` by default |
| `kvseparator:"="` | separator of map key and value in string values, `:` by default |

## yaml, json, toml, ini and properties
```go
yamlDriver, err := yaml.New("config.yaml",
	yaml.WithFiles("config.local.yaml"), // additional files, the next file overrides the previous, nested maps are merged
	yaml.WithOptional(),                 // files can be absent, by default absent file is an error
	yaml.WithEnvExpansion(),             // replace ${VAR} and ${VAR:-default} with environment variables
	yaml.WithFS(embedFS, "defaults.yaml"),
	yaml.WithData("inline", []byte("name: app")),
	yaml.WithWatchInterval(time.Second), // files are watched by the events of the file system and by polling, 5 seconds by default
)
```
The json, toml, ini and properties drivers have the same options. Files are read once per `Parse` call, so the next `Parse` call rereads them.

## env
```go
envDriver, err := env.New(
	env.WithPrefix("APP_"),             // env:"HTTP_PORT" is read from APP_HTTP_PORT
	env.WithAutoKeys(),                 // fields without env tag: HTTP.MaxConns -> HTTP_MAX_CONNS
	env.WithSecretFiles(),              // read value from the file with path from HTTP_TOKEN_FILE
	env.WithFiles(".env", ".env.local"), // custom .env files instead of the default ones
	env.WithFilesIsolation(),           // do not export values from .env files to the process environment
	env.WithIndexedKeys(),              // slices, maps and structs from separate variables, see below
)
```
With `env.WithIndexedKeys()` slices, arrays, maps and structs can be set by separate variables,
if the variable with the whole value is not set:

| Type | Variables |
|---|---|
| `Hosts []string` with `env:"HOSTS"` | `HOSTS_0=a`, `HOSTS_1=b` |
| `Labels map[string]string` with `env:"LABELS"` | `LABELS_env=prod`, the rest of the name is a key, as is |
| `Servers []Server` with `env:"SERVERS"` | `SERVERS_0_HOST=a`, `SERVERS_0_MAX_CONNS=10`, `SERVERS_1_HOST=b` |

Names of the struct fields are taken from their `env` tags or built from the field names (`MaxConns` -> `MAX_CONNS`).
For maps with simple values all variables with the name of the field as a prefix are used as map items,
so the name of such field should not be a prefix of other variables.

By default, the driver reads `.env` files from the executable directory and from the working directory
and exports their values to the process environment. Variables of the process environment have priority over `.env` files.

## flag
```go
flagDriver, err := flag.New() // os.Args[1:] by default, flag.WithArgs(args) for custom arguments
```
Supported forms: `-name=value`, `--name=value`, `-name value`, `--name value`, `-name` and `--name` for bool fields.
Slices and arrays can be filled with repeated flags. The driver does not know all flags of the application,
so unknown flags are ignored.

## consul
```go
consulDriver, err := consul.New("app") // consul:"port" inside consul:"http" is read from app/http/port
yamlDriver, err := yaml.New("config.yaml",
	yaml.WithSource(consul.Document("app/config")), // YAML document from Consul KV, merged with the file
)
```
The address and the token are taken from `CONSUL_HTTP_ADDR` and `CONSUL_HTTP_TOKEN` or set by options.
Changes in Consul are reported for reloading right after they happen, by blocking queries.

## vault
```go
vaultDriver, err := vault.New("app/config", // vault:"password" inside vault:"database" is read from database.password
	vault.WithPaths("app/database"),          // secrets are merged in the order of the paths
	vault.WithTokenFile("/vault/secrets/token"), // the token renewed by Vault Agent
)
```
The address and the token are taken from `VAULT_ADDR` and `VAULT_TOKEN` or set by options, KV version 2 at `secret/` by default.
The secrets are checked for changes every 30 seconds while there are subscriptions.

## etcd
```go
etcdDriver, err := etcd.New("app", // etcd:"port" inside etcd:"http" is read from app/http/port
	etcd.WithEndpoints("etcd-1:2379", "etcd-2:2379", "etcd-3:2379"), // the next endpoint is used if the current one is not available
	etcd.WithAuth("app", password),
)
yamlDriver, err := yaml.New("config.yaml",
	yaml.WithSource(etcd.Document("app/config")), // YAML document from etcd, merged with the file
)
```
The endpoints and the user are taken from `ETCDCTL_ENDPOINTS`, `ETCDCTL_USER` and `ETCDCTL_PASSWORD` or set by options.
Changes in etcd are reported for reloading right after they happen, by the stream of the events.

# Supported types
All drivers support the same set of types, it is checked by the conformance tests in `drivers/conformance_test.go`.

| Types | String value (env, flag, tag) | yaml, json and toml |
|---|---|---|
| `string`, `bool`, integers, floats and named types based on them | `value` | scalar |
| `time.Duration` | `1m30s` | `1m30s` |
| `time.Time` | `2024-01-02T03:04:05Z`, `2024-01-02 03:04:05`, `2024-01-02` | the same or yaml timestamp |
| `url.URL`, `[]byte`, any `encoding.TextUnmarshaler` (`net.IP`, `uuid.UUID`, ...) | `value` | scalar |
| `[]T`, `[N]T` | `a,b,c`, items can be quoted: `"a,b",c` | list or string |
| `map[K]V` | `a:1,b:2` | map or string |
| `[]Struct`, `map[K]Struct`, nested slices and maps | JSON: `[{"host":"a","port":1}]` | native |
| pointers to all types above | the same | the same |
| `Struct`, `*Struct` | parsed by fields | parsed by fields |

Fields with struct type are parsed by their fields, every field of the nested struct has its own tags.
It works the same way for structs stored by value and by pointer:
```go
type TLS struct {
	Cert string `env:"TLS_CERT" yaml:"cert"`
	Key  string `env:"TLS_KEY" yaml:"key"`
}

type Config struct {
	TLS *TLS `yaml:"tls"` // nil if there are no values for its fields
}
```
The struct stored by pointer is created when the first value for its fields is found, otherwise the pointer is not changed.
Pointers to pointers (`**Struct`) and recursive types (`Next *Node` inside `Node`) are parsed as a single value.
Fields of embedded structs without the tag are on the level of the embedding struct, like in `encoding/json`.

String values of slices, arrays, maps and structs are parsed as JSON if they start with `[` or `{`.
Keys of the structs nested into slices and maps are taken from the `json` tag for string values and JSON files
and from the `yaml` tag for YAML files, field name is used if the tag is not set (case-insensitive).
If the value of a slice or map is provided by several drivers, the last driver replaces the whole value;
with the `merge:"true"` tag slices are appended and maps get the keys of the next drivers.

# Example

```go
package main

import (
	"fmt"
	"time"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/env"
	"github.com/insei/tinyconf/drivers/flag"
	"github.com/insei/tinyconf/drivers/tag"
	"github.com/insei/tinyconf/drivers/yaml"
	"github.com/insei/tinyconf/logger"
)

type HTTP struct {
	Host    string        `initial:"localhost" yaml:"host" env:"HTTP_HOST" flag:"http-host" doc:"http host"`
	Port    int           `initial:"8080" yaml:"port" env:"HTTP_PORT" flag:"http-port" doc:"http port"`
	Timeout time.Duration `initial:"30s" yaml:"timeout" env:"HTTP_TIMEOUT" doc:"http timeout"`
}

type Config struct {
	HTTP     HTTP              `yaml:"http"`
	Hosts    []string          `initial:"a,b" yaml:"hosts" env:"HOSTS" flag:"host" doc:"allowed hosts"`
	Labels   map[string]string `yaml:"labels" env:"LABELS" doc:"labels"`
	Password string            `yaml:"password" env:"PASSWORD" hidden:"true" required:"true"`
}

func main() {
	tagDriver, err := tag.New("initial")
	if err != nil {
		panic(err)
	}
	yamlDriver, err := yaml.New("config.yaml", yaml.WithOptional())
	if err != nil {
		panic(err)
	}
	envDriver, err := env.New()
	if err != nil {
		panic(err)
	}
	flagDriver, err := flag.New()
	if err != nil {
		panic(err)
	}
	config, err := tinyconf.New(
		tinyconf.WithLogger(logger.NewFmtLogger(logger.INFO)),
		tinyconf.WithDriver(tagDriver),
		tinyconf.WithDriver(yamlDriver),
		tinyconf.WithDriver(envDriver),
		tinyconf.WithDriver(flagDriver),
	)
	if err != nil {
		panic(err)
	}

	c := &Config{}
	if err = config.Register(c); err != nil {
		panic(err)
	}
	fmt.Println(config.GenDoc("yaml"))
	if err = config.Parse(c); err != nil {
		panic(err)
	}
	for _, source := range config.Sources(c) {
		fmt.Printf("%s is set by %s driver from %s\n", source.Field, source.Driver, source.Source)
	}

	// configs can be parsed partially, by the fields of the registered configs
	http := &HTTP{}
	if err = config.Parse(http); err != nil {
		panic(err)
	}
	fmt.Println(*http)
}
```

# Reloading
After every change of the config sources the manager parses the configs with subscribers again.
A package subscribes to its own part of the config and is called only when this part is changed:
```go
err := tinyconf.Subscribe(ctx, config, func(prev, next *HTTP) {
	server.SetTimeout(next.Timeout)
})

timeout := tinyconf.Current[HTTP](config).Timeout // the current version, it must not be changed
```
Watching starts with the first subscription and stops when the contexts of all subscriptions are done.
Changes are reported by yaml, json, toml, ini and properties drivers for files from the OS file system, by the consul and etcd drivers and their documents, and by the vault driver.

# Custom formats
Config files of any format with nested objects are read by the format driver with the decoder of the format,
with the same features as yaml, json and toml: several files, merging, reloading, documentation.
```go
driver, err := format.New("hujson", func(data []byte) (map[string]any, error) {
	var doc map[string]any
	data, err := hujson.Standardize(data) // JSON with comments
	if err != nil {
		return nil, err
	}
	return doc, json.Unmarshal(data, &doc)
}, "config.hujson") // hujson:"port" inside hujson:"http" is read from http.port
```

# Custom drivers
The driver should implement the `tinyconf.Driver` interface:
```go
type Driver interface {
	GenDoc(...*Registered) string
	GetName() string
	GetValue(field fmap.Field) (*Value, error)
}
```
`GetValue` should return `tinyconf.ErrValueNotFound` if the source has no value for the field
and `tinyconf.ErrIncorrectTagSettings` if the field has no settings for the driver, all other errors are returned from `Parse`.
To support the same types as the built-in drivers, use the `convert` package: `convert.StringToField` for string values
and `convert.TreeToField` for values decoded from formats like yaml or json.

If the driver implements the optional `tinyconf.Loader` or `tinyconf.ContextLoader` interface, `Load` (or `LoadContext` with the context of `ParseContext`) is called once at the start of every `Parse` call,
so the driver can read its source once and not for every field.
If the driver implements the optional `tinyconf.Watcher` interface, it can report changes of its source for reloading.

More examples: the service in the [`example`](example) directory, runnable examples on [pkg.go.dev](https://pkg.go.dev/github.com/insei/tinyconf#pkg-examples), recipes in the [documentation](https://insei.github.io/tinyconf/use-cases/).
