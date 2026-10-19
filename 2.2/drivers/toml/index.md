# toml

The toml driver reads values from TOML files.

```go
import "github.com/insei/tinyconf/v2/drivers/toml"

tomlDriver, err := toml.New("config.toml")
```

## Keys

The `toml` tag is a key in the file. Tags of [nested structs](https://insei.github.io/tinyconf/2.2/features/structs/index.md) are names of tables:

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

Set the `toml` tag on all parents of the field: without the tag of the parent the field is read from the root of the file, and the manager [logs a warning](https://insei.github.io/tinyconf/2.2/features/logging/index.md). [Embedded structs](https://insei.github.io/tinyconf/2.2/features/structs/#embedded-structs) without the tag are the exception, their fields are on the level of the embedding struct. Native TOML dates are supported for `time.Time` fields.

## Options

The options are the same as in the [yaml driver](https://insei.github.io/tinyconf/2.2/drivers/yaml/#options):

```go
tomlDriver, err := toml.New("config.toml",
    toml.WithFiles("config.local.toml"),
    toml.WithOptional(),
    toml.WithEnvExpansion(),
)
```

| Option                        | Description                                                                                                                                                                                                         |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `WithFiles(files...)`         | additional files from the OS file system                                                                                                                                                                            |
| `WithFS(fsys, files...)`      | files from any `fs.FS`, `embed.FS` for example                                                                                                                                                                      |
| `WithData(name, data)`        | in-memory content, the name is used in error messages                                                                                                                                                               |
| `WithSource(source)`          | content from a remote storage: [`consul.Document`](https://insei.github.io/tinyconf/2.2/drivers/consul/#document)                                                                                                   |
| `WithOptional()`              | files can be absent                                                                                                                                                                                                 |
| `WithEnvExpansion()`          | replaces `${VAR}` and `${VAR:-default}` in the content with environment variables                                                                                                                                   |
| `WithWatchInterval(interval)` | interval of polling the files for [reloading](https://insei.github.io/tinyconf/2.2/features/reloading/index.md), 5 seconds by default; changes are detected by the events of the file system without waiting for it |

Several files are [merged](https://insei.github.io/tinyconf/2.2/drivers/yaml/#several-files) in the order of addition, tables are merged by keys. Files are read once per `Parse` call.

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

- The `doc` tags are not used.
- Values of the fields with `hidden:"true"` are replaced with empty strings.
- TOML has no null, so nil pointers and other fields without values are skipped.
