[![codecov](https://codecov.io/gh/Insei/tinyconf/graph/badge.svg?token=TSOJ1SWF9D)](https://codecov.io/gh/Insei/tinyconf)
[![build](https://github.com/insei/tinyconf/actions/workflows/go.yml/badge.svg)](https://github.com/Insei/tinyconf/actions/workflows/go.yml)
[![Goreport](https://goreportcard.com/badge/github.com/insei/tinyconf)](https://goreportcard.com/report/github.com/insei/tinyconf)
[![GoDoc](https://godoc.org/github.com/insei/tinyconf?status.svg)](https://godoc.org/github.com/insei/tinyconf)
# tinyconf
tinyconf - a simple and universal library for parsing configs.

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
| `GenDoc(driverName string) string` | generates documentation of all registered configs in the driver format |
| `Sources(conf any) []FieldSource` | returns which driver has set the value of every field in the last `Parse` call |

`Parse` parses all fields even if some of them have errors, all errors are returned as one error that can be checked
with `errors.Is` and `errors.As`.

# Drivers

| Driver | Package | Struct tag | Source |
|---|---|---|---|
| tag | `drivers/tag` | any, `initial` for example | default values from struct tags |
| yaml | `drivers/yaml` | `yaml` | YAML files |
| json | `drivers/json` | `json` | JSON files |
| env | `drivers/env` | `env` | environment variables and `.env` files |
| flag | `drivers/flag` | `flag` | command line flags |

Common struct tags:

| Tag | Description |
|---|---|
| `doc:"text"` | field description for `GenDoc` |
| `hidden:"true"` | value is masked in logs and is not shown in `GenDoc` |
| `required:"true"` | `Parse` returns `tinyconf.ErrRequired` if the field has no value from drivers and has zero value in the config |
| `separator:";"` | separator of slice, array and map items in string values, `,` by default |
| `kvseparator:"="` | separator of map key and value in string values, `:` by default |

## yaml and json
```go
yamlDriver, err := yaml.New("config.yaml",
	yaml.WithFiles("config.local.yaml"), // additional files, the next file overrides the previous, nested maps are merged
	yaml.WithOptional(),                 // files can be absent, by default absent file is an error
	yaml.WithEnvExpansion(),             // replace ${VAR} and ${VAR:-default} with environment variables
	yaml.WithFS(embedFS, "defaults.yaml"),
	yaml.WithData("inline", []byte("name: app")),
)
```
The json driver has the same options. Files are read once per `Parse` call, so the next `Parse` call rereads them.

## env
```go
envDriver, err := env.New(
	env.WithPrefix("APP_"),             // env:"HTTP_PORT" is read from APP_HTTP_PORT
	env.WithAutoKeys(),                 // fields without env tag: HTTP.MaxConns -> HTTP_MAX_CONNS
	env.WithSecretFiles(),              // read value from the file with path from HTTP_TOKEN_FILE
	env.WithFiles(".env", ".env.local"), // custom .env files instead of the default ones
	env.WithFilesIsolation(),           // do not export values from .env files to the process environment
)
```
By default, the driver reads `.env` files from the executable directory and from the working directory
and exports their values to the process environment. Variables of the process environment have priority over `.env` files.

## flag
```go
flagDriver, err := flag.New() // os.Args[1:] by default, flag.WithArgs(args) for custom arguments
```
Supported forms: `-name=value`, `--name=value`, `-name value`, `--name value`, `-name` and `--name` for bool fields.
Slices and arrays can be filled with repeated flags. The driver does not know all flags of the application,
so unknown flags are ignored.

# Supported types
All drivers support the same set of types, it is checked by the conformance tests in `drivers/conformance_test.go`.

| Types | String value (env, flag, tag) | yaml and json |
|---|---|---|
| `string`, `bool`, integers, floats and named types based on them | `value` | scalar |
| `time.Duration` | `1m30s` | `1m30s` |
| `time.Time` | `2024-01-02T03:04:05Z`, `2024-01-02 03:04:05`, `2024-01-02` | the same or yaml timestamp |
| `url.URL`, `[]byte`, any `encoding.TextUnmarshaler` (`net.IP`, `uuid.UUID`, ...) | `value` | scalar |
| `[]T`, `[N]T` | `a,b,c`, items can be quoted: `"a,b",c` | list or string |
| `map[K]V` | `a:1,b:2` | map or string |
| structs, `[]Struct`, `map[K]Struct`, nested slices and maps | JSON: `[{"host":"a","port":1}]` | native |
| pointers to all types above | the same | the same |

String values of slices, arrays, maps and structs are parsed as JSON if they start with `[` or `{`.
Keys of the structs nested into slices and maps are taken from the `json` tag for string values and JSON files
and from the `yaml` tag for YAML files, field name is used if the tag is not set (case-insensitive).
If the value of a slice or map is provided by several drivers, the last driver replaces the whole value.

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

If the driver implements the optional `tinyconf.Loader` interface, `Load` is called once at the start of every `Parse` call,
so the driver can read its source once and not for every field.

More examples in `parser_test.go` and others test files.
