# toml

The toml driver reads values from TOML files.

```go
import "github.com/insei/tinyconf/drivers/toml"

tomlDriver, err := toml.New("config.toml")
```

## Keys

The `toml` tag is a key in the file. Tags of [nested structs](../features/structs.md) are names of tables:

```go
type Server struct {
    Host string `toml:"host"`
    Port int    `toml:"port"`
}

type Config struct {
    Name string `toml:"name"`
    HTTP struct {
        Host    string        `toml:"host"`
        Timeout time.Duration `toml:"timeout"`
    } `toml:"http"`
    Hosts   []string          `toml:"hosts"`
    Labels  map[string]string `toml:"labels"`
    Servers []Server          `toml:"servers"`
}
```

```toml
name = "app"
hosts = ["a", "b"]

[http]
host = "example.com"
timeout = "30s"

[labels]
env = "prod"

[[servers]]
host = "a"
port = 1

[[servers]]
host = "b"
port = 2
```

A field is read only if it and all its parents have the `toml` tag. Native TOML dates are supported for `time.Time` fields.

## Options

The options are the same as in the [yaml driver](yaml.md#options):

```go
tomlDriver, err := toml.New("config.toml",
    toml.WithFiles("config.local.toml"),
    toml.WithOptional(),
    toml.WithEnvExpansion(),
)
```

| Option | Description |
|---|---|
| `WithFiles(files...)` | additional files from the OS file system |
| `WithFS(fsys, files...)` | files from any `fs.FS`, `embed.FS` for example |
| `WithData(name, data)` | in-memory content, the name is used in error messages |
| `WithOptional()` | files can be absent |
| `WithEnvExpansion()` | replaces `${VAR}` and `${VAR:-default}` in the content with environment variables |

Several files are [merged](yaml.md#several-files) in the order of addition, tables are merged by keys. Files are read once per `Parse` call.

## Documentation

`GenDoc("toml")` generates a TOML document with all fields and their current values:

```toml
hosts = ["a", "b"]
name = "app"

[http]
  host = "example.com"
  timeout = "30s"

[labels]
  env = "prod"
```

* The `doc` tags are not used.
* Values of the fields with `hidden:"true"` are replaced with empty strings.
* TOML has no null, so nil pointers and other fields without values are skipped.
