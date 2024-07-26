package util

import (
	"reflect"
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
