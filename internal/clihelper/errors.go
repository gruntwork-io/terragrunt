package clihelper

import (
	"fmt"
	"strings"

	"errors"

	"github.com/urfave/cli/v2"
)

// InvalidCommandNameError reports a command name that matches no command.
type InvalidCommandNameError string

func (cmdName InvalidCommandNameError) Error() string {
	return fmt.Sprintf("invalid command name %q", string(cmdName))
}

// InvalidKeyValueError reports a map flag entry that does not split into a key and a value.
type InvalidKeyValueError struct {
	value string
	sep   string
}

// NewInvalidKeyValueError returns an [InvalidKeyValueError] for value, which did not split on sep.
func NewInvalidKeyValueError(sep, value string) *InvalidKeyValueError {
	return &InvalidKeyValueError{value, sep}
}

func (err InvalidKeyValueError) Error() string {
	return fmt.Sprintf(
		"invalid key-value pair, expected format KEY%sVALUE, got %s.",
		err.sep,
		err.value,
	)
}

type exitError struct {
	err      error
	exitCode ExitCode
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
	return int(ee.exitCode)
}

// NewExitError calls Exit to create a new ExitCoder.
func NewExitError(message any, exitCode ExitCode) ExitCoder {
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
// This function is the default error-handling behavior for an App.
func handleExitCoder(_ *Context, err error, osExiter func(code int)) error {
	if err == nil {
		return nil
	}

	var exitErr cli.ExitCoder
	if ok := errors.As(err, &exitErr); ok {
		var writeErr error
		if err.Error() != "" {
			_, writeErr = fmt.Fprintln(cli.ErrWriter, err)
		}

		osExiter(exitErr.ExitCode())

		return writeErr
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

// ErrMsgFlagUndefined is the prefix of the error the standard flag package returns for an
// undefined flag.
const ErrMsgFlagUndefined = "flag provided but not defined:"

// UndefinedFlagError reports a flag that no flag in scope defines. Its value is the flag name
// without dashes.
type UndefinedFlagError string

func (flag UndefinedFlagError) Error() string {
	return ErrMsgFlagUndefined + " -" + string(flag)
}

var (
	// ErrMultipleTimesSettingFlag is returned when a single-value flag appears more than once on the
	// command line.
	ErrMultipleTimesSettingFlag = errors.New("setting the flag multiple times")
	// ErrMultipleTimesSettingEnvVar is returned when more than one env var sets a single-value flag.
	ErrMultipleTimesSettingEnvVar = errors.New("setting the env var multiple times")
)

// IsMultipleTimesSettingError reports whether the message of err contains the message of
// [ErrMultipleTimesSettingFlag] or [ErrMultipleTimesSettingEnvVar].
//
// It matches on the message because the standard flag package wraps setter errors with %v,
// which drops the error chain.
func IsMultipleTimesSettingError(err error) bool {
	return strings.Contains(err.Error(), ErrMultipleTimesSettingFlag.Error()) ||
		strings.Contains(err.Error(), ErrMultipleTimesSettingEnvVar.Error())
}
