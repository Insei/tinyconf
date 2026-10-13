# yaml

The yaml driver reads values from YAML files.

```go
import "github.com/insei/tinyconf/drivers/yaml"

yamlDriver, err := yaml.New("config.yaml")
```

## Keys

The `yaml` tag is a key in the file. Tags of [nested structs](../features/structs.md) are keys of nested maps:

```go
type Config struct {
    Name string `yaml:"name"`
    HTTP struct {
        Host string `yaml:"host"`
        Port int    `yaml:"port"`
    } `yaml:"http"`
}
```

```yaml
name: app
http:
  host: example.com
  port: 9090
```

Options after the comma are ignored: `yaml:"port,omitempty"` is the `port` key.

!!! warning "Set the tags of the parents"
    The path of the nested field is built from the `yaml` tags of its parents. If the `HTTP` field has no `yaml` tag, the `Host` field is read from the `host` key in the root of the file, not from `http.host`.

    When a value is found this way, the manager [logs a warning](../features/logging.md) with the name of the parent without the tag and the key that was used.

## Options

```go
yamlDriver, err := yaml.New("config.yaml",
    yaml.WithFiles("config.local.yaml"),
    yaml.WithOptional(),
    yaml.WithEnvExpansion(),
)
```

| Option | Description |
|---|---|
| `WithFiles(files...)` | additional files from the OS file system |
| `WithFS(fsys, files...)` | files from any `fs.FS`, `embed.FS` for example |
| `WithData(name, data)` | in-memory content, the name is used in error messages |
| `WithOptional()` | files can be absent |
| `WithEnvExpansion()` | replaces `${VAR}` in the content with environment variables |
| `WithWatchInterval(interval)` | interval of checking the files for changes in [`Watch`](../features/reloading.md), 5 seconds by default |

The first argument of `New` can be an empty string, if all sources are set by options.

## Several files

Sources are merged in the order of addition: the file from `New` first, then the sources from options in the order of the options. Values from the next source override values from the previous one, nested maps are merged by keys, all other values — lists too — are replaced.

=== "config.yaml"

    ```yaml
    http:
      host: example.com
      port: 80
    hosts: [a, b]
    ```

=== "config.local.yaml"

    ```yaml
    http:
      port: 9090
    hosts: [c]
    ```

=== "Result"

    ```yaml
    http:
      host: example.com
      port: 9090
    hosts: [c]
    ```

It is a base for the layered configs: defaults embedded into the binary, the main file, the local overrides.

```go
//go:embed defaults.yaml
var defaults embed.FS

yamlDriver, err := yaml.New("",
    yaml.WithFS(defaults, "defaults.yaml"),
    yaml.WithFiles("/etc/app/config.yaml", "config.local.yaml"),
    yaml.WithOptional(),
)
```

## Missing files and values

A key that is absent in the files is not an error, the field keeps its value from the previous driver.

A file that can not be read is an error of `Parse`, and the driver is skipped in this `Parse` call. With `WithOptional()` absent files are skipped silently, all other errors — a directory instead of the file, incorrect YAML — are still returned.

## Environment variables in files

With `WithEnvExpansion()` the content of the files is processed before decoding:

| Syntax | Result |
|---|---|
| `${VAR}` | value of the variable, empty string if it is not set |
| `${VAR:-default}` | value of the variable, `default` if it is not set or empty |
| `$$` | the `$` sign |

```yaml
database:
  host: ${DB_HOST:-localhost}
  password: ${DB_PASSWORD}
```

## Reading

Files are read once per `Parse` call, not for every field. The next `Parse` call rereads them, so the changed file is picked up without recreation of the driver.

## Documentation

`GenDoc("yaml")` generates a commented template of the file, see [Documentation generation](../features/gendoc.md).
