package env

import (
	"reflect"
	"strings"
	"testing"
)

type fuzzServer struct {
	Host string
	Port int
	Tags []string
}

type fuzzEmbedded struct {
	fuzzServer
	Name string `env:"NAME"`
}

// FuzzIndexedTree checks that the variables of any names and values do not panic.
func FuzzIndexedTree(f *testing.F) {
	for _, seed := range []string{
		"0=a\n1=b", "0_HOST=a\n0_PORT=1\n0_TAGS_0=x", "env=prod\nTEAM_NAME=core", "a_0=1\nb=2,3",
		"0_0=1\n0_1=2\n1=3,4", "=whole", "0_NAME=n\n0_HOST=h", "01=x\n-1=y\n0=", "HOST=a\nHOST_NAME=b",
	} {
		f.Add(seed)
	}
	types := []reflect.Type{
		reflect.TypeOf([]string{}), reflect.TypeOf([2]int{}), reflect.TypeOf(map[string]string{}),
		reflect.TypeOf([][]int{}), reflect.TypeOf([]fuzzServer{}), reflect.TypeOf(map[string]fuzzServer{}),
		reflect.TypeOf(&fuzzServer{}), reflect.TypeOf(map[string][]int{}), reflect.TypeOf([]fuzzEmbedded{}),
	}
	f.Fuzz(func(t *testing.T, input string) {
		vars := make(map[string]string)
		for _, line := range strings.Split(input, "\n") {
			name, value, _ := strings.Cut(line, "=")
			vars[name] = value
		}
		for _, typeOf := range types {
			_, _, _ = indexedTree(typeOf, vars)
		}
	})
}
