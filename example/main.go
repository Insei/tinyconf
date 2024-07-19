package main

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/env"
	"github.com/insei/tinyconf/drivers/tag"
	"github.com/insei/tinyconf/drivers/yaml"
	"github.com/insei/tinyconf/logger"
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
	fmt.Println(config.GenDoc("yaml"))
	err = config.Parse(&c)
	if err != nil {
		panic(err)
	}
}
