# Custom formats

The format driver reads config files of any format with nested objects. The format is set by a function that decodes the content to a map, everything else is the same as in the [yaml](yaml.md), [json](json.md) and [toml](toml.md) drivers: several files, merging, embedded files, [remote documents](consul.md#document), environment variables in the content, [reloading](../features/reloading.md), [documentation](../features/gendoc.md).

```go
import "github.com/insei/tinyconf/drivers/format"

driver, err := format.New("hujson", decodeHuJSON, "config.hujson")
```

The name of the driver is the name of the struct tag with the keys, and the name for [`GenDoc`](../features/gendoc.md):

```go
type Config struct {
    HTTP struct {
        Port int `hujson:"port"`
    } `hujson:"http"`
}
```

## Decoder

```go
type Decoder func(data []byte) (map[string]any, error)
```

The decoder returns the content as a map:

* keys of the map are the keys of the config, nested maps with string keys are nested objects;
* slices are lists, other values are scalars: strings, numbers, booleans, `time.Time`;
* empty content can be decoded to a nil map.

Values are converted to the types of the fields by the same rules as the values of yaml and json, see [Supported types](../features/types.md).

For example, [HuJSON](https://github.com/tailscale/hujson) — JSON with comments and trailing commas:

```go
import "github.com/tailscale/hujson"

func decodeHuJSON(data []byte) (map[string]any, error) {
    data, err := hujson.Standardize(data)
    if err != nil {
        return nil, err
    }
    var doc map[string]any
    return doc, json.Unmarshal(data, &doc)
}
```

Libraries that decode to the structs only, like HCL, need the conversion of their result to the map.

## Options

| Option | Description |
|---|---|
| `WithFiles(files...)` | additional files from the OS file system |
| `WithFS(fsys, files...)` | files from any `fs.FS`, `embed.FS` for example |
| `WithData(name, data)` | in-memory content, the name is used in error messages |
| `WithSource(source)` | content from a remote storage: [`consul.Document`](consul.md#document), [`etcd.Document`](etcd.md#document) |
| `WithOptional()` | files can be absent |
| `WithEnvExpansion()` | replaces `${VAR}` in the content with environment variables |
| `WithWatchInterval(interval)` | interval of polling the files for [reloading](../features/reloading.md), 5 seconds by default |
| `WithEncoder(encode)` | encoder of the documents for `GenDoc` |

The rules of the files are the same as in the [yaml](yaml.md#several-files) driver.

## Documentation

`GenDoc` needs the encoder:

```go
type Encoder func(doc map[string]any) ([]byte, error)

driver, err := format.New("hujson", decodeHuJSON, "config.hujson", format.WithEncoder(func(doc map[string]any) ([]byte, error) {
    return json.MarshalIndent(doc, "", "  ")
}))
```

The document has the same structure as the result of the decoder, with the current values of the registered configs; values of `hidden:"true"` fields are empty strings. Without the encoder `GenDoc` returns an empty string.

## Not a file format?

If the values are not stored as documents — a database, an API, a secret manager — write a [custom driver](custom.md).
