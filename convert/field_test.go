package convert

import (
	"testing"
	"time"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

type fieldConfig struct {
	Timeout time.Duration
	Level   *namedInt
	Hosts   []string       `separator:";"`
	Labels  map[string]int `separator:";" kvseparator:"="`
	Servers []server
	Nested  struct {
		Name  string
		At    time.Time
		inner struct{ value int }
	}
}

func TestStringToField(t *testing.T) {
	storage, err := fmap.Get[fieldConfig]()
	assert.NoError(t, err)

	got, err := StringToField("a,b;c", storage.MustFind("Hosts"))
	assert.NoError(t, err)
	assert.Equal(t, []string{"a,b", "c"}, got)

	got, err = StringToField("a=1;b=2", storage.MustFind("Labels"))
	assert.NoError(t, err)
	assert.Equal(t, map[string]int{"a": 1, "b": 2}, got)

	_, err = StringToField("5", storage.MustFind("Timeout"))
	assert.Error(t, err)
}

func TestTreeToField(t *testing.T) {
	storage, err := fmap.Get[fieldConfig]()
	assert.NoError(t, err)

	got, err := TreeToField([]any{map[string]any{"address": "a"}}, storage.MustFind("Servers"), "yaml")
	assert.NoError(t, err)
	assert.Equal(t, []server{{Host: "a"}}, got)

	got, err = TreeToField("a;b", storage.MustFind("Hosts"), "yaml")
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestGetSetFieldValue(t *testing.T) {
	storage, err := fmap.Get[fieldConfig]()
	assert.NoError(t, err)
	conf := &fieldConfig{}

	timeout := storage.MustFind("Timeout")
	SetFieldValue(timeout, conf, 5*time.Second)
	assert.Equal(t, 5*time.Second, conf.Timeout)
	assert.Equal(t, 5*time.Second, GetFieldValue(timeout, conf))

	level := storage.MustFind("Level")
	SetFieldValue(level, conf, ptr(namedInt(2)))
	assert.Equal(t, ptr(namedInt(2)), conf.Level)
	assert.Equal(t, ptr(namedInt(2)), GetFieldValue(level, conf))
	SetFieldValue(level, conf, nil)
	assert.Nil(t, conf.Level)

	at := storage.MustFind("Nested.At")
	date := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	SetFieldValue(at, conf, date)
	assert.Equal(t, date, conf.Nested.At)
	assert.Equal(t, date, GetFieldValue(at, conf))
	assert.Equal(t, "", conf.Nested.Name)

	unexported := storage.MustFind("Nested.inner.value")
	SetFieldValue(unexported, conf, 42)
	assert.Equal(t, 42, conf.Nested.inner.value)
	assert.Equal(t, 42, GetFieldValue(unexported, conf))

	// not a pointer
	assert.Equal(t, date, GetFieldValue(at, *conf))

	hosts := storage.MustFind("Hosts")
	SetFieldValue(hosts, conf, []string{"a"})
	assert.Equal(t, []string{"a"}, GetFieldValue(hosts, conf))
}

func TestIsValueField(t *testing.T) {
	type config struct {
		Name   string
		At     time.Time
		Ptr    *server
		Nested struct {
			At    *time.Time
			Inner struct {
				Value int
			}
		}
	}
	storage, err := fmap.Get[config]()
	assert.NoError(t, err)
	tests := map[string]bool{
		"Name":               true,
		"At":                 true,
		"At.wall":            false,
		"Ptr":                true,
		"Nested":             false,
		"Nested.At":          true,
		"Nested.Inner":       false,
		"Nested.Inner.Value": true,
	}
	for path, want := range tests {
		assert.Equal(t, want, IsValueField(storage, storage.MustFind(path)), path)
	}
}
