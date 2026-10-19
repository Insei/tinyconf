# tag

The tag driver takes values right from the struct tags. It is used for default values.

```go
import "github.com/insei/tinyconf/v2/drivers/tag"

tagDriver, err := tag.New("initial")
```

The argument is the name of the tag with values, any name can be used: `initial`, `default`.

```go
type Config struct {
    Host    string        `initial:"localhost"`
    Port    int           `initial:"8080"`
    Timeout time.Duration `initial:"30s"`
    Hosts   []string      `initial:"a,b"`
}
```

Add it first, so all other drivers can override the defaults:

```go
config, err := tinyconf.New(
    tinyconf.WithDriver(tagDriver),
    tinyconf.WithDriver(yamlDriver),
    tinyconf.WithDriver(envDriver),
)
```

Values are parsed by the rules of [string values](https://insei.github.io/tinyconf/2.2/features/types/index.md). An incorrect default value is an error of `Parse`, so a typo in the tag is found at the first start.

The driver has no own documentation format, `GenDoc` returns an empty string for it. Default values are visible in the documentation of other drivers as the values of the fields.
