# yaml

The yaml driver reads values from a YAML file.

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

## Missing values

A key that is absent in the file is not an error, the field keeps its value from the previous driver.

The file itself should exist: if it can not be read, `Parse` returns an error for every field with the `yaml` tag.

## Documentation

`GenDoc("yaml")` generates a commented template of the file, see [Documentation generation](../features/gendoc.md).
