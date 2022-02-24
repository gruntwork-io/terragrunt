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
//   nil will be returned if the path is invalid
//
//   Using an example terraform output:
//     a = {
//       b = {
//         c = "foo"
//       }
//       "d" = [
//         1,
//         2
//       ]
//     }
//
//   path ["a", "b", "c"] will return "foo"
//   path ["a", "d", "1"] will return 2
//   path ["a", "foo"] will return nil
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
