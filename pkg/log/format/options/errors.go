package options

import (
	"fmt"
	"strings"
)

// InvalidOptionError is an invalid `option` syntax error.
type InvalidOptionError struct {
	str string
}

// NewInvalidOptionError returns a new `InvalidOptionError` instance.
func NewInvalidOptionError(str string) *InvalidOptionError {
	return &InvalidOptionError{
		str: str,
	}
}

func (err InvalidOptionError) Error() string {
	return fmt.Sprintf("invalid option syntax %q", err.str)
}

// EmptyOptionNameError is an empty `option` name error.
type EmptyOptionNameError struct {
	str string
}

// NewEmptyOptionNameError returns a new `EmptyOptionNameError` instance.
func NewEmptyOptionNameError(str string) *EmptyOptionNameError {
	return &EmptyOptionNameError{
		str: str,
	}
}

func (err EmptyOptionNameError) Error() string {
	return fmt.Sprintf("empty option name %q", err.str)
}

// InvalidOptionNameError is an invalid `option` name error.
type InvalidOptionNameError struct {
	name string
	opts Options
}

// NewInvalidOptionNameError returns a new `InvalidOptionNameError` instance.
func NewInvalidOptionNameError(name string, opts Options) *InvalidOptionNameError {
	return &InvalidOptionNameError{
		name: name,
		opts: opts,
	}
}

func (err InvalidOptionNameError) Error() string {
	return fmt.Sprintf("invalid option name %q, available names: %s", err.name, strings.Join(err.opts.Names(), ","))
}

// InvalidOptionValueError is an invalid `option` value error.
type InvalidOptionValueError struct {
	val string
	opt Option
	err error
}

// NewInvalidOptionValueError returns a new `InvalidOptionValueError` instance.
func NewInvalidOptionValueError(opt Option, val string, err error) *InvalidOptionValueError {
	return &InvalidOptionValueError{
		val: val,
		opt: opt,
		err: err,
	}
}

func (err InvalidOptionValueError) Error() string {
	return fmt.Sprintf("option %q, invalid value %q, %v", err.opt.Name(), err.val, err.err)
}

func (err InvalidOptionValueError) Unwrap() error {
	return err.err
}
