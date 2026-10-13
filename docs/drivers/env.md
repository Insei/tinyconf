# env

The env driver reads values from environment variables.

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
    Debug bool `env:"DEBUG"`
}
```

```bash
HTTP_HOST=example.com
HTTP_PORT=9090
DEBUG=true
```

## .env files

`env.New` reads the `.env` files from the directory of the executable and from the working directory and exports their values to the process environment.

```bash
# comment
HTTP_HOST=example.com
TOKEN="quoted value"
```

* Empty lines and lines started with `#` are skipped.
* Matching single or double quotes around the value are removed.
* Variables that are already set in the process environment are not overridden, so the real environment has priority over the files.

## Documentation

`GenDoc("env")` generates a commented template of the `.env` file, see [Documentation generation](../features/gendoc.md).
