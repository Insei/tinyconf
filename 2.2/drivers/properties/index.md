# properties

The properties driver reads values from `.properties` files — the format of Java and Spring, `application.properties`.

```go
import "github.com/insei/tinyconf/v2/drivers/properties"

propertiesDriver, err := properties.New("application.properties")
```

The driver has its own parser and adds no dependencies.

## Keys

The `properties` tag is a part of the key. Tags of [nested structs](https://insei.github.io/tinyconf/2.2/features/structs/index.md) are the parts before the dots:

```go
type Config struct {
    Name string `properties:"name"`
    HTTP struct {
        Host string `properties:"host"`
        Port int    `properties:"port"`
    } `properties:"http"`
}
```

```properties
name=app
http.host=example.com
http.port=9090
```

Set the tags of the parents

The key of the nested field is built from the `properties` tags of its parents. If the `HTTP` field has no `properties` tag, the `Host` field is read from the `host` key, and the manager [logs a warning](https://insei.github.io/tinyconf/2.2/features/logging/index.md). [Embedded structs](https://insei.github.io/tinyconf/2.2/features/structs/#embedded-structs) without the tag are the exception.

## Syntax

The syntax of [`java.util.Properties`](https://docs.oracle.com/javase/8/docs/api/java/util/Properties.html#load-java.io.Reader-):

| Element                       | Example                                                                                 |
| ----------------------------- | --------------------------------------------------------------------------------------- |
| key and value                 | `port=9090`, `port: 9090`, `port 9090`                                                  |
| comment                       | `# comment`, `! comment`                                                                |
| escapes                       | `\t`, `\n`, `\r`, `\f`, `\\`, `\uXXXX`; `\` before any other character is the character |
| separators and spaces in keys | `key\ with\=separators=value`                                                           |
| long value on several lines   | a backslash at the end of the line, the leading spaces of the next line are skipped     |

The spaces before the value are skipped, the spaces at the end are a part of the value, like in Java.

Values are strings, they are parsed by the rules of [string values](https://insei.github.io/tinyconf/2.2/features/types/index.md) like environment variables: `true`, `9090`, `1m30s`; lists are comma separated `hosts=a,b`, maps are `labels=env:prod,team:core` or the keys `labels.env=prod`, structs in lists are JSON with the keys by the `properties` tags: `servers=[{"host": "a", "port": 1}]`.

A key can not be a value and an object at the same time: `http=1` and `http.port=2` in one file is an error of `Parse` with the number of the line.

## Options

The options are the same as of the [yaml](https://insei.github.io/tinyconf/2.2/drivers/yaml/#options) driver:

| Option                        | Description                                                                                                                                                                                                         |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `WithFiles(files...)`         | additional files from the OS file system: `application-prod.properties`                                                                                                                                             |
| `WithFS(fsys, files...)`      | files from any `fs.FS`, `embed.FS` for example                                                                                                                                                                      |
| `WithData(name, data)`        | in-memory content, the name is used in error messages                                                                                                                                                               |
| `WithSource(source)`          | content from a remote storage: [`consul.Document`](https://insei.github.io/tinyconf/2.2/drivers/consul/#document), [`etcd.Document`](https://insei.github.io/tinyconf/2.2/drivers/etcd/#document)                   |
| `WithOptional()`              | files can be absent                                                                                                                                                                                                 |
| `WithEnvExpansion()`          | replaces `${VAR}` in the content with environment variables                                                                                                                                                         |
| `WithWatchInterval(interval)` | interval of polling the files for [reloading](https://insei.github.io/tinyconf/2.2/features/reloading/index.md), 5 seconds by default; changes are detected by the events of the file system without waiting for it |

Files are [merged](https://insei.github.io/tinyconf/2.2/drivers/yaml/#several-files) in the order of addition, like the profiles of Spring:

```go
propertiesDriver, err := properties.New("application.properties",
    properties.WithFiles("application-"+profile+".properties"),
    properties.WithOptional(),
)
```

## Documentation

`GenDoc("properties")` generates the file with the current values, sorted by the keys. Values of `hidden:"true"` fields are empty, the special characters are escaped, so the file is read back to the same config.

```properties
http.host=example.com
http.port=9090
name=app
```
