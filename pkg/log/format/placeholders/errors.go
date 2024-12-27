package placeholders

import (
	"fmt"
	"strings"
)

// InvalidPlaceholderNameError is an invalid `placeholder` name error.
type InvalidPlaceholderNameError struct {
	str  string
	opts Placeholders
}

// NewInvalidPlaceholderNameError returns a new `InvalidPlaceholderNameError` instance.
func NewInvalidPlaceholderNameError(str string, opts Placeholders) *InvalidPlaceholderNameError {
	return &InvalidPlaceholderNameError{
		str:  str,
		opts: opts,
	}
}

func (err InvalidPlaceholderNameError) Error() string {
	var name string

	for index := range len(err.str) {
		if !isPlaceholderNameCharacter(err.str[index]) {
			break
		}

		name = err.str[:index+1]
	}

	return fmt.Sprintf("invalid placeholder name %q, available names: %s", name, strings.Join(err.opts.Names(), ","))
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
