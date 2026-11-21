// Package valuesblock decodes the values block of a unit or stack config and resolves the values a component is given
// against it.
//
// A values block declares each value the component accepts with a value block, which may set a type constraint, a
// default and a description:
//
//	values {
//	  value "tags" {
//	    type    = map(string)
//	    default = {}
//	  }
//	}
//
// Resolution rejects keys the block does not declare, converts each supplied value to its type, fills in defaults
// and reports required values that nothing sets.
package valuesblock
