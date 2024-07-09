[![codecov](https://codecov.io/gh/Insei/tinyconf/graph/badge.svg?token=TSOJ1SWF9D)](https://codecov.io/gh/Insei/tinyconf)
[![build](https://github.com/insei/tinyconf/actions/workflows/go.yml/badge.svg)](https://github.com/Insei/tinyconf/actions/workflows/go.yml)
[![Goreport](https://goreportcard.com/badge/github.com/insei/tinyconf)](https://goreportcard.com/report/github.com/insei/tinyconf)
[![GoDoc](https://godoc.org/github.com/insei/tinyconf?status.svg)](https://godoc.org/github.com/insei/tinyconf)
# tinyconf
tinyconf - a simple and universal library for parsing configs.

# Installation
Install via go get. Note that Go 1.18 or newer is required.
```sh
go get github.com/insei/tinyconf@latest
```

# Description
tinyconf parses the config and returns the config structure. When initializing tinyconf.Manager, you can add a logger to it, as well as drivers (for priority in parsing the config, i.e. it will check them starting from the last one you specified). You can also add your own driver.

tinyconf.Manager has methods:
```
Register(conf any) error
Parse(conf any) error 
```
where: <br>
`Register(conf any) error` - registers map[strings]fmap.Field for the config.<br>
`Parse(conf any) error` - parses config from registered drivers.<br>

# Example

```go
package main

import (
	"fmt"
	"github.com/google/uuid"
	"tinyconf"
	"tinyconf/drivers/env"
	"tinyconf/drivers/tag"
	"tinyconf/drivers/yaml"
	"tinyconf/logger"
)

type Embedded struct {
	Test string `initial:"Shakalaka"`
}

type DefConf struct {
	Embedded Embedded
	Test     string    `initial:"123" yaml:"test" env:"DEFAULT_TEST"`
	Test2    int       `initial:"123"  yaml:"test2"`
	Test3    int32     `initial:"2" yaml:"test3,omitempty"`
	Test4    *int32    `initial:"3" yaml:"test4,omitempty"`
	Test5    *string   `initial:"*string" yaml:"test5,omitempty" env:"DEFAULT_TEST"`
	Test6    string    `yaml:"test6,omitempty"`
	Test7    uuid.UUID `initial:"f9a49892-860e-48ec-b927-73f9d2560eec"`
	Password string    `initial:"Qwerty" yaml:"password" hidden:"true"`
}

func main() {
	yamlDriver, err := yaml.New("config.yaml")
	if err != nil {
		return
	}
	envDriver, err := env.New()
	if err != nil {
		return
	}
	tagDriver, err := tag.New("initial")
	if err != nil {
		return
	}
	config, err := tinyconf.New(
		tinyconf.WithLogger(logger.NewFmtLogger(logger.TRACE)),
		tinyconf.WithDriver(tagDriver),
		tinyconf.WithDriver(yamlDriver),
		tinyconf.WithDriver(envDriver),
	)
	if err != nil {
		return
	}
	c := DefConf{
		Embedded: Embedded{},
		Test:     "Tests",
		Test2:    22,
		Test3:    0,
		Test4:    nil,
		Test5:    nil,
		Test6:    "",
		Test7:    uuid.UUID{},
		Password: "",
	}
	err = config.Register(&c)
	if err != nil {
		panic(err)
	}
	err = config.Parse(&c)
	if err != nil {
		panic(err)
	}
	fmt.Print(c)
}
```

More examples in `parser_test.go` and others test files.
