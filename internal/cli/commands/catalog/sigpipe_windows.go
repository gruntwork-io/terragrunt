//go:build windows

package catalog

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// notifyBrokenPipe does nothing on Windows, which has no SIGPIPE. A write to
// a pipe whose reader has gone fails with an error there, so the renderer
// already learns about it the same way it does for any other write failure.
func notifyBrokenPipe(_ context.Context, _ *venv.Venv, _ context.CancelFunc) context.CancelFunc {
	return func() {}
}
