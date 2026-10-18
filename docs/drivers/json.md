# json

The json driver reads values from JSON files.

```go
import "github.com/insei/tinyconf/v2/drivers/json"

jsonDriver, err := json.New("config.json")
```

## Keys

The `json` tag is a key in the file. Tags of [nested structs](../features/structs.md) are keys of nested objects:

```go
type Config struct {
    Name string `json:"name"`
    HTTP struct {
        Host string `json:"host"`
        Port int    `json:"port"`
    } `json:"http"`
    Hosts []string `json:"hosts"`
}
```

```json
{
  "name": "app",
  "http": {
    "host": "example.com",
    "port": 9090
  },
  "hosts": ["a", "b"]
}
```

Options after the comma are ignored: `json:"port,omitempty"` is the `port` key. Set the `json` tag on all parents of the field: without the tag of the parent the field is read from the root of the file, and the manager [logs a warning](../features/logging.md). [Embedded structs](../features/structs.md#embedded-structs) without the tag are the exception, their fields are on the level of the embedding struct.

## Options

The options are the same as in the [yaml driver](yaml.md#options):

```go
jsonDriver, err := json.New("config.json",
    json.WithFiles("config.local.json"),
    json.WithOptional(),
    json.WithEnvExpansion(),
)
```

| Option | Description |
|---|---|
| `WithFiles(files...)` | additional files from the OS file system |
| `WithFS(fsys, files...)` | files from any `fs.FS`, `embed.FS` for example |
| `WithData(name, data)` | in-memory content, the name is used in error messages |
| `WithSource(source)` | content from a remote storage: [`consul.Document`](consul.md#document) |
| `WithOptional()` | files can be absent |
| `WithEnvExpansion()` | replaces `${VAR}` and `${VAR:-default}` in the content with environment variables |
| `WithWatchInterval(interval)` | interval of polling the files for [reloading](../features/reloading.md), 5 seconds by default; changes are detected by the events of the file system without waiting for it |

Several files are [merged](yaml.md#several-files) in the order of addition, objects are merged by keys. Files are read once per `Parse` call.

## Documentation

`GenDoc("json")` generates a JSON document with all fields and their current values:

```json
{
  "hosts": [
    "a",
    "b"
  ],
  "http": {
    "host": "example.com",
    "port": 9090
  },
  "name": "app"
}
```

JSON has no comments, so the `doc` tags are not used. Values of the fields with `hidden:"true"` are replaced with empty strings.
