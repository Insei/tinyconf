package tinyconf

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestManager_Parse_Merge(t *testing.T) {
	type Config struct {
		Hosts   []string          `merge:"true"`
		Labels  map[string]string `merge:"true"`
		Ports   *[]int            `merge:"true"`
		Replace []string
	}
	defaults := newMapMockDriver("defaults", map[string]any{
		"Hosts": []string{"a"}, "Labels": map[string]string{"env": "dev", "team": "core"}, "Replace": []string{"a"},
	})
	file := newMapMockDriver("file", map[string]any{
		"Hosts": []string{"b"}, "Labels": map[string]string{"env": "prod"}, "Ports": &[]int{80}, "Replace": []string{"b"},
	})
	env := newMapMockDriver("env", map[string]any{"Ports": &[]int{443}})
	log := newDebugLogger()
	c, err := New(WithLogger(log), WithDriver(defaults), WithDriver(file), WithDriver(env))
	assert.NoError(t, err)
	// the value of the struct is replaced by the first driver, it is not merged
	conf := &Config{Hosts: []string{"initial"}}
	assert.NoError(t, c.Register(conf))
	assert.NoError(t, c.Parse(context.Background(), conf))
	assert.Equal(t, []string{"a", "b"}, conf.Hosts)
	assert.Equal(t, map[string]string{"env": "prod", "team": "core"}, conf.Labels)
	assert.Equal(t, &[]int{80, 443}, conf.Ports)
	assert.Equal(t, []string{"b"}, conf.Replace)
	// the source is the last driver, the log has every step
	assert.Contains(t, c.Sources(conf), FieldSource{Field: "Hosts", Driver: "file", Source: "hosts"})
	assert.Contains(t, log.take(), "override: config=*tinyconf.Config driver=file field=Hosts key=hosts value=[a b]")

	// the next Parse does not duplicate the items
	assert.NoError(t, c.Parse(context.Background(), conf))
	assert.Equal(t, []string{"a", "b"}, conf.Hosts)
}
