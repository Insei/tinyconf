package tinyconf

import (
	"testing"
	"time"
)

type benchHTTP struct {
	Host    string
	Port    int
	Timeout time.Duration
}

type benchConfig struct {
	Name     string
	Level    string
	Hosts    []string
	Labels   map[string]string
	Database struct {
		Host     string
		Port     int
		User     string
		Password string `hidden:"true"`
	}
	HTTP benchHTTP
}

func newBenchManager(b *testing.B) *Manager {
	b.Helper()
	c, err := New(WithDriver(&staticMockDriver{}))
	if err != nil {
		b.Fatal(err)
	}
	conf := &benchConfig{}
	if err = c.Register(conf); err != nil {
		b.Fatal(err)
	}
	if err = c.Parse(conf); err != nil {
		b.Fatal(err)
	}
	return c
}

func BenchmarkParse(b *testing.B) {
	c := newBenchManager(b)
	conf := &benchConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := c.Parse(conf); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParse_SubConfig(b *testing.B) {
	c := newBenchManager(b)
	http := &benchHTTP{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := c.Parse(http); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCurrent(b *testing.B) {
	c := newBenchManager(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if Current[benchConfig](c) == nil {
			b.Fatal("no current version")
		}
	}
}

func BenchmarkCurrent_SubConfig(b *testing.B) {
	c := newBenchManager(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if Current[benchHTTP](c) == nil {
			b.Fatal("no current version")
		}
	}
}
