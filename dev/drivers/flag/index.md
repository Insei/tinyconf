# flag

The flag driver reads values from command line flags.

```go
import "github.com/insei/tinyconf/v2/drivers/flag"

flagDriver, err := flag.New()
```

Add it last, so the flags have the highest priority:

```go
config, err := tinyconf.New(
    tinyconf.WithDriver(tagDriver),
    tinyconf.WithDriver(yamlDriver),
    tinyconf.WithDriver(envDriver),
    tinyconf.WithDriver(flagDriver),
)
```

## Names

The `flag` tag is the name of the flag without dashes:

```go
type Config struct {
    HTTP struct {
        Host string `flag:"http-host" doc:"http host"`
        Port int    `flag:"http-port" doc:"http port"`
    }
    Hosts   []string `flag:"host" doc:"allowed hosts"`
    Verbose bool     `flag:"verbose"`
}
```

```bash
app --http-host=example.com --http-port 9090 --host=a --host=b -verbose
```

A field without the `flag` tag is not read from the command line.

## Supported forms

| Form                              | Example            |
| --------------------------------- | ------------------ |
| `-name=value`, `--name=value`     | `--http-port=9090` |
| `-name value`, `--name value`     | `--http-port 9090` |
| `-name`, `--name` for bool fields | `--verbose`        |

- Bool fields do not take the next argument as a value: use `--verbose=false` to set false.
- Slices and arrays can be filled with repeated flags: `--host=a --host=b,c` is `[a b c]`. For all other types the last occurrence of the flag is used.
- All arguments after `--` are ignored.
- Values are parsed by the rules of [string values](https://insei.github.io/tinyconf/dev/features/types/index.md).

## Unknown flags

The driver does not own the command line: it only looks for the flags of the config fields. Unknown flags and positional arguments are ignored, so the driver can be used together with the `flag` package of the standard library or with any CLI framework.

No --help

The driver does not handle `-h` and `--help`. Print [`GenDoc("flag")`](#documentation) from your own handler of the help flag.

## Custom arguments

By default `os.Args[1:]` is used. Custom arguments are set with `WithArgs`, in tests or for sub commands:

```go
flagDriver, err := flag.New(flag.WithArgs(os.Args[2:]))
```

## Documentation

`GenDoc("flag")` generates the usage of all flags with descriptions from the `doc` tags and current values as defaults:

```text
  -http-host
        http host (default example.com)
  -http-port
        http port (default 9090)
  -host
        allowed hosts (default a,b)
  -verbose
         (default false)
```

Values of the fields with `hidden:"true"` are not shown.
