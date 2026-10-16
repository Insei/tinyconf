// Package convert contains the single values conversion layer for all tinyconf drivers.
//
// Drivers are divided into two groups:
//   - string sources (env, struct tags, flags), they should use FromString;
//   - tree sources (yaml, json), they should use FromTree.
//
// Both entry points uses the same rules, so set of supported types is the same for all drivers.
package convert

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/insei/cast"
)

const (
	// DefaultSeparator is a default separator of slice, array and map items in string values.
	DefaultSeparator = ","
	// DefaultKeyValueSeparator is a default separator of map key and value in string values.
	DefaultKeyValueSeparator = ":"
	// DefaultTag is a default struct tag with key names for structs nested into slices and maps.
	DefaultTag = "json"
)

// Options is a conversion settings.
type Options struct {
	// Separator of slice, array and map items in string values.
	Separator string
	// KeyValueSeparator is a separator of map key and value in string values.
	KeyValueSeparator string
	// Tag is a struct tag with key names for structs nested into slices and maps.
	// If field has no such tag, field name is used (case-insensitive).
	Tag string
}

func (o Options) withDefaults() Options {
	if o.Separator == "" {
		o.Separator = DefaultSeparator
	}
	if o.KeyValueSeparator == "" {
		o.KeyValueSeparator = DefaultKeyValueSeparator
	}
	if o.Tag == "" {
		o.Tag = DefaultTag
	}
	return o
}

var (
	typeOfTime            = reflect.TypeOf(time.Time{})
	typeOfDuration        = reflect.TypeOf(time.Duration(0))
	typeOfURL             = reflect.TypeOf(url.URL{})
	typeOfBytes           = reflect.TypeOf([]byte(nil))
	typeOfTextUnmarshaler = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

	// additional time layouts, RFC3339 is parsed by cast
	timeLayouts = []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"}

	basicTypes = map[reflect.Kind]reflect.Type{
		reflect.Bool:    reflect.TypeOf(false),
		reflect.String:  reflect.TypeOf(""),
		reflect.Int:     reflect.TypeOf(int(0)),
		reflect.Int8:    reflect.TypeOf(int8(0)),
		reflect.Int16:   reflect.TypeOf(int16(0)),
		reflect.Int32:   reflect.TypeOf(int32(0)),
		reflect.Int64:   reflect.TypeOf(int64(0)),
		reflect.Uint:    reflect.TypeOf(uint(0)),
		reflect.Uint8:   reflect.TypeOf(uint8(0)),
		reflect.Uint16:  reflect.TypeOf(uint16(0)),
		reflect.Uint32:  reflect.TypeOf(uint32(0)),
		reflect.Uint64:  reflect.TypeOf(uint64(0)),
		reflect.Float32: reflect.TypeOf(float32(0)),
		reflect.Float64: reflect.TypeOf(float64(0)),
	}
)

func isTextUnmarshaler(t reflect.Type) bool {
	return reflect.PointerTo(t).Implements(typeOfTextUnmarshaler)
}

// IsLeaf reports whether the type is parsed from a single value,
// i.e. drivers should not go into the fields of this type, even if it is a struct.
func IsLeaf(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return true
	}
	return t == typeOfTime || t == typeOfURL || isTextUnmarshaler(t)
}

// IsScalar reports whether the type is parsed from a single string without any separators:
// all types except slices, arrays, maps, structs that are not parsed from a single value, and pointers to them.
func IsScalar(t reflect.Type) bool {
	return isScalar(indirect(t))
}

// isScalar reports whether the type is parsed from a single string without any separators.
func isScalar(t reflect.Type) bool {
	if t == typeOfBytes {
		return true
	}
	if _, ok := basicTypes[t.Kind()]; ok {
		return true
	}
	return t == typeOfTime || t == typeOfURL || isTextUnmarshaler(t)
}

// FromString converts string value to the value with type t.
//
// Supported types:
//   - bool, string, integers, floats and all named types based on them;
//   - time.Duration, time.Time (RFC3339), url.URL, []byte;
//   - all types that implements encoding.TextUnmarshaler;
//   - slices and arrays: "a,b,c";
//   - maps: "key1:value1,key2:value2";
//   - structs and any nested slices, arrays and maps as JSON: `[{"host":"a"}]`;
//   - pointers to all types above.
//
// For slices, arrays, maps and structs the value is parsed as JSON if it starts with '[' or '{'.
// Items that contains separator can be quoted with double or single quotes.
func FromString(s string, t reflect.Type, opts Options) (any, error) {
	val, err := fromString(s, t, opts.withDefaults())
	if err != nil {
		return nil, err
	}
	return val.Interface(), nil
}

// FromTree converts value decoded from yaml, json or similar formats to the value with type t.
// Value can be a scalar, []any, map[string]any or map[any]any.
// Set of supported types is the same as in FromString, strings in the tree is parsed by FromString rules.
func FromTree(v any, t reflect.Type, opts Options) (any, error) {
	val, err := fromTree(v, t, opts.withDefaults())
	if err != nil {
		return nil, err
	}
	return val.Interface(), nil
}

func errUnsupported(s string, t reflect.Type) error {
	return fmt.Errorf("failed to convert %s to %s, unsupported type", s, t.String())
}

func fromString(s string, t reflect.Type, opts Options) (reflect.Value, error) {
	if t.Kind() == reflect.Ptr {
		elem, err := fromString(s, t.Elem(), opts)
		if err != nil {
			return reflect.Value{}, err
		}
		ptr := reflect.New(t.Elem())
		ptr.Elem().Set(elem)
		return ptr, nil
	}
	if isScalar(t) {
		return scalarFromString(s, t)
	}
	switch t.Kind() {
	case reflect.Interface:
		if t.NumMethod() == 0 {
			val := reflect.New(t).Elem()
			val.Set(reflect.ValueOf(s))
			return val, nil
		}
	case reflect.Slice, reflect.Array:
		if isJSON(s, '[') {
			return fromJSON(s, t, opts)
		}
		if !isScalar(indirect(t.Elem())) && t.Elem().Kind() != reflect.Interface {
			return reflect.Value{}, fmt.Errorf("failed to convert %s to %s, use JSON for nested types", s, t.String())
		}
		items, err := split(s, opts.Separator, "")
		if err != nil {
			return reflect.Value{}, err
		}
		return sequence(t, len(items), func(i int) (reflect.Value, error) {
			return fromString(unquote(items[i]), t.Elem(), opts)
		})
	case reflect.Map:
		if isJSON(s, '{') {
			return fromJSON(s, t, opts)
		}
		if !isScalar(indirect(t.Elem())) && t.Elem().Kind() != reflect.Interface {
			return reflect.Value{}, fmt.Errorf("failed to convert %s to %s, use JSON for nested types", s, t.String())
		}
		items, err := split(s, opts.Separator, opts.KeyValueSeparator)
		if err != nil {
			return reflect.Value{}, err
		}
		val := reflect.MakeMapWithSize(t, len(items))
		for _, item := range items {
			keyStr, elemStr, ok := splitKeyValue(item, opts.KeyValueSeparator)
			if !ok {
				return reflect.Value{}, fmt.Errorf("failed to convert %s to %s, item %s has no key value separator %s",
					s, t.String(), item, opts.KeyValueSeparator)
			}
			key, err := fromString(keyStr, t.Key(), opts)
			if err != nil {
				return reflect.Value{}, err
			}
			elem, err := fromString(elemStr, t.Elem(), opts)
			if err != nil {
				return reflect.Value{}, err
			}
			val.SetMapIndex(key, elem)
		}
		return val, nil
	case reflect.Struct:
		if isJSON(s, '{') {
			return fromJSON(s, t, opts)
		}
		return reflect.Value{}, fmt.Errorf("failed to convert %s to %s, use JSON for structs", s, t.String())
	}
	return reflect.Value{}, errUnsupported(s, t)
}

func scalarFromString(s string, t reflect.Type) (reflect.Value, error) {
	switch {
	case t == typeOfDuration:
		d, err := time.ParseDuration(s)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(d), nil
	case t == typeOfTime:
		val, err := cast.ToReflect(s, t)
		if err == nil {
			return reflect.ValueOf(val), nil
		}
		for _, layout := range timeLayouts {
			if parsed, layoutErr := time.Parse(layout, s); layoutErr == nil {
				return reflect.ValueOf(parsed), nil
			}
		}
		return reflect.Value{}, err
	case t == typeOfURL:
		u, err := url.Parse(s)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(*u), nil
	case isTextUnmarshaler(t):
		ptr := reflect.New(t)
		if err := ptr.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s)); err != nil {
			return reflect.Value{}, err
		}
		return ptr.Elem(), nil
	case t == typeOfBytes:
		return reflect.ValueOf([]byte(s)), nil
	}
	basicType, ok := basicTypes[t.Kind()]
	if !ok {
		return reflect.Value{}, errUnsupported(s, t)
	}
	val, err := cast.ToReflect(s, basicType)
	if err != nil {
		return reflect.Value{}, err
	}
	// named types based on basic types
	return reflect.ValueOf(val).Convert(t), nil
}

// DecodeJSON decodes JSON to the tree for FromTree.
// Numbers are decoded to int if it is possible, otherwise to float64.
func DecodeJSON(data []byte) (any, error) {
	var tree any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&tree); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after JSON value")
	}
	return normalizeJSON(tree), nil
}

func normalizeJSON(v any) any {
	switch val := v.(type) {
	case json.Number:
		if i, err := strconv.Atoi(val.String()); err == nil {
			return i
		}
		if f, err := val.Float64(); err == nil {
			return f
		}
		return val.String()
	case []any:
		for i := range val {
			val[i] = normalizeJSON(val[i])
		}
	case map[string]any:
		for key := range val {
			val[key] = normalizeJSON(val[key])
		}
	}
	return v
}

func fromJSON(s string, t reflect.Type, opts Options) (reflect.Value, error) {
	tree, err := DecodeJSON([]byte(s))
	if err != nil {
		return reflect.Value{}, fmt.Errorf("failed to convert %s to %s, incorrect JSON: %w", s, t.String(), err)
	}
	return fromTree(tree, t, opts)
}

func fromTree(v any, t reflect.Type, opts Options) (reflect.Value, error) {
	if v == nil {
		return reflect.Zero(t), nil
	}
	if t.Kind() == reflect.Ptr {
		elem, err := fromTree(v, t.Elem(), opts)
		if err != nil {
			return reflect.Value{}, err
		}
		ptr := reflect.New(t.Elem())
		ptr.Elem().Set(elem)
		return ptr, nil
	}
	valOf := reflect.ValueOf(v)
	if t.Kind() == reflect.Interface && valOf.Type().Implements(t) {
		val := reflect.New(t).Elem()
		val.Set(valOf)
		return val, nil
	}
	isSequence := valOf.Kind() == reflect.Slice && valOf.Type() != typeOfBytes
	isMap := valOf.Kind() == reflect.Map
	if !isSequence && !isMap {
		if valOf.Type() == t {
			return valOf, nil
		}
		return fromString(toString(v), t, opts)
	}
	if isScalar(t) && t != typeOfBytes {
		return reflect.Value{}, fmt.Errorf("failed to convert %s to %s, single value expected", valOf.Kind(), t.String())
	}
	switch {
	case isSequence && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array):
		return sequence(t, valOf.Len(), func(i int) (reflect.Value, error) {
			return fromTree(valOf.Index(i).Interface(), t.Elem(), opts)
		})
	case isMap && t.Kind() == reflect.Map:
		val := reflect.MakeMapWithSize(t, valOf.Len())
		iter := valOf.MapRange()
		for iter.Next() {
			key, err := fromTree(iter.Key().Interface(), t.Key(), opts)
			if err != nil {
				return reflect.Value{}, err
			}
			elem, err := fromTree(iter.Value().Interface(), t.Elem(), opts)
			if err != nil {
				return reflect.Value{}, err
			}
			val.SetMapIndex(key, elem)
		}
		return val, nil
	case isMap && t.Kind() == reflect.Struct:
		val := reflect.New(t).Elem()
		if err := structFromMap(valOf, val, opts); err != nil {
			return reflect.Value{}, err
		}
		return val, nil
	}
	return reflect.Value{}, fmt.Errorf("failed to convert %s to %s", valOf.Kind(), t.String())
}

func structFromMap(m reflect.Value, val reflect.Value, opts Options) error {
	t := val.Type()
	for i := 0; i < t.NumField(); i++ {
		fld := t.Field(i)
		name, hasName := "", false
		if tagVal, ok := fld.Tag.Lookup(opts.Tag); ok {
			name = strings.Split(tagVal, ",")[0]
			hasName = name != ""
		}
		if name == "-" {
			continue
		}
		if fld.Anonymous && !hasName && !IsLeaf(fld.Type) {
			// fields of embedded structs are on the same level
			embedded := val.Field(i)
			if fld.Type.Kind() == reflect.Ptr {
				if fld.Type.Elem().Kind() == reflect.Ptr || !embedded.CanSet() {
					continue
				}
				embedded.Set(reflect.New(fld.Type.Elem()))
				embedded = embedded.Elem()
			}
			if err := structFromMap(m, embedded, opts); err != nil {
				return err
			}
			continue
		}
		if !val.Field(i).CanSet() {
			continue
		}
		item, ok := mapValue(m, name, hasName, fld.Name)
		if !ok {
			continue
		}
		fieldVal, err := fromTree(item, fld.Type, opts)
		if err != nil {
			return fmt.Errorf("field %s: %w", fld.Name, err)
		}
		val.Field(i).Set(fieldVal)
	}
	return nil
}

func mapValue(m reflect.Value, name string, hasName bool, fieldName string) (any, bool) {
	iter := m.MapRange()
	for iter.Next() {
		key := toString(iter.Key().Interface())
		if (hasName && key == name) || (!hasName && strings.EqualFold(key, fieldName)) {
			return iter.Value().Interface(), true
		}
	}
	return nil, false
}

// sequence creates slice or array with type t from n items.
func sequence(t reflect.Type, n int, item func(i int) (reflect.Value, error)) (reflect.Value, error) {
	var val reflect.Value
	if t.Kind() == reflect.Array {
		if n != t.Len() {
			return reflect.Value{}, fmt.Errorf("failed to convert to %s, expected %d items, got %d", t.String(), t.Len(), n)
		}
		val = reflect.New(t).Elem()
	} else {
		val = reflect.MakeSlice(t, n, n)
	}
	for i := 0; i < n; i++ {
		elem, err := item(i)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("item %d: %w", i, err)
		}
		val.Index(i).Set(elem)
	}
	return val, nil
}

func indirect(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case time.Time:
		return val.Format(time.RFC3339Nano)
	case fmt.Stringer:
		return val.String()
	}
	return fmt.Sprintf("%v", v)
}

func isJSON(s string, open byte) bool {
	s = strings.TrimSpace(s)
	return s != "" && s[0] == open
}

func unquote(s string) string {
	if len(s) > 1 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// split splits string to items by separator.
// Item or map value can be quoted, separators inside of quotes are ignored.
// Whitespaces around items are removed, quotes are kept.
func split(s, sep, kvSep string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var items []string
	var quote byte
	start, tokenStart := 0, true
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case tokenStart && (c == '"' || c == '\''):
			quote = c
			tokenStart = false
		case strings.HasPrefix(s[i:], sep):
			items = append(items, strings.TrimSpace(s[start:i]))
			i += len(sep) - 1
			start = i + 1
			tokenStart = true
		case kvSep != "" && strings.HasPrefix(s[i:], kvSep):
			i += len(kvSep) - 1
			tokenStart = true
		case c != ' ' && c != '\t':
			tokenStart = false
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("failed to split %s, quote %c is not closed", s, quote)
	}
	return append(items, strings.TrimSpace(s[start:])), nil
}

// splitKeyValue splits map item to key and value, quotes around key and value are removed.
func splitKeyValue(item, kvSep string) (string, string, bool) {
	idx := -1
	if item != "" && (item[0] == '"' || item[0] == '\'') {
		if end := strings.IndexByte(item[1:], item[0]); end >= 0 {
			// skip separators inside of quoted key
			if sepIdx := strings.Index(item[end+2:], kvSep); sepIdx >= 0 {
				idx = end + 2 + sepIdx
			}
		}
	} else {
		idx = strings.Index(item, kvSep)
	}
	if idx < 0 {
		return "", "", false
	}
	return unquote(strings.TrimSpace(item[:idx])), unquote(strings.TrimSpace(item[idx+len(kvSep):])), true
}

// Merge merges the next value of the field with the previous one: slices are appended, maps get the keys of the next map,
// pointers to slices and maps are merged by their values. []byte and all other values are replaced by the next one.
// The previous value is not changed, the merged value is a new slice or map.
func Merge(prev, next any) any {
	prevOf, nextOf := reflect.ValueOf(prev), reflect.ValueOf(next)
	if !prevOf.IsValid() || !nextOf.IsValid() || prevOf.Type() != nextOf.Type() {
		return next
	}
	isPtr := nextOf.Kind() == reflect.Ptr
	if isPtr {
		if prevOf.IsNil() || nextOf.IsNil() {
			return next
		}
		prevOf, nextOf = prevOf.Elem(), nextOf.Elem()
	}
	var merged reflect.Value
	switch {
	case nextOf.Kind() == reflect.Slice && nextOf.Type().Elem().Kind() != reflect.Uint8:
		merged = reflect.MakeSlice(nextOf.Type(), 0, prevOf.Len()+nextOf.Len())
		merged = reflect.AppendSlice(reflect.AppendSlice(merged, prevOf), nextOf)
	case nextOf.Kind() == reflect.Map:
		merged = reflect.MakeMapWithSize(nextOf.Type(), prevOf.Len()+nextOf.Len())
		for _, m := range []reflect.Value{prevOf, nextOf} {
			iter := m.MapRange()
			for iter.Next() {
				merged.SetMapIndex(iter.Key(), iter.Value())
			}
		}
	default:
		return next
	}
	if isPtr {
		ptr := reflect.New(merged.Type())
		ptr.Elem().Set(merged)
		return ptr.Interface()
	}
	return merged.Interface()
}
