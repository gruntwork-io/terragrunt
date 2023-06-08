package cli

import (
	"strconv"

	"github.com/gruntwork-io/terragrunt/errors"
)

// -- int64 Value
type int64Value int64

func newInt64Value(val int64, p *int64) *int64Value {
	*p = val
	return (*int64Value)(p)
}

func (i *int64Value) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		return errors.Errorf("error parse: %w", err)
	}
	*i = int64Value(v)
	return nil
}

func (i *int64Value) Get() any { return int64(*i) }

func (i *int64Value) String() string { return strconv.FormatInt(int64(*i), 10) }
