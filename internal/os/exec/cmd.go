// Package exec runs external commands. It wraps exec.Cmd package with support for allocating a pseudo-terminal.
package exec

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// Cmd is a command type.
type Cmd struct {
	*exec.Cmd

	filename string

	logger log.Logger
	usePTY bool

	forwardSignalDelay time.Duration
	interruptSignal    os.Signal
}

// Command returns the `Cmd` struct to execute the named program with
// the given arguments.
func Command(name string, args ...string) *Cmd {
	cmd := &Cmd{
		Cmd:             exec.Command(name, args...),
		logger:          log.Default(),
		filename:        filepath.Base(name),
		interruptSignal: signal.InterruptSignal,
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd
}

// Configure sets options to the `Cmd`.
func (cmd *Cmd) Configure(opts ...Option) {
	for _, opt := range opts {
		opt(cmd)
	}
}

// Start starts the specified command but does not wait for it to complete.
func (cmd *Cmd) Start() error {
	// If we need to allocate a ptty for the command, route through the ptty routine.
	// Otherwise, directly call the command.
	if cmd.usePTY {
		if err := runCommandWithPTY(cmd.logger, cmd.Cmd); err != nil {
			return err
		}
	} else if err := cmd.Cmd.Start(); err != nil {
		return errors.New(err)
	}

	return nil
}

// RegisterGracefullyShutdown registers a graceful shutdown for the command in two ways:
//  1. If the context cancel contains a cause with a signal, this means that Terragrunt received the signal from the OS,
//     since our executed command may also receive the same signal, we need to give the command time to gracefully shutting down,
//     to avoid the command receiving this signal twice.
//     Thus we will send the signal to the executed command with a delay or immediately if Terragrunt receives this same signal again.
//  2. If the context does not contain any causes, this means that there was some failure and we need to terminate all executed commands,
//     in this situation we are sure that commands did not receive any signal, so we send them an interrupt signal immediately.
func (cmd *Cmd) RegisterGracefullyShutdown(ctx context.Context) func() {
	ctxShutdown, cancelShutdown := context.WithCancel(context.Background())

	go func() {
		select {
		case <-ctxShutdown.Done():
		case <-ctx.Done():
			if cause := new(signal.ContextCanceledError); errors.As(context.Cause(ctx), &cause) && cause.Signal != nil {
				cmd.ForwardSignal(ctxShutdown, cause.Signal)

				return
			}

			cmd.SendSignal(cmd.interruptSignal)
		}
	}()

	return cancelShutdown
}

// ForwardSignal forwards a given `sig` with a delay if cmd.forwardSignalDelay is greater than 0,
// and if the same signal is received again, it is forwarded immediately.
func (cmd *Cmd) ForwardSignal(ctx context.Context, sig os.Signal) {
	ctxDelay, cancelDelay := context.WithCancel(ctx)
	defer cancelDelay()

	signal.NotifierWithContext(ctx, func(_ os.Signal) {
		cancelDelay()
	}, sig)

	if cmd.forwardSignalDelay > 0 {
		cmd.logger.Debugf("%s signal will be forwarded to %s with delay %s",
			cases.Title(language.English).String(sig.String()),
			cmd.filename,
			cmd.forwardSignalDelay,
		)
	}

	select {
	case <-ctx.Done():
		return
	case <-time.After(cmd.forwardSignalDelay):
	case <-ctxDelay.Done():
	}

	cmd.SendSignal(sig)
}

// SendSignal sends the given `sig` to the executed command.
func (cmd *Cmd) SendSignal(sig os.Signal) {
	cmd.logger.Debugf("%s signal is forwarded to %s", cases.Title(language.English).String(sig.String()), cmd.filename)

	if err := cmd.Process.Signal(sig); err != nil {
		cmd.logger.Errorf("Failed to forwarding signal %s to %s: %v", sig, cmd.filename, err)
	}
}
