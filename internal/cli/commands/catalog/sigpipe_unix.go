//go:build !windows

package catalog

import (
	"context"
	"os"
	"syscall"

	"github.com/gruntwork-io/terragrunt/internal/os/signal"
)

// notifyBrokenPipe makes a write to a closed standard output fail with EPIPE
// instead of killing the process.
//
// Go raises SIGPIPE for writes to file descriptors 1 and 2, and kills the
// program with it, but only while the program is not receiving the signal.
// Receiving it for the life of the stream is what lets the renderer stop on
// its own and lets the command remove the repositories it cloned on the way
// out, which a signal death skips.
//
// Cancelling from the callback ends discovery as soon as the reader goes
// away, rather than at whichever write fails next. Registration is scoped to
// ctx, so the rest of the process, the TUI included, keeps the default
// disposition.
func notifyBrokenPipe(ctx context.Context, cancel context.CancelFunc) {
	signal.NotifierWithContext(ctx, func(_ os.Signal) { cancel() }, syscall.SIGPIPE)
}
