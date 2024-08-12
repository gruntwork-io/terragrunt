package util

import (
	goErrors "errors"
	"fmt"
	"os/exec"
	"syscall"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/go-multierror"
)

// IsCommandExecutable - returns true if a command can be executed without errors.
func IsCommandExecutable(command string, args ...string) bool {
	cmd := exec.Command(command, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if ok := goErrors.As(err, &exitErr); ok {
			return exitErr.ExitCode() == 0
		}
		return false
	}
	return true
}

type CmdOutput struct {
	Stdout string
	Stderr string
}

// Return the exit code of a command. If the error does not implement iErrorCode or is not an exec.ExitError
// or *multierror.Error type, the error is returned.
func GetExitCode(err error) (int, error) {
	// Interface to determine if we can retrieve an exit status from an error
	type iErrorCode interface {
		ExitStatus() (int, error)
	}

	if exiterr, ok := errors.Unwrap(err).(iErrorCode); ok {
		return exiterr.ExitStatus()
	}

	var exiterr *exec.ExitError
	if ok := goErrors.As(err, &exiterr); ok {
		status := exiterr.Sys().(syscall.WaitStatus)
		return status.ExitStatus(), nil
	}

	var multiErr *multierror.Error
	if ok := goErrors.As(err, &multiErr); ok {
		for _, err := range multiErr.Errors {
			exitCode, exitCodeErr := GetExitCode(err)
			if exitCodeErr == nil {
				return exitCode, nil
			}
		}
	}

	return 0, err
}

// ProcessExecutionError - error returned when a command fails, contains StdOut and StdErr
type ProcessExecutionError struct {
	Err        error
	StdOut     string
	Stderr     string
	WorkingDir string
}

func (err ProcessExecutionError) Error() string {
	// Include in error message the working directory where the command was run, so it's easier for the user to
	return fmt.Sprintf("[%s] %s", err.WorkingDir, err.Err.Error())
}

func (err ProcessExecutionError) ExitStatus() (int, error) {
	return GetExitCode(err.Err)
}
