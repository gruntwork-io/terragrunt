package placeholders

import (
	"fmt"
	"strings"
)

// EmptyPlaceholderNameError is an empty `placeholder` name error.
type EmptyPlaceholderNameError struct {
	str string
}

// NewEmptyPlaceholderNameError returns a new `EmptyPlaceholderNameError` instance.
func NewEmptyPlaceholderNameError(str string) *EmptyPlaceholderNameError {
	return &EmptyPlaceholderNameError{
		str: str,
	}
}

func (err EmptyPlaceholderNameError) Error() string {
	return fmt.Sprintf("empty placeholder name %q", err.str)
}

// InvalidPlaceholderNameError is an invalid `placeholder` name error.
type InvalidPlaceholderNameError struct {
	name string
	opts Placeholders
}

// NewInvalidPlaceholderNameError returns a new `InvalidPlaceholderNameError` instance.
func NewInvalidPlaceholderNameError(name string, opts Placeholders) *InvalidPlaceholderNameError {
	return &InvalidPlaceholderNameError{
		name: name,
		opts: opts,
	}
}

func (err InvalidPlaceholderNameError) Error() string {
	return fmt.Sprintf("invalid placeholder name %q, available names: %s", err.name, strings.Join(err.opts.Names(), ","))
}

// InvalidPlaceholderOptionError is an invalid `placeholder` option error.
type InvalidPlaceholderOptionError struct {
	ph  Placeholder
	err error
}

// NewInvalidPlaceholderOptionError returns a new `InvalidPlaceholderOptionError` instance.
func NewInvalidPlaceholderOptionError(ph Placeholder, err error) *InvalidPlaceholderOptionError {
	return &InvalidPlaceholderOptionError{
		ph:  ph,
		err: err,
	}
}

func (err InvalidPlaceholderOptionError) Error() string {
	return fmt.Sprintf("placeholder %q, %v", err.ph.Name(), err.err)
}

func (err InvalidPlaceholderOptionError) Unwrap() error {
	return err.err
}
