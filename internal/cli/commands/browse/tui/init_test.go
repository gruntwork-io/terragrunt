package tui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitDeliversDiscoveryResultWithRacing drives the background-discovery
// listener the program arms in Init: the send and the listening receive run on
// separate goroutines so the race detector exercises the handoff.
func TestInitDeliversDiscoveryResultWithRacing(t *testing.T) {
	t.Parallel()

	resultCh := make(chan tui.DiscoveryResult)

	warnCh := make(chan viewtui.Warning)
	close(warnCh)

	m := tui.NewModel(logger.CreateLogger(), vfs.NewMemMapFS(), tui.NewRoot("/repo"), tui.ColorDisabled, resultCh, warnCh)

	want := tui.DiscoveryResult{Components: component.Components{component.NewUnit("/repo/vpc")}}

	go func() { resultCh <- want }()

	batch, ok := m.Init()().(tea.BatchMsg)
	require.True(t, ok, "Init should batch the background listener alongside its other startup commands")

	var (
		got       tui.DiscoveryResult
		delivered bool
	)

	for _, cmd := range batch {
		if res, isResult := cmd().(tui.DiscoveryResult); isResult {
			got, delivered = res, true
		}
	}

	require.True(t, delivered, "the armed listener should deliver the discovery result as a message")
	assert.Equal(t, want.Components, got.Components)
}
