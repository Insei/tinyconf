package convert

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/insei/fmap/v3"
)

var (
	typeOfTextMarshaler = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	typeOfStringer      = reflect.TypeOf((*fmt.Stringer)(nil)).Elem()
)

// ToTree converts the value to the tree of scalars, []any and map[string]any.
// The tree can be encoded to yaml or json and parsed back by FromTree.
// Tag is a struct tag with key names for structs nested into slices and maps.
func ToTree(v any, tag string) any {
	if tag == "" {
		tag = DefaultTag
	}
	return toTree(reflect.ValueOf(v), tag)
}

func toTree(val reflect.Value, tag string) any {
	for val.IsValid() && (val.Kind() == reflect.Ptr || val.Kind() == reflect.Interface) {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}
	if !val.IsValid() {
		return nil
	}
	t := val.Type()
	if isScalar(t) {
		return scalarToTree(val)
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		items := make([]any, val.Len())
		for i := range items {
			items[i] = toTree(val.Index(i), tag)
		}
		return items
	case reflect.Map:
		items := make(map[string]any, val.Len())
		iter := val.MapRange()
		for iter.Next() {
			items[fmt.Sprint(toTree(iter.Key(), tag))] = toTree(iter.Value(), tag)
		}
		return items
	case reflect.Struct:
		items := make(map[string]any, t.NumField())
		structToMap(val, items, tag)
		return items
	}
	return fmt.Sprint(val.Interface())
}

func structToMap(val reflect.Value, items map[string]any, tag string) {
	t := val.Type()
	for i := 0; i < t.NumField(); i++ {
		fld := t.Field(i)
		name := strings.Split(fld.Tag.Get(tag), ",")[0]
		if name == "-" {
			continue
		}
		if fld.Anonymous && name == "" && !IsLeaf(fld.Type) {
			embedded := val.Field(i)
			if embedded.Kind() == reflect.Ptr {
				if embedded.IsNil() || embedded.Elem().Kind() != reflect.Struct {
					continue
				}
				embedded = embedded.Elem()
			}
			structToMap(embedded, items, tag)
			continue
		}
		if fld.PkgPath != "" {
			continue
		}
		if name == "" {
			name = fld.Name
		}
		items[name] = toTree(val.Field(i), tag)
	}
}

func scalarToTree(val reflect.Value) any {
	t := val.Type()
	switch {
	case t == typeOfDuration:
		return val.Interface().(time.Duration).String()
	case t == typeOfTime:
		return val.Interface().(time.Time).Format(time.RFC3339Nano)
	case t == typeOfBytes:
		return string(val.Bytes())
	}
	// methods with pointer receivers should be available too
	addressable := reflect.New(t)
	addressable.Elem().Set(val)
	if addressable.Type().Implements(typeOfTextMarshaler) {
		if text, err := addressable.Interface().(encoding.TextMarshaler).MarshalText(); err == nil {
			return string(text)
		}
	}
	if addressable.Type().Implements(typeOfStringer) {
		return addressable.Interface().(fmt.Stringer).String()
	}
	if basicType, ok := basicTypes[t.Kind()]; ok {
		return val.Convert(basicType).Interface()
	}
	return fmt.Sprint(val.Interface())
}

// ToString formats the value to the string that can be parsed back by FromString.
// Slices, arrays and maps of the simple values are formatted with separators, all nested types are formatted as JSON.
func ToString(v any, opts Options) string {
	opts = opts.withDefaults()
	switch tree := ToTree(v, opts.Tag).(type) {
	case nil:
		return ""
	case []any:
		items := make([]string, len(tree))
		for i, item := range tree {
			if !isTreeScalar(item) {
				return toJSON(tree)
			}
			items[i] = quote(fmt.Sprint(item), opts.Separator)
		}
		return strings.Join(items, opts.Separator)
	case map[string]any:
		items := make([]string, 0, len(tree))
		for key, item := range tree {
			if !isTreeScalar(item) || reflect.ValueOf(v).Kind() == reflect.Struct {
				return toJSON(tree)
			}
			items = append(items, quote(key, opts.Separator, opts.KeyValueSeparator)+opts.KeyValueSeparator+quote(fmt.Sprint(item), opts.Separator))
		}
		sort.Strings(items)
		return strings.Join(items, opts.Separator)
	default:
		return fmt.Sprint(tree)
	}
}

// FieldToString formats the value of the field to the string that can be parsed back by StringToField.
func FieldToString(v any, field fmap.Field) string {
	return ToString(v, fieldOptions(field, ""))
}

// FieldToTagString formats the value of the field to the string that can be parsed back by TreeToField with the tag:
// keys of the structs nested into slices and maps are taken from the tag, for the formats with string values like INI.
func FieldToTagString(v any, field fmap.Field, tag string) string {
	return ToString(v, fieldOptions(field, tag))
}

func isTreeScalar(v any) bool {
	switch v.(type) {
	case nil, []any, map[string]any:
		return false
	}
	return true
}

func toJSON(tree any) string {
	data, err := json.Marshal(tree)
	if err != nil {
		return fmt.Sprint(tree)
	}
	return string(data)
}

// quote adds quotes to the item if it can not be parsed back without them.
func quote(item string, separators ...string) string {
	need := item != strings.TrimSpace(item) || strings.HasPrefix(item, `"`) || strings.HasPrefix(item, "'")
	for _, sep := range separators {
		need = need || strings.Contains(item, sep)
	}
	if !need {
		return item
	}
	if strings.Contains(item, `"`) {
		return "'" + item + "'"
	}
	return `"` + item + `"`
}
