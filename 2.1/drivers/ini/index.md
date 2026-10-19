# ini

The ini driver reads values from INI files.

```go
import "github.com/insei/tinyconf/v2/drivers/ini"

iniDriver, err := ini.New("config.ini")
```

The driver has its own parser and adds no dependencies.

## Keys

The `ini` tag is a key in the file. Tags of [nested structs](https://insei.github.io/tinyconf/2.1/features/structs/index.md) are sections:

```go
type Config struct {
    Name string `ini:"name"`
    HTTP struct {
        Host string `ini:"host"`
        Port int    `ini:"port"`
        TLS  struct {
            Cert string `ini:"cert"`
        } `ini:"tls"`
    } `ini:"http"`
}
```

```ini
name = app

[http]
host = example.com
port = 9090

[http.tls]
cert = /etc/tls/cert.pem
```

Dots in the names of the sections and in the keys are nested objects: `[http.tls]` is the `tls` object inside `http`, and `http.port = 9090` in the root is the same as `port = 9090` in the `[http]` section.

Set the tags of the parents

The path of the nested field is built from the `ini` tags of its parents. If the `HTTP` field has no `ini` tag, the `Host` field is read from the `host` key in the root of the file, and the manager [logs a warning](https://insei.github.io/tinyconf/2.1/features/logging/index.md). [Embedded structs](https://insei.github.io/tinyconf/2.1/features/structs/#embedded-structs) without the tag are the exception.

## Syntax

| Element                              | Example                                |
| ------------------------------------ | -------------------------------------- |
| key and value                        | `port = 9090`, `port: 9090`            |
| section                              | `[http]`, `[http.tls]`                 |
| comment                              | `; comment`, `# comment`               |
| comment after the value              | `port = 9090 ; comment`, after a space |
| value in double quotes, with escapes | `name = "my \"app\"\n"`                |
| value in single quotes, as is        | `name = 'my app; # not a comment'`     |
| empty value                          | `name =`                               |

Values of INI are strings, they are parsed by the rules of [string values](https://insei.github.io/tinyconf/2.1/features/types/index.md) like environment variables: `true`, `9090`, `1m30s`; lists are comma separated `hosts = a,b`, maps are `labels = env:prod,team:core`, structs in lists are JSON with the keys by the `ini` tags: `servers = '[{"host": "a", "port": 1}]'`. A map can be a section too:

```ini
[labels]
env = prod
team = core
```

A key can not be a value and a section at the same time: `http = 1` and `[http]` in one file is an error of `Parse` with the number of the line.

## Options

The options are the same as of the [yaml](https://insei.github.io/tinyconf/2.1/drivers/yaml/#options) driver:

| Option                        | Description                                                                                                                                                                                                         |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `WithFiles(files...)`         | additional files from the OS file system                                                                                                                                                                            |
| `WithFS(fsys, files...)`      | files from any `fs.FS`, `embed.FS` for example                                                                                                                                                                      |
| `WithData(name, data)`        | in-memory content, the name is used in error messages                                                                                                                                                               |
| `WithSource(source)`          | content from a remote storage: [`consul.Document`](https://insei.github.io/tinyconf/2.1/drivers/consul/#document), [`etcd.Document`](https://insei.github.io/tinyconf/2.1/drivers/etcd/#document)                   |
| `WithOptional()`              | files can be absent                                                                                                                                                                                                 |
| `WithEnvExpansion()`          | replaces `${VAR}` in the content with environment variables                                                                                                                                                         |
| `WithWatchInterval(interval)` | interval of polling the files for [reloading](https://insei.github.io/tinyconf/2.1/features/reloading/index.md), 5 seconds by default; changes are detected by the events of the file system without waiting for it |

Files are [merged](https://insei.github.io/tinyconf/2.1/drivers/yaml/#several-files) in the order of addition, sections are merged by keys.

## Documentation

`GenDoc("ini")` generates the INI file with the current values: the values of the root first, then the sections of the nested structs. Values of `hidden:"true"` fields are empty, the values that need it are quoted, so the file is read back to the same config.

```ini
hosts = a,b
name = 'my app; with comment chars'

[http]
host = example.com
port = 9090
```
