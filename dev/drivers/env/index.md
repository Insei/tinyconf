# env

The env driver reads values from environment variables and `.env` files.

```go
import "github.com/insei/tinyconf/v2/drivers/env"

envDriver, err := env.New()
```

## Names

The `env` tag is the name of the variable. Names are not built from the names of the parent structs, the variable is read exactly as it is written in the tag:

```go
type Config struct {
    HTTP struct {
        Host string `env:"HTTP_HOST"`
        Port int    `env:"HTTP_PORT"`
    }
    Hosts []string `env:"HOSTS"`
}
```

```bash
HTTP_HOST=example.com
HTTP_PORT=9090
HOSTS=a.example.com,b.example.com
```

Values are parsed by the rules of [string values](https://insei.github.io/tinyconf/dev/features/types/index.md).

## Options

```go
envDriver, err := env.New(
    env.WithPrefix("APP_"),
    env.WithAutoKeys(),
    env.WithSecretFiles(),
    env.WithFiles(".env", ".env.local"),
    env.WithFilesIsolation(),
    env.WithIndexedKeys(),
)
```

| Option                        | Description                                                                          |
| ----------------------------- | ------------------------------------------------------------------------------------ |
| `WithPrefix(prefix)`          | adds the prefix to the names of all variables                                        |
| `WithAutoKeys()`              | reads the fields without the `env` tag, the name is built from the path of the field |
| `WithSecretFiles()`           | reads the value from the file, if the variable with the `_FILE` suffix is set        |
| `WithFiles(files...)`         | custom `.env` files instead of the default ones                                      |
| `WithFilesIsolation()`        | does not export the values of the `.env` files to the process environment            |
| `WithWatchInterval(interval)` | the interval of polling the `.env` files for changes, 5 seconds by default           |
| `WithIndexedKeys()`           | reads slices, arrays, maps and structs from separate variables                       |

### Prefix

With `env.WithPrefix("APP_")` the field with the `env:"HTTP_PORT"` tag is read from the `APP_HTTP_PORT` variable. It allows several applications to share one environment and keeps the tags short.

### Auto keys

With `env.WithAutoKeys()` the `env` tag is not required. The name of the variable is built from the struct path of the field:

| Field                                        | Variable         |
| -------------------------------------------- | ---------------- |
| `Name`                                       | `NAME`           |
| `HTTP.Port`                                  | `HTTP_PORT`      |
| `HTTP.MaxConns`                              | `HTTP_MAX_CONNS` |
| `DBName`                                     | `DB_NAME`        |
| `HTTP.Timeouts.Read`, `Timeouts` is embedded | `HTTP_READ`      |

The `env` tag still has priority, and the field can be excluded with `env:"-"`.

### Secret files

Docker and Kubernetes mount secrets as files. With `env.WithSecretFiles()` the value can be read from such file: if the `HTTP_TOKEN` variable is not set, but `HTTP_TOKEN_FILE` is set, the value is read from the file with the path from `HTTP_TOKEN_FILE`.

```go
type Config struct {
    Token string `env:"HTTP_TOKEN" hidden:"true"`
}
```

```bash
HTTP_TOKEN_FILE=/run/secrets/http_token
```

The trailing line break of the file content is removed. The variable with the value itself has priority over the file.

### Indexed keys

By default, a slice, a map or a list of structs is set by one variable with the whole value: `HOSTS=a,b` or JSON. With `env.WithIndexedKeys()` every item can have its own variable. It is the form that is natural for Kubernetes manifests and Docker Compose files, where a long JSON in one variable is hard to read and to override.

```go
type Server struct {
    Host     string `env:"HOST"`
    MaxConns int
    Tags     []string
}

type Config struct {
    Hosts   []string          `env:"HOSTS"`
    Labels  map[string]string `env:"LABELS"`
    Servers []Server          `env:"SERVERS"`
}
```

| Type                       | Variables                                                                              |
| -------------------------- | -------------------------------------------------------------------------------------- |
| `Hosts []string`           | `HOSTS_0=a`, `HOSTS_1=b`                                                               |
| `Labels map[string]string` | `LABELS_env=prod`, `LABELS_team=core`                                                  |
| `Servers []Server`         | `SERVERS_0_HOST=a`, `SERVERS_0_MAX_CONNS=10`, `SERVERS_0_TAGS_0=x`, `SERVERS_1_HOST=b` |

Rules:

- **Slices and arrays** — the index is added to the name of the field. Indexes should be sequential and start from 0, a gap is an error.
- **Maps** — the rest of the name after the prefix is a key, as is, case-sensitive.
- **Structs** inside slices and maps — names of the fields are taken from their `env` tags or built from the field names: `MaxConns` is `MAX_CONNS`. Fields with `env:"-"` are skipped.
- **Nesting** is not limited: `SERVERS_0_TAGS_0` is the first tag of the first server.
- The variable with the **whole value has priority**: if `HOSTS` is set, `HOSTS_0` is ignored.
- The [prefix](#prefix) is added to all names: `APP_HOSTS_0`.

Names of the map fields

For a map with simple values every variable started with the name of the field and `_` becomes an item of the map. With `Labels` read from `LABELS`, the variable `LABELS_FILE` is the `FILE` key. Choose the name of such field so that it is not a prefix of other variables.

## .env files

By default, `env.New` reads the `.env` files from the directory of the executable and from the working directory and exports their values to the process environment. Absent default files are skipped.

```bash
# comment
HTTP_HOST=example.com
TOKEN="quoted value"
```

- Empty lines and lines started with `#` are skipped.
- Matching single or double quotes around the value are removed.
- Variables that are already set in the process environment are not overridden, so the real environment has priority over the files.

### Custom files

```go
env.New(env.WithFiles(".env", ".env.local"))
```

`WithFiles` replaces the default files. All listed files should exist, otherwise `env.New` returns an error. The value from the previous file has priority over the next files, and the process environment has priority over all files.

`env.WithFiles()` without arguments disables reading of the `.env` files at all.

### Isolation

Exported values are visible to the whole process and to its child processes. With `env.WithFilesIsolation()` the values of the files are available only for the driver, `os.Getenv` does not see them. It is also useful in tests, where the changed environment of the process is a side effect.

### Reloading

The `.env` files are read again before every `Parse`, and they are watched for [reloading](https://insei.github.io/tinyconf/dev/features/reloading/index.md): a change of a file reloads the configs with subscribers. The files are watched by the events of the file system and by polling, as the files of the [yaml](https://insei.github.io/tinyconf/dev/drivers/yaml/index.md) driver; the default files that do not exist yet are watched too.

```go
envDriver, err := env.New(env.WithFiles(".env"), env.WithWatchInterval(time.Minute))
```

The driver changes only the variables that it exported itself: the new value of the file replaces the exported one, the variable removed from the file is removed from the process environment. Variables set by the environment of the process or changed by the application keep their values. The process environment itself is not watched, it is not changed from the outside of the process.

## Documentation

`GenDoc("env")` generates a commented template of the `.env` file, see [Documentation generation](https://insei.github.io/tinyconf/dev/features/gendoc/index.md). Names of the variables are shown with the prefix, fields without the `env` tag are included if the auto keys are enabled.
