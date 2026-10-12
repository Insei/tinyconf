package convert

import (
	"reflect"

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

// GetFieldValue returns field value from the obj with exact field type.
func GetFieldValue(field fmap.Field, obj any) any {
	return reflect.ValueOf(field.GetPtr(obj)).Elem().Interface()
}

// SetFieldValue sets field value in the obj, val should have exact field type.
func SetFieldValue(field fmap.Field, obj any, val any) {
	dest := reflect.ValueOf(field.GetPtr(obj)).Elem()
	if val == nil {
		dest.Set(reflect.Zero(dest.Type()))
		return
	}
	dest.Set(reflect.ValueOf(val))
}
