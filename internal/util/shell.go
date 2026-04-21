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
	"github.com/gruntwork-io/terragrunt/internal/vexec"
)

// IsCommandExecutable returns true if the command can be run to completion
// without error via the given vexec.Exec.
func IsCommandExecutable(e vexec.Exec, ctx context.Context, command string, args ...string) bool {
	return vexec.Run(e, ctx, command, args...) == nil
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
	Command         string
	Args            []string
	Output          CmdOutput
	LogShowAbsPaths bool
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
