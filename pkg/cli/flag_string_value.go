package cli

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/errors"
)

// -- string Value
type stringValue struct {
	value      *string
	hasBeenSet bool
}

func newStringValue(val string, p *string) *stringValue {
	*p = val
	return &stringValue{value: p}
}

func (val *stringValue) Set(str string) error {
	if val.hasBeenSet {
		err := fmt.Errorf("set more than once")
		return errors.WithStackTrace(err)
	}
	val.hasBeenSet = true

	*val.value = str
	return nil
}

func (val *stringValue) Get() any { return string(*val.value) }

func (val *stringValue) String() string { return string(*val.value) }
