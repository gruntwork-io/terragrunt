package cli

import (
	"strconv"

	"github.com/gruntwork-io/terragrunt/errors"
)

// -- bool Value
type boolValue struct {
	value      *bool
	hasBeenSet bool
}

func newBoolValue(val bool, p *bool) *boolValue {
	*p = val
	return &boolValue{value: p}
}

func (val *boolValue) Set(strVal string) error {
	if val.hasBeenSet {
		return errors.Errorf("set more than once")
	}
	val.hasBeenSet = true

	value, err := strconv.ParseBool(strVal)
	if err != nil {
		return errors.Errorf("error parse: %w", err)
	}

	*val.value = value
	return nil
}

func (val *boolValue) Get() any { return bool(*val.value) }

func (val *boolValue) String() string { return strconv.FormatBool(bool(*val.value)) }

func (val *boolValue) IsBoolFlag() bool { return true }
