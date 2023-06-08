package cli

import (
	"fmt"
	"strconv"

	"github.com/gruntwork-io/terragrunt/errors"
)

// -- int Value
type intValue struct {
	value      *int
	hasBeenSet bool
}

func newIntValue(val int, p *int) *intValue {
	*p = val
	return &intValue{value: p}
}

func (val *intValue) Set(strVal string) error {
	if val.hasBeenSet {
		err := fmt.Errorf("set more than once")
		return errors.WithStackTrace(err)
	}
	val.hasBeenSet = true

	value, err := strconv.Atoi(strVal)
	if err != nil {
		err = fmt.Errorf("error parse: %w", err)
		return errors.WithStackTrace(err)
	}

	*val.value = value
	return nil
}

func (val *intValue) Get() any { return int(*val.value) }

func (val *intValue) String() string { return strconv.Itoa(int(*val.value)) }
