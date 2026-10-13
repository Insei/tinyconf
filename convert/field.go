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
// Nil pointers to the structs in the path are replaced with new structs if create is true, otherwise false is returned.
func fieldValueOf(field fmap.Field, obj any, create bool) (reflect.Value, bool) {
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
		if val.Kind() == reflect.Ptr {
			// struct stored by pointer
			if val.IsNil() {
				if !create {
					return reflect.Value{}, false
				}
				val.Set(reflect.New(val.Type().Elem()))
			}
			val = val.Elem()
		}
		val = val.FieldByName(name)
		// allows to work with unexported fields
		val = reflect.NewAt(val.Type(), unsafe.Pointer(val.UnsafeAddr())).Elem()
	}
	return val, true
}

// GetFieldValue returns field value from the obj with exact field type.
// Zero value is returned for the field of the struct stored by pointer, if that pointer is nil.
func GetFieldValue(field fmap.Field, obj any) any {
	val, ok := fieldValueOf(field, obj, false)
	if !ok {
		return reflect.Zero(field.GetType()).Interface()
	}
	return val.Interface()
}

// SetFieldValue sets field value in the obj, val should have exact field type, obj should be a pointer to struct.
// Nil pointers to the structs in the path of the field are replaced with new structs.
func SetFieldValue(field fmap.Field, obj any, val any) {
	dest, _ := fieldValueOf(field, obj, true)
	if val == nil {
		dest.Set(reflect.Zero(dest.Type()))
		return
	}
	dest.Set(reflect.ValueOf(val))
}

// hasNestedFields reports whether the storage has fields of the struct stored in the field by value or by pointer.
// fmap does not go into recursive types and pointers to pointers.
func hasNestedFields(storage fmap.Storage, field fmap.Field) bool {
	t := field.GetType()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t.NumField() == 0 {
		return false
	}
	_, ok := storage.Find(field.GetStructPath() + "." + t.Field(0).Name)
	return ok
}

// isSection reports whether the field is a struct that is parsed by its fields.
func isSection(storage fmap.Storage, field fmap.Field) bool {
	t := field.GetType()
	if IsLeaf(t) {
		return false
	}
	// struct without nested fields in the storage can be parsed only as a single value
	return t.Kind() == reflect.Struct || hasNestedFields(storage, field)
}

// CopySections replaces structs stored by pointer and parsed by their fields with their copies,
// so the fields of the obj can be set without changing the object it was copied from.
func CopySections(storage fmap.Storage, obj any) {
	// parents are always before their fields
	for _, path := range storage.GetAllPaths() {
		field := storage.MustFind(path)
		if field.GetType().Kind() != reflect.Ptr || !isSection(storage, field) || IsInsideValue(storage, field) {
			continue
		}
		val, ok := fieldValueOf(field, obj, false)
		if !ok || val.IsNil() {
			continue
		}
		copied := reflect.New(val.Type().Elem())
		copied.Elem().Set(val.Elem())
		val.Set(copied)
	}
}

// IsInsideValue reports whether the field is a part of the value that is parsed as a whole,
// time.Time for example. Such fields are not requested from drivers and not documented.
func IsInsideValue(storage fmap.Storage, field fmap.Field) bool {
	path := field.GetStructPath()
	for i := strings.IndexByte(path, '.'); i >= 0; {
		if parent, ok := storage.Find(path[:i]); ok && !isSection(storage, parent) {
			return true
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
// Structs, stored by value or by pointer, are parsed by their fields,
// except the types that are parsed from a single value, time.Time for example.
// Fields of such types are not value fields.
func IsValueField(storage fmap.Storage, field fmap.Field) bool {
	return !isSection(storage, field) && !IsInsideValue(storage, field)
}
