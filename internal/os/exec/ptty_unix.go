//go:build !windows
// +build !windows

package exec

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"
	"golang.org/x/term"

	"github.com/creack/pty"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
)

// runCommandWithPTY will allocate a pseudo-tty to run the subcommand in. This is only necessary when running
// interactive commands, so that terminal features like readline work through the subcommand when stdin, stdout, and
// stderr is being shared.
// NOTE: This is based on the quickstart example from https://github.com/creack/pty
func runCommandWithPTY(logger log.Logger, cmd *exec.Cmd) (err error) {
	cmdStdout := cmd.Stdout

	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// NOTE: in order to ensure we can return errors that occur in cleanup, we use a variable binding for the return
	// value so that it can be updated.
	pseudoTerminal, err := pty.Start(cmd)
	if err != nil {
		return errors.New(err)
	}

	defer func() {
		if closeErr := pseudoTerminal.Close(); closeErr != nil {
			closeErr = errors.Errorf("Error closing pty: %w", closeErr)

			// Only overwrite the previous error if there was no error since this error has lower priority than any
			// errors in the main routine
			if err == nil {
				err = closeErr
			} else {
				logger.Error(closeErr)
			}
		}
	}()

	// Every time the current terminal size changes, we need to make sure the PTY also updates the size.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	go func() {
		for range ch {
			if inheritSizeErr := pty.InheritSize(os.Stdin, pseudoTerminal); inheritSizeErr != nil {
				inheritSizeErr = errors.Errorf("Error resizing pty: %w", inheritSizeErr)

				// We don't propagate this error upstream because it does not affect normal operation of the command
				logger.Error(inheritSizeErr)
			}
		}
	}()
	ch <- syscall.SIGWINCH // Make sure the pty matches current size

	// Set stdin in raw mode so that we preserve readline properties
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return errors.New(err)
	}

	defer func() {
		if restoreErr := term.Restore(int(os.Stdin.Fd()), oldState); restoreErr != nil {
			restoreErr = errors.Errorf("error restoring terminal state: %w", restoreErr)

			// Only overwrite the previous error if there was no error since this error has lower priority than any
			// errors in the main routine
			if err == nil {
				err = restoreErr
			} else {
				logger.Error(restoreErr)
			}
		}
	}()

	ctx := context.Background()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errGroup, ctx := errgroup.WithContext(ctx)

	// Copy stdout to the pty.
	errGroup.Go(func() error {
		defer cancel()

		if _, err := util.Copy(ctx, cmdStdout, pseudoTerminal); err != nil {
			return errors.Errorf("error forwarding stdout: %w", err)
		}

		return nil
	})

	// Copy stdin to the pty.
	errGroup.Go(func() error {
		defer cancel()

		if _, err := util.Copy(ctx, pseudoTerminal, os.Stdin); err != nil {
			return errors.Errorf("error forwarding stdin: %w", err)
		}

		return nil
	})

	if err := errGroup.Wait(); err != nil && !errors.IsError(err, io.EOF) && !errors.IsContextCanceled(err) {
		return errors.New(err)
	}

	return nil
}

// PrepareConsole is run at the start of the application to set up the console.
func PrepareConsole(_ log.Logger) {
	// No operation function to match windows execution
}
