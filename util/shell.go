package util

import (
	"bytes"
	"fmt"
	"strings"
	"syscall"

	"os/exec"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

// IsCommandExecutable - returns true if a command can be executed without errors.
func IsCommandExecutable(command string, args ...string) bool {
	cmd := exec.Command(command, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if ok := errors.As(err, &exitErr); ok {
			return exitErr.ExitCode() == 0
		}

		return false
	}

	return true
}

type CmdOutput struct {
	Stdout bytes.Buffer
	Stderr bytes.Buffer
}

// GetExitCode returns the exit code of a command. If the error does not
// implement errorCode or is not an exec.ExitError
// or *errors.MultiError type, the error is returned.
func GetExitCode(err error) (int, error) {
	var exitStatus interface {
		ExitStatus() (int, error)
	}

	if errors.As(err, &exitStatus) {
		return exitStatus.ExitStatus()
	}

	var exitCoder cli.ExitCoder

	if errors.As(err, &exitCoder) {
		return exitCoder.ExitCode(), nil
	}

	var exiterr *exec.ExitError
	if ok := errors.As(err, &exiterr); ok {
		status := exiterr.Sys().(syscall.WaitStatus)
		return status.ExitStatus(), nil
	}

	var multiErr *errors.MultiError
	if ok := errors.As(err, &multiErr); ok {
		for _, err := range multiErr.WrappedErrors() {
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
	Err            error
	Output         CmdOutput
	WorkingDir     string
	Command        string
	Args           []string
	DisableSummary bool
}

func (err ProcessExecutionError) Error() string {
	if err.DisableSummary {
		return fmt.Sprintf("Failed to execute \"%s %s\" in %s",
			err.Command,
			strings.Join(err.Args, " "),
			err.WorkingDir,
		)
	}

	return fmt.Sprintf("Failed to execute \"%s %s\" in %s\n%s\n%v",
		err.Command,
		strings.Join(err.Args, " "),
		err.WorkingDir,
		err.Output.Stderr.String(),
		err.Err,
	)
}

func (err ProcessExecutionError) ExitStatus() (int, error) {
	return GetExitCode(err.Err)
}

func (err ProcessExecutionError) Unwrap() error {
	return err.Err
}
