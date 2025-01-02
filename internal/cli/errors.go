package cli

import (
	"errors"
	"fmt"

	"github.com/urfave/cli/v2"
)

type InvalidCommandNameError string

func (cmdName InvalidCommandNameError) Error() string {
	return fmt.Sprintf("invalid command name %q", string(cmdName))
}

type InvalidKeyValueError struct {
	value string
	sep   string
}

func NewInvalidKeyValueError(sep, value string) *InvalidKeyValueError {
	return &InvalidKeyValueError{value, sep}
}

func (err InvalidKeyValueError) Error() string {
	return fmt.Sprintf("invalid key-value pair, expected format KEY%sVALUE, got %s.", err.sep, err.value)
}

type exitError struct {
	exitCode int
	err      error
}

func (ee *exitError) Unwrap() error {
	return ee.err
}

func (ee *exitError) Error() string {
	if ee.err == nil {
		return ""
	}

	return ee.err.Error()
}

func (ee *exitError) ExitCode() int {
	return ee.exitCode
}

// ExitCoder is the interface checked by `App` and `Command` for a custom exit code
type ExitCoder interface {
	error
	ExitCode() int
	Unwrap() error
}

// NewExitError calls Exit to create a new ExitCoder.
func NewExitError(message interface{}, exitCode int) ExitCoder {
	var err error

	if message != nil {
		switch e := message.(type) {
		case error:
			err = e
		default:
			err = fmt.Errorf("%+v", message)
		}
	}

	return &exitError{
		err:      err,
		exitCode: exitCode,
	}
}

// handleExitCoder handles errors implementing ExitCoder by printing their
// message and calling osExiter with the given exit code.
//
// If the given error instead implements MultiError, each error will be checked
// for the ExitCoder interface, and osExiter will be called with the last exit
// code found, or exit code 1 if no ExitCoder is found.
//
// This function is the default error-handling behavior for an App.
func handleExitCoder(_ *Context, err error, osExiter func(code int)) error {
	if err == nil {
		return nil
	}

	var exitErr cli.ExitCoder
	if ok := errors.As(err, &exitErr); ok {
		if err.Error() != "" {
			_, _ = fmt.Fprintln(cli.ErrWriter, err)
		}

		osExiter(exitErr.ExitCode())

		return nil
	}

	return err
}

// InvalidValueError is used to wrap errors from `strconv` to make the error message more user friendly.
type InvalidValueError struct {
	underlyingError error
	msg             string
}

func (err InvalidValueError) Error() string {
	return err.msg
}

func (err InvalidValueError) Unwrap() error {
	return err.underlyingError
}
