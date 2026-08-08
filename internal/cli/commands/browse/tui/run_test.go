package tui_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// TestRunTreatsCancelledContextAsCleanExit exercises the program's whole
// lifecycle headlessly: a context cancelled before the loop starts unwinds to a
// nil error rather than surfacing the program's kill as a failure. The channels
// mirror what the browse command supplies once discovery has finished — a result
// waiting on the buffer and a closed warning stream — so the listeners Init arms
// complete instead of blocking a leaked goroutine.
func TestRunTreatsCancelledContextAsCleanExit(t *testing.T) {
	t.Parallel()

	resultCh := make(chan tui.DiscoveryResult, 1)
	resultCh <- tui.DiscoveryResult{}

	warnCh := make(chan viewtui.Warning)
	close(warnCh)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	l := logger.CreateLogger()

	err := tui.Run(ctx, l, vfs.NewMemMapFS(), tui.NewRoot("/repo"), tui.ColorDisabled, resultCh, warnCh)
	require.NoError(t, err)
}
