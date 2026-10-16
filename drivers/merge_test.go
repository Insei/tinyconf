package drivers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/env"
	"github.com/insei/tinyconf/drivers/tag"
	"github.com/insei/tinyconf/drivers/yaml"
)

func TestMergeTag(t *testing.T) {
	type config struct {
		Hosts  []string          `initial:"default" yaml:"hosts" env:"MERGE_HOSTS" merge:"true"`
		Labels map[string]string `initial:"env:dev,team:core" yaml:"labels" env:"MERGE_LABELS" merge:"true"`
	}
	t.Setenv("MERGE_HOSTS", "from-env")
	t.Setenv("MERGE_LABELS", "env:prod")
	tagDriver, err := tag.New("initial")
	assert.NoError(t, err)
	yamlDriver, err := yaml.New("", yaml.WithData("data", []byte("hosts: [from-file]\nlabels: {region: eu}\n")))
	assert.NoError(t, err)
	envDriver, err := env.New(env.WithFiles())
	assert.NoError(t, err)
	manager, err := tinyconf.New(tinyconf.WithDriver(tagDriver), tinyconf.WithDriver(yamlDriver), tinyconf.WithDriver(envDriver))
	assert.NoError(t, err)
	conf := &config{}
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))
	// the values of all drivers in the order of the drivers
	assert.Equal(t, []string{"default", "from-file", "from-env"}, conf.Hosts)
	assert.Equal(t, map[string]string{"env": "prod", "team": "core", "region": "eu"}, conf.Labels)
}
