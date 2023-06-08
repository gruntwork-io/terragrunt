package cli

import "strings"

// -- string val Value
type stringSliceValue struct {
	val        *[]string
	sep        string
	hasBeenSet bool
}

func newStringSliceValue(val []string, p *[]string, sep string) *stringSliceValue {
	*p = val
	return &stringSliceValue{
		val: p,
		sep: sep,
	}
}

func (val *stringSliceValue) Set(str string) error {
	if !val.hasBeenSet {
		val.hasBeenSet = true
		*val.val = []string{}
	}

	*val.val = append(*val.val, str)
	return nil
}

func (val *stringSliceValue) Get() any { return []string(*val.val) }

func (val *stringSliceValue) String() string { return strings.Join(*val.val, val.sep) }
