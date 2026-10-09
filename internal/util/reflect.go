package util

import (
	"reflect"
	"strconv"
)

// KindOf returns the kind of the type or Invalid if value is nil.
func KindOf(value any) reflect.Kind {
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
func MustWalkTerraformOutput(value any, path ...string) any {
	found := value
	for _, p := range path {
		if found == nil {
			return nil
		}

		v := reflect.ValueOf(found)

		switch v.Kind() { //nolint:exhaustive // only the kinds a path can descend into matter; reflect.Kind has 26 members
		case reflect.Map:
			key := reflect.ValueOf(p)
			if !key.Type().AssignableTo(v.Type().Key()) {
				return nil
			}

			elem := v.MapIndex(key)
			if !elem.IsValid() {
				return nil
			}

			found = elem.Interface()

		case reflect.Slice, reflect.Array:
			i, err := strconv.Atoi(p)
			if err != nil {
				return nil
			}

			if i < 0 || i >= v.Len() {
				return nil
			}

			found = v.Index(i).Interface()

		default:
			return found
		}
	}

	return found
}
