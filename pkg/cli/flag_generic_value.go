package cli

import (
	"flag"

	"github.com/gruntwork-io/terragrunt/errors"
)

// -- int Value
type genericValue[T flag.Getter] struct {
	value      T
	hasBeenSet bool
}

func newGenreicValue[T flag.Getter](value T) flag.Getter {
	return &genericValue[T]{
		value: value,
	}
}

func (val *genericValue[T]) Set(str string) error {
	if val.hasBeenSet {
		return errors.Errorf("set more than once")
	}
	val.hasBeenSet = true

	return val.value.Set(str)
}

func (val *genericValue[T]) Get() any {
	return val.value.Get()
}

func (val *genericValue[T]) String() string {
	return val.value.String()
}
