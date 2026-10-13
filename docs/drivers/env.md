# env

The env driver reads values from environment variables and `.env` files.

```go
import "github.com/insei/tinyconf/drivers/env"

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

Values are parsed by the rules of [string values](../features/types.md).

## Options

```go
envDriver, err := env.New(
    env.WithPrefix("APP_"),
    env.WithAutoKeys(),
    env.WithSecretFiles(),
    env.WithFiles(".env", ".env.local"),
    env.WithFilesIsolation(),
)
```

| Option | Description |
|---|---|
| `WithPrefix(prefix)` | adds the prefix to the names of all variables |
| `WithAutoKeys()` | reads the fields without the `env` tag, the name is built from the path of the field |
| `WithSecretFiles()` | reads the value from the file, if the variable with the `_FILE` suffix is set |
| `WithFiles(files...)` | custom `.env` files instead of the default ones |
| `WithFilesIsolation()` | does not export the values of the `.env` files to the process environment |

### Prefix

With `env.WithPrefix("APP_")` the field with the `env:"HTTP_PORT"` tag is read from the `APP_HTTP_PORT` variable. It allows several applications to share one environment and keeps the tags short.

### Auto keys

With `env.WithAutoKeys()` the `env` tag is not required. The name of the variable is built from the struct path of the field:

| Field | Variable |
|---|---|
| `Name` | `NAME` |
| `HTTP.Port` | `HTTP_PORT` |
| `HTTP.MaxConns` | `HTTP_MAX_CONNS` |
| `DBName` | `DB_NAME` |

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

## .env files

By default, `env.New` reads the `.env` files from the directory of the executable and from the working directory and exports their values to the process environment. Absent default files are skipped.

```bash
# comment
HTTP_HOST=example.com
TOKEN="quoted value"
```

* Empty lines and lines started with `#` are skipped.
* Matching single or double quotes around the value are removed.
* Variables that are already set in the process environment are not overridden, so the real environment has priority over the files.

### Custom files

```go
env.New(env.WithFiles(".env", ".env.local"))
```

`WithFiles` replaces the default files. All listed files should exist, otherwise `env.New` returns an error. The value from the previous file has priority over the next files, and the process environment has priority over all files.

`env.WithFiles()` without arguments disables reading of the `.env` files at all.

### Isolation

Exported values are visible to the whole process and to its child processes. With `env.WithFilesIsolation()` the values of the files are available only for the driver, `os.Getenv` does not see them. It is also useful in tests, where the changed environment of the process is a side effect.

## Documentation

`GenDoc("env")` generates a commented template of the `.env` file, see [Documentation generation](../features/gendoc.md). Names of the variables are shown with the prefix, fields without the `env` tag are included if the auto keys are enabled.
