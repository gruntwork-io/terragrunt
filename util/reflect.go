package util

import (
	"reflect"
	"strconv"
)

// Return the kind of the type or Invalid if value is nil
func KindOf(value interface{}) reflect.Kind {
	valueType := reflect.TypeOf(value)
	if valueType == nil {
		return reflect.Invalid
	}
	return valueType.Kind()
}

// MustWalkTerraformOutput is a helper utility to deeply return a value from a terraform output.
//
//	nil will be returned if the path is invalid
//
//	Using an example terraform output:
//	  a = {
//	    b = {
//	      c = "foo"
//	    }
//	    "d" = [
//	      1,
//	      2
//	    ]
//	  }
//
//	path ["a", "b", "c"] will return "foo"
//	path ["a", "d", "1"] will return 2
//	path ["a", "foo"] will return nil
func MustWalkTerraformOutput(value interface{}, path ...string) interface{} {
	if value == nil {
		return nil
	}
	found := value
	for _, p := range path {
		v := reflect.ValueOf(found)
		switch reflect.TypeOf(found).Kind() {
		case reflect.Map:
			if !v.MapIndex(reflect.ValueOf(p)).IsValid() {
				return nil
			}
			found = v.MapIndex(reflect.ValueOf(p)).Interface()

		case reflect.Slice, reflect.Array:
			i, err := strconv.Atoi(p)
			if err != nil {
				return nil
			}
			if v.Len()-1 < i {
				return nil
			}
			found = v.Index(i).Interface()

		default:
			return found
		}
	}
	return found
}

// DeepCopy performs a deep copy of a struct and returns a new struct instance
func DeepCopy(src interface{}) interface{} {
	if src == nil {
		return nil
	}

	srcVal := reflect.ValueOf(src)
	srcType := srcVal.Type()

	// Ensure the src is a struct or pointer to a struct
	if srcType.Kind() == reflect.Ptr {
		srcType = srcType.Elem()
		srcVal = srcVal.Elem()
	}
	if srcType.Kind() != reflect.Struct {
		return src
	}

	// Create a new instance of the struct
	dstVal := reflect.New(srcType).Elem()

	// Copy all fields
	for i := 0; i < srcVal.NumField(); i++ {
		fieldVal := srcVal.Field(i)
		if !fieldVal.CanSet() {
			continue
		}

		dstField := dstVal.Field(i)
		copyValue(dstField, fieldVal)
	}

	// Return the new struct instance
	return dstVal.Interface()
}

// copyValue performs a deep copy of a value from src to dst
func copyValue(dst, src reflect.Value) {
	if !dst.CanSet() {
		return
	}

	switch src.Kind() {
	case reflect.Ptr:
		if !src.IsNil() {
			srcVal := src.Elem()
			dst.Set(reflect.New(srcVal.Type()))
			copyValue(dst.Elem(), srcVal)
		}

	case reflect.Struct:
		dst.Set(reflect.New(src.Type()).Elem())
		for i := 0; i < src.NumField(); i++ {
			copyValue(dst.Field(i), src.Field(i))
		}

	case reflect.Slice:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.MakeSlice(src.Type(), src.Len(), src.Cap()))
		for i := 0; i < src.Len(); i++ {
			copyValue(dst.Index(i), src.Index(i))
		}

	case reflect.Map:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.MakeMap(src.Type()))
		for _, key := range src.MapKeys() {
			newVal := reflect.New(src.MapIndex(key).Type()).Elem()
			copyValue(newVal, src.MapIndex(key))
			dst.SetMapIndex(key, newVal)
		}

	default:
		dst.Set(src)
	}
}
