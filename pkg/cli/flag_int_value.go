package cli

import (
	"strconv"

	"github.com/gruntwork-io/terragrunt/errors"
)

// -- int Value
type intValue int

func newIntValue(val int, p *int) *intValue {
	*p = val
	return (*intValue)(p)
}

func (i *intValue) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, strconv.IntSize)
	if err != nil {
		return errors.Errorf("error parse: %w", err)
	}
	*i = intValue(v)
	return nil
}

func (i *intValue) Get() any { return int(*i) }

func (i *intValue) String() string { return strconv.Itoa(int(*i)) }
