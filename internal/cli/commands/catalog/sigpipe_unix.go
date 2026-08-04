//go:build !windows

package catalog

import (
	"context"
	"os"
	"syscall"

	"github.com/gruntwork-io/terragrunt/internal/os/signal"
)

// notifyBrokenPipe makes a write to a closed standard output fail with EPIPE
// instead of killing the process, and returns the function that restores the
// default disposition.
//
// Go raises SIGPIPE for writes to file descriptors 1 and 2, and kills the
// program with it, but only while the program is not receiving the signal.
// Receiving it is what lets the renderer stop on its own and lets the command
// remove the repositories it cloned on the way out, which a signal death
// skips.
//
// Cancelling from the callback ends discovery as soon as the reader goes
// away, rather than at whichever write fails next. The registration outlives
// that cancellation and ends only when the caller stops it: cleanup writes
// too, and by then the reader is already gone. Everything outside that
// window, the TUI included, keeps the default disposition.
func notifyBrokenPipe(ctx context.Context, cancel context.CancelFunc) context.CancelFunc {
	notifyCtx, stop := context.WithCancel(ctx)

	signal.NotifierWithContext(notifyCtx, func(_ os.Signal) { cancel() }, syscall.SIGPIPE)

	return stop
}
