package properties

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// FuzzDecode checks that decode does not panic and that the decoded document is encoded and decoded back to the same document.
func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		"name=app\nhttp.port=8080\n",
		"# comment\n! comment\nkey\\ with\\=separators = value\nlong = a,\\\n   b\n",
		"unicode = \\u043f\\u0440\nescapes = tab\\tnew\\nline\n",
		"empty\ntail = end\\",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := decode(data)
		if err != nil {
			return
		}
		lines := make(map[string]string)
		flatten("", doc, lines)
		keys := make([]string, 0, len(lines))
		for key := range lines {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var encoded strings.Builder
		for _, key := range keys {
			encoded.WriteString(escape(key, true) + "=" + escape(lines[key], false) + "\n")
		}
		decoded, err := decode([]byte(encoded.String()))
		if err != nil {
			t.Fatalf("encoded document is not decoded: %v\n%s", err, encoded.String())
		}
		if !reflect.DeepEqual(doc, decoded) {
			t.Fatalf("document is changed by encoding:\n%#v\n%s\n%#v", doc, encoded.String(), decoded)
		}
	})
}
