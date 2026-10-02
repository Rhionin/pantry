package server

import (
	"reflect"
	"strings"

	"github.com/Rhionin/pantry/internal/product"
)

// scrubImageURLs clears imageUrl fields a browser should not load. It walks
// the value that is about to be encoded so every JSON response is covered,
// including rows that were stored before this check existed.
func scrubImageURLs(data any) any {
	if data == nil {
		return nil
	}
	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.Struct, reflect.Slice, reflect.Array:
		ptr := reflect.New(v.Type())
		ptr.Elem().Set(v)
		scrubValue(ptr)
		return ptr.Interface()
	default:
		scrubValue(v)
		return data
	}
}

func scrubValue(v reflect.Value) {
	if !v.IsValid() {
		return
	}
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			field := v.Field(i)
			name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
			if name == "imageUrl" && field.Kind() == reflect.String && field.CanSet() {
				field.SetString(product.SafeImageURL(field.String()))
				continue
			}
			scrubValue(field)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			scrubValue(v.Index(i))
		}
	}
}
