package cli

import "strings"

// -- string slice Value
type stringSliceValue struct {
	value      *[]string
	sep        string
	hasBeenSet bool
}

func newStringSliceValue(val []string, p *[]string, sep string) *stringSliceValue {
	*p = val
	return &stringSliceValue{
		value: p,
		sep:   sep,
	}
}

func (val *stringSliceValue) Set(str string) error {
	if !val.hasBeenSet {
		val.hasBeenSet = true
		*val.value = []string{}
	}

	*val.value = append(*val.value, str)
	return nil
}

func (val *stringSliceValue) Get() any { return []string(*val.value) }

func (val *stringSliceValue) String() string { return strings.Join(*val.value, val.sep) }
