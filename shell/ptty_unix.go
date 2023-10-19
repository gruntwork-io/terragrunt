//go:build !windows
// +build !windows

package shell

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/creack/pty"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

// runCommandWithPTTY will allocate a pseudo-tty to run the subcommand in. This is only necessary when running
// interactive commands, so that terminal features like readline work through the subcommand when stdin, stdout, and
// stderr is being shared.
// NOTE: This is based on the quickstart example from https://github.com/creack/pty
func runCommandWithPTTY(terragruntOptions *options.TerragruntOptions, cmd *exec.Cmd, cmdStdout io.Writer, cmdStderr io.Writer) (err error) {
	// NOTE: in order to ensure we can return errors that occur in cleanup, we use a variable binding for the return
	// value so that it can be updated.

	pseudoTerminal, startErr := pty.Start(cmd)
	defer func() {
		if closeErr := pseudoTerminal.Close(); closeErr != nil {
			terragruntOptions.Logger.Errorf("Error closing pty: %s", closeErr)
			// Only overwrite the previous error if there was no error since this error has lower priority than any
			// errors in the main routine
			if err == nil {
				err = errors.WithStackTrace(closeErr)
			}
		}
	}()
	if startErr != nil {
		return errors.WithStackTrace(startErr)
	}

	// Every time the current terminal size changes, we need to make sure the PTY also updates the size.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if inheritSizeErr := pty.InheritSize(os.Stdin, pseudoTerminal); inheritSizeErr != nil {
				terragruntOptions.Logger.Errorf("error resizing pty: %s", inheritSizeErr)
				// We don't propagate this error upstream because it does not affect normal operation of the command
			}
		}
	}()
	ch <- syscall.SIGWINCH // Make sure the pty matches current size

	// Set stdin in raw mode so that we preserve readline properties
	oldState, setRawErr := term.MakeRaw(int(os.Stdin.Fd()))
	if setRawErr != nil {
		return errors.WithStackTrace(setRawErr)
	}
	defer func() {
		if restoreErr := term.Restore(int(os.Stdin.Fd()), oldState); restoreErr != nil {
			terragruntOptions.Logger.Errorf("Error restoring terminal state: %s", restoreErr)
			// Only overwrite the previous error if there was no error since this error has lower priority than any
			// errors in the main routine
			if err == nil {
				err = errors.WithStackTrace(restoreErr)
			}
		}
	}()

	// Copy stdin to the pty
	go func() {
		_, copyStdinErr := io.Copy(pseudoTerminal, os.Stdin)
		terragruntOptions.Logger.Errorf("Error forwarding stdin: %s", copyStdinErr)
		// We don't propagate this error upstream because it does not affect normal operation of the command. A repeat
		// of the same stdin in this case should resolve the issue.
	}()

	// ... and the pty to stdout.
	_, copyStdoutErr := io.Copy(cmdStdout, pseudoTerminal)
	if copyStdoutErr != nil {
		return errors.WithStackTrace(copyStdoutErr)
	}

	return nil
}

func PrepareConsole(terragruntOptions *options.TerragruntOptions) {
	// No operation function to match windows execution
}
