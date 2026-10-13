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

type node struct {
	Value int
	Next  *node
}

func TestIsValueField(t *testing.T) {
	type config struct {
		Name   string
		At     time.Time
		Ptr    *server
		Double **server
		Node   *node
		Nested struct {
			At    *time.Time
			Inner *struct {
				Value int
			}
		}
	}
	storage, err := fmap.Get[config]()
	assert.NoError(t, err)
	tests := map[string]bool{
		"Name":    true,
		"At":      true,
		"At.wall": false,
		// struct stored by pointer is parsed by its fields
		"Ptr":         false,
		"Ptr.Host":    true,
		"Ptr.Timeout": true,
		"Ptr.Tags":    true,
		// fmap does not go into pointers to pointers and recursive types, they are parsed as a single value
		"Double":             true,
		"Node":               false,
		"Node.Value":         true,
		"Node.Next":          true,
		"Nested":             false,
		"Nested.At":          true,
		"Nested.At.wall":     false,
		"Nested.Inner":       false,
		"Nested.Inner.Value": true,
	}
	for path, want := range tests {
		assert.Equal(t, want, IsValueField(storage, storage.MustFind(path)), path)
	}
	assert.True(t, IsInsideValue(storage, storage.MustFind("At.wall")))
	assert.True(t, IsInsideValue(storage, storage.MustFind("Nested.At.wall")))
	assert.False(t, IsInsideValue(storage, storage.MustFind("Ptr.Host")))
	assert.False(t, IsInsideValue(storage, storage.MustFind("Nested.Inner.Value")))
}

func TestFieldValue_StructPointers(t *testing.T) {
	type tls struct {
		Cert string
		key  string
	}
	type backup struct {
		Host string
		TLS  *tls
		tls  *tls
	}
	type config struct {
		Backup *backup
	}
	conf := &config{}
	storage, err := fmap.GetFrom(conf)
	assert.NoError(t, err)

	// zero values are returned for the fields of not created structs, config is not changed
	assert.Equal(t, "", GetFieldValue(storage.MustFind("Backup.TLS.Cert"), conf))
	assert.Equal(t, (*tls)(nil), GetFieldValue(storage.MustFind("Backup.TLS"), conf))
	assert.Equal(t, config{}, *conf)
	assert.Equal(t, "", GetFieldValue(storage.MustFind("Backup.Host"), *conf))

	// structs are created on set
	SetFieldValue(storage.MustFind("Backup.TLS.Cert"), conf, "cert")
	SetFieldValue(storage.MustFind("Backup.TLS.key"), conf, "key")
	SetFieldValue(storage.MustFind("Backup.tls.Cert"), conf, "unexported")
	SetFieldValue(storage.MustFind("Backup.Host"), conf, "host")
	assert.Equal(t, config{Backup: &backup{
		Host: "host",
		TLS:  &tls{Cert: "cert", key: "key"},
		tls:  &tls{Cert: "unexported"},
	}}, *conf)
	assert.Equal(t, "cert", GetFieldValue(storage.MustFind("Backup.TLS.Cert"), conf))
	assert.Equal(t, "unexported", GetFieldValue(storage.MustFind("Backup.tls.Cert"), *conf))

	SetFieldValue(storage.MustFind("Backup.TLS"), conf, nil)
	assert.Nil(t, conf.Backup.TLS)
}

func TestCopySections(t *testing.T) {
	type tls struct {
		Cert string
	}
	type backup struct {
		Host string
		TLS  *tls
		At   *time.Time
		Node *node
	}
	type config struct {
		Backup *backup
		Empty  *backup
	}
	at := time.Now()
	conf := &config{Backup: &backup{Host: "host", TLS: &tls{Cert: "cert"}, At: &at, Node: &node{Next: &node{Value: 2}}}}
	storage, err := fmap.GetFrom(conf)
	assert.NoError(t, err)

	copied := *conf
	CopySections(storage, &copied)
	assert.Equal(t, *conf, copied)
	assert.NotSame(t, conf.Backup, copied.Backup)
	assert.NotSame(t, conf.Backup.TLS, copied.Backup.TLS)
	assert.NotSame(t, conf.Backup.Node, copied.Backup.Node)
	assert.Nil(t, copied.Empty)
	// values parsed as a whole are replaced by drivers, not changed in place
	assert.Same(t, conf.Backup.At, copied.Backup.At)
	assert.Same(t, conf.Backup.Node.Next, copied.Backup.Node.Next)

	SetFieldValue(storage.MustFind("Backup.TLS.Cert"), &copied, "changed")
	SetFieldValue(storage.MustFind("Backup.Node.Value"), &copied, 5)
	assert.Equal(t, "cert", conf.Backup.TLS.Cert)
	assert.Equal(t, 0, conf.Backup.Node.Value)
}
