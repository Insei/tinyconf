package ini

import (
	"reflect"
	"strings"
	"testing"
)

// withoutEmptyObjects removes the objects without values: the sections without keys are not written by encode.
func withoutEmptyObjects(doc map[string]any) map[string]any {
	for key, value := range doc {
		if nested, ok := value.(map[string]any); ok {
			if withoutEmptyObjects(nested); len(nested) == 0 {
				delete(doc, key)
			}
		}
	}
	return doc
}

// FuzzDecode checks that decode does not panic and that the decoded document is encoded and decoded back to the same document.
func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		"name = app\n[http]\nport = 8080\n",
		"; comment\n# comment\nkey: value ; inline\n[a.b]\nc = 'quoted ; #'\nd = \"esc\\\"aped\\n\"\n",
		"http.port = 1\n[x]\nhttp.host = a\n",
		"[http]\n[http.tls]\ncert =\n",
		"key = \" padded \"\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := decode(data)
		if err != nil {
			return
		}
		var encoded strings.Builder
		encode(&encoded, nil, doc)
		decoded, err := decode([]byte(encoded.String()))
		if err != nil {
			t.Fatalf("encoded document is not decoded: %v\n%s", err, encoded.String())
		}
		if !reflect.DeepEqual(withoutEmptyObjects(doc), withoutEmptyObjects(decoded)) {
			t.Fatalf("document is changed by encoding:\n%#v\n%s\n%#v", doc, encoded.String(), decoded)
		}
	})
}
