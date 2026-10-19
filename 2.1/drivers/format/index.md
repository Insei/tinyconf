# Custom formats

The format driver reads config files of any format with nested objects. The format is set by a function that decodes the content to a map, everything else is the same as in the [yaml](https://insei.github.io/tinyconf/2.1/drivers/yaml/index.md), [json](https://insei.github.io/tinyconf/2.1/drivers/json/index.md) and [toml](https://insei.github.io/tinyconf/2.1/drivers/toml/index.md) drivers: several files, merging, embedded files, [remote documents](https://insei.github.io/tinyconf/2.1/drivers/consul/#document), environment variables in the content, [reloading](https://insei.github.io/tinyconf/2.1/features/reloading/index.md), [documentation](https://insei.github.io/tinyconf/2.1/features/gendoc/index.md).

```go
import "github.com/insei/tinyconf/v2/drivers/format"

driver, err := format.New("hujson", decodeHuJSON, "config.hujson")
```

The name of the driver is the name of the struct tag with the keys, and the name for [`GenDoc`](https://insei.github.io/tinyconf/2.1/features/gendoc/index.md):

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

- keys of the map are the keys of the config, nested maps with string keys are nested objects;
- slices are lists, other values are scalars: strings, numbers, booleans, `time.Time`;
- empty content can be decoded to a nil map.

Values are converted to the types of the fields by the same rules as the values of yaml and json, see [Supported types](https://insei.github.io/tinyconf/2.1/features/types/index.md).

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

| Option                        | Description                                                                                                                                                                                       |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `WithFiles(files...)`         | additional files from the OS file system                                                                                                                                                          |
| `WithFS(fsys, files...)`      | files from any `fs.FS`, `embed.FS` for example                                                                                                                                                    |
| `WithData(name, data)`        | in-memory content, the name is used in error messages                                                                                                                                             |
| `WithSource(source)`          | content from a remote storage: [`consul.Document`](https://insei.github.io/tinyconf/2.1/drivers/consul/#document), [`etcd.Document`](https://insei.github.io/tinyconf/2.1/drivers/etcd/#document) |
| `WithOptional()`              | files can be absent                                                                                                                                                                               |
| `WithEnvExpansion()`          | replaces `${VAR}` in the content with environment variables                                                                                                                                       |
| `WithWatchInterval(interval)` | interval of polling the files for [reloading](https://insei.github.io/tinyconf/2.1/features/reloading/index.md), 5 seconds by default                                                             |
| `WithEncoder(encode)`         | encoder of the documents for `GenDoc`                                                                                                                                                             |

The rules of the files are the same as in the [yaml](https://insei.github.io/tinyconf/2.1/drivers/yaml/#several-files) driver.

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

If the values are not stored as documents — a database, an API, a secret manager — write a [custom driver](https://insei.github.io/tinyconf/2.1/drivers/custom/index.md).
