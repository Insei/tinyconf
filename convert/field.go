package convert

import (
	"reflect"
	"strings"
	"unsafe"

	"github.com/insei/fmap/v3"
)

const (
	// SeparatorTag is a struct tag that overrides separator of slice, array and map items for the field.
	SeparatorTag = "separator"
	// KeyValueSeparatorTag is a struct tag that overrides separator of map key and value for the field.
	KeyValueSeparatorTag = "kvseparator"
)

func fieldOptions(field fmap.Field, tag string) Options {
	return Options{
		Separator:         field.GetTag().Get(SeparatorTag),
		KeyValueSeparator: field.GetTag().Get(KeyValueSeparatorTag),
		Tag:               tag,
	}
}

// StringToField converts string value to the value with type of the field.
// Separators can be overridden with SeparatorTag and KeyValueSeparatorTag tags of the field.
func StringToField(s string, field fmap.Field) (any, error) {
	return FromString(s, field.GetType(), fieldOptions(field, ""))
}

// TreeToField converts value decoded from yaml, json or similar formats to the value with type of the field.
// Tag is a struct tag with key names for structs nested into slices and maps.
func TreeToField(v any, field fmap.Field, tag string) (any, error) {
	return FromTree(v, field.GetType(), fieldOptions(field, tag))
}

// fieldValueOf returns addressable and settable value of the field in the obj.
// Value is found by the struct path, because offsets of the nested struct fields in fmap are relative to the parent.
func fieldValueOf(field fmap.Field, obj any) reflect.Value {
	val := reflect.ValueOf(obj)
	for val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if !val.CanAddr() {
		// obj is not a pointer, value can be only read from the copy
		addressable := reflect.New(val.Type()).Elem()
		addressable.Set(val)
		val = addressable
	}
	for _, name := range strings.Split(field.GetStructPath(), ".") {
		val = val.FieldByName(name)
	}
	// allows to work with unexported fields
	return reflect.NewAt(val.Type(), unsafe.Pointer(val.UnsafeAddr())).Elem()
}

// GetFieldValue returns field value from the obj with exact field type.
func GetFieldValue(field fmap.Field, obj any) any {
	return fieldValueOf(field, obj).Interface()
}

// SetFieldValue sets field value in the obj, val should have exact field type, obj should be a pointer to struct.
func SetFieldValue(field fmap.Field, obj any, val any) {
	dest := fieldValueOf(field, obj)
	if val == nil {
		dest.Set(reflect.Zero(dest.Type()))
		return
	}
	dest.Set(reflect.ValueOf(val))
}

// IsInsideValue reports whether the field is a part of the value that is parsed as a whole:
// field of the struct stored by pointer or field of the type that is parsed from a single value, time.Time for example.
// Such fields are not requested from drivers and not documented.
func IsInsideValue(storage fmap.Storage, field fmap.Field) bool {
	path := field.GetStructPath()
	for i := strings.IndexByte(path, '.'); i >= 0; {
		if parent, ok := storage.Find(path[:i]); ok {
			// pointers to structs are parsed as a single value, even if fmap goes into them
			if parent.GetType().Kind() == reflect.Ptr || IsLeaf(parent.GetType()) {
				return true
			}
		}
		next := strings.IndexByte(path[i+1:], '.')
		if next < 0 {
			break
		}
		i += next + 1
	}
	return false
}

// IsValueField reports whether the field value should be requested from drivers.
// Structs are parsed by their fields, except the types that are parsed from a single value, time.Time for example.
// Fields of such types are not value fields.
func IsValueField(storage fmap.Storage, field fmap.Field) bool {
	if IsInsideValue(storage, field) {
		return false
	}
	// pointers to structs are parsed as a single value too
	return field.GetType().Kind() != reflect.Struct || IsLeaf(field.GetType())
}
