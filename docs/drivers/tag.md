# tag

The tag driver takes values right from the struct tags. It is used for default values.

```go
import "github.com/insei/tinyconf/drivers/tag"

tagDriver, err := tag.New("initial")
```

The argument is the name of the tag with values, any name can be used: `initial`, `default`.

```go
type Config struct {
    Host  string  `initial:"localhost"`
    Port  int     `initial:"8080"`
    Debug bool    `initial:"true"`
    Ratio float64 `initial:"0.5"`
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

An incorrect default value is an error of `Parse`, so a typo in the tag is found at the first start.

The driver has no own documentation format, `GenDoc` returns an empty string for it. Default values are visible in the documentation of other drivers as the values of the fields.
