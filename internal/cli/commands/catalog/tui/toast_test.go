package tui_test

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sizedWelcomeModel builds a WelcomeModel that has seen a window size, so the
// toast overlay has dimensions to composite into.
func sizedWelcomeModel(t *testing.T) tui.WelcomeModel {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	noSourcesLoad := func(_ context.Context, _ tui.StatusFunc, _ chan<- *tui.ComponentEntry) error {
		return nil
	}

	m := tui.NewWelcomeModel(t.Context(), logger.CreateLogger(), venv.OSVenv(), opts, noSourcesLoad)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	welcome, ok := next.(tui.WelcomeModel)
	require.Truef(t, ok, "Update returned %T, want tui.WelcomeModel", next)

	return welcome
}

func TestWelcomeWarningSurfacesAsToast(t *testing.T) {
	t.Parallel()

	m := sizedWelcomeModel(t)

	next, cmd := m.Update(viewtui.Warning{Message: "repository failed to load"})
	require.NotNil(t, cmd, "a warning must schedule its toast's expiry")

	m, ok := next.(tui.WelcomeModel)
	require.True(t, ok)
	assert.Contains(t, m.View().Content, "repository failed to load")

	// Toast IDs are assigned sequentially from 1.
	next, _ = m.Update(viewtui.ToastExpired{ID: 1})

	m, ok = next.(tui.WelcomeModel)
	require.True(t, ok)
	assert.NotContains(t, m.View().Content, "repository failed to load")
}

func TestToastsCarryOverToListModel(t *testing.T) {
	t.Parallel()

	m := sizedWelcomeModel(t)

	next, _ := m.Update(viewtui.Warning{Message: "sticky warning"})

	m, ok := next.(tui.WelcomeModel)
	require.True(t, ok)
	require.Contains(t, m.View().Content, "sticky warning")

	// The first component swaps the welcome model for the streaming list
	// model; the active toast must survive the swap.
	entry := tui.NewComponentEntry(tui.NewComponentForTest(
		tui.ComponentKindModule,
		"github.com/gruntwork-io/test-repo-1",
		"modules/aws-vpc",
		"# AWS VPC Module",
	))

	next, _ = m.Update(tui.ComponentMsg(entry))

	listModel, ok := next.(tui.Model)
	require.Truef(t, ok, "Update returned %T, want tui.Model", next)

	// The swapped-in model is sized via a command, so size it directly here.
	next, _ = listModel.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	listModel, ok = next.(tui.Model)
	require.True(t, ok)
	assert.Contains(t, listModel.View().Content, "sticky warning")

	// The list model keeps handling the toast lifecycle after the swap.
	next, _ = listModel.Update(viewtui.ToastExpired{ID: 1})

	listModel, ok = next.(tui.Model)
	require.True(t, ok)
	assert.NotContains(t, listModel.View().Content, "sticky warning")
}
