package util

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// IsCommandExecutable - returns true if a command can be executed without errors.
func IsCommandExecutable(ctx context.Context, command string, args ...string) bool {
	cmd := exec.CommandContext(ctx, command, args...)
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

	var exitCoder clihelper.ExitCoder
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
	Err             error
	WorkingDir      string
	RootWorkingDir  string
	LogShowAbsPaths bool
	Command         string
	Args            []string
	Output          CmdOutput
	DisableSummary  bool
}

func (err ProcessExecutionError) Error() string { //nolint:gocritic
	commandStr := strings.TrimSpace(
		strings.Join(append([]string{err.Command}, err.Args...), " "),
	)

	workingDirForLog := RelPathForLog(err.RootWorkingDir, err.WorkingDir, err.LogShowAbsPaths)

	if err.DisableSummary {
		return fmt.Sprintf("Failed to execute \"%s\" in %s",
			commandStr,
			workingDirForLog,
		)
	}

	return fmt.Sprintf("Failed to execute \"%s\" in %s\n%s\n%v",
		commandStr,
		workingDirForLog,
		err.Output.Stderr.String(),
		err.Err,
	)
}

func (err ProcessExecutionError) ExitStatus() (int, error) { //nolint:gocritic
	return GetExitCode(err.Err)
}

func (err ProcessExecutionError) Unwrap() error { //nolint:gocritic
	return err.Err
}
