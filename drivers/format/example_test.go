package format_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/format"
)

// A format is plugged by the decoder: here it is JSON without the lines with // comments.
func ExampleNew() {
	decode := func(data []byte) (map[string]any, error) {
		var lines []string
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "//") {
				lines = append(lines, line)
			}
		}
		var doc map[string]any
		return doc, json.Unmarshal([]byte(strings.Join(lines, "\n")), &doc)
	}
	type Config struct {
		HTTP struct {
			Port int `jsonc:"port"`
		} `jsonc:"http"`
	}
	driver, err := format.New("jsonc", decode, "", format.WithData("config.jsonc", []byte(`{
		// the port of the server
		"http": {"port": 9090}
	}`)))
	if err != nil {
		panic(err)
	}
	config, err := tinyconf.New(tinyconf.WithDriver(driver))
	if err != nil {
		panic(err)
	}
	conf := &Config{}
	if err = config.Register(conf); err != nil {
		panic(err)
	}
	if err = config.Parse(context.Background(), conf); err != nil {
		panic(err)
	}
	fmt.Println(conf.HTTP.Port)
	// Output: 9090
}
