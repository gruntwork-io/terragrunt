package tui_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
)

// TestNewModelPanicsOnNilChannels pins the precondition that keeps a nil
// channel, which would deadlock the background listeners, out of the browser by
// construction: neither channel may be nil.
func TestNewModelPanicsOnNilChannels(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	fs := vfs.NewMemMapFS()
	root := tui.NewRoot("/repo")
	resultCh := make(chan tui.DiscoveryResult)
	warnCh := make(chan viewtui.Warning)

	assert.PanicsWithValue(t, tui.ErrChannelsRequired, func() {
		tui.NewModel(l, fs, root, tui.ColorDisabled, nil, warnCh)
	})
	assert.PanicsWithValue(t, tui.ErrChannelsRequired, func() {
		tui.NewModel(l, fs, root, tui.ColorDisabled, resultCh, nil)
	})
}
