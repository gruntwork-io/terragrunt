package cli

import (
	"strconv"

	"github.com/gruntwork-io/terragrunt/errors"
)

// -- bool Value
type boolValue bool

func newBoolValue(val bool, p *bool) *boolValue {
	*p = val
	return (*boolValue)(p)
}

func (b *boolValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return errors.Errorf("error parse: %w", err)
	}
	*b = boolValue(v)
	return nil
}

func (b *boolValue) Get() any { return bool(*b) }

func (b *boolValue) String() string { return strconv.FormatBool(bool(*b)) }

func (b *boolValue) IsBoolFlag() bool { return true }
