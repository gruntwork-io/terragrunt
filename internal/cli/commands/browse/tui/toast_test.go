package tui_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWarningSurfacesAsToast(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/terragrunt.hcl", nil, 0o644))

	m := newModel(t, fs, tui.NewRoot("/repo"), false)

	m = update(t, m, viewtui.Warning{Message: "cycle detected in dependency graph"})

	// The toast floats over the layout: the warning is visible and the view
	// keeps the terminal's height without overflowing its width. (Compositing
	// trims trailing blank cells, so the width can come up slightly short.)
	content := m.View().Content
	assert.Contains(t, content, "cycle detected")
	assert.Equal(t, testHeight, lipgloss.Height(content))
	assert.LessOrEqual(t, lipgloss.Width(content), testWidth)
}

func TestWarningSchedulesExpiryAndKeepsListening(t *testing.T) {
	t.Parallel()

	m := newModel(t, vfs.NewMemMapFS(), tui.NewRoot("/repo"), false)

	// The command carries the toast's expiry tick and the re-armed listener;
	// without it the toast would never dismiss and later warnings would be lost.
	_, cmd := m.Update(viewtui.Warning{Message: "boom"})
	assert.NotNil(t, cmd)
}

func TestToastExpires(t *testing.T) {
	t.Parallel()

	m := newModel(t, vfs.NewMemMapFS(), tui.NewRoot("/repo"), false)

	m = update(t, m, viewtui.Warning{Message: "transient warning"})
	require.Contains(t, m.View().Content, "transient warning")

	// Toast IDs are assigned sequentially from 1.
	m = update(t, m, viewtui.ToastExpired{ID: 1})

	assert.NotContains(t, m.View().Content, "transient warning")
}

func TestExpiryOfDismissedToastIsNoop(t *testing.T) {
	t.Parallel()

	m := newModel(t, vfs.NewMemMapFS(), tui.NewRoot("/repo"), false)

	m = update(t, m, viewtui.Warning{Message: "one"})
	m = update(t, m, viewtui.ToastExpired{ID: 1})
	m = update(t, m, viewtui.ToastExpired{ID: 1})

	assert.NotContains(t, m.View().Content, "one")
}

func TestToastStackDropsOldestPastCap(t *testing.T) {
	t.Parallel()

	m := newModel(t, vfs.NewMemMapFS(), tui.NewRoot("/repo"), false)

	for _, msg := range []string{"first", "second", "third", "fourth"} {
		m = update(t, m, viewtui.Warning{Message: msg})
	}

	content := m.View().Content
	assert.NotContains(t, content, "first", "the oldest toast should be dropped past the cap")
	assert.Contains(t, content, "second")
	assert.Contains(t, content, "third")
	assert.Contains(t, content, "fourth")
}

func TestLongToastMessageIsClipped(t *testing.T) {
	t.Parallel()

	m := newModel(t, vfs.NewMemMapFS(), tui.NewRoot("/repo"), false)

	m = update(t, m, viewtui.Warning{Message: strings.Repeat("very long warning ", 50)})

	// One pathological warning wraps into a capped box instead of covering the
	// screen, and the view keeps the terminal's dimensions.
	content := m.View().Content
	assert.Contains(t, content, "very long warning")
	assert.Equal(t, testHeight, lipgloss.Height(content))
	assert.LessOrEqual(t, lipgloss.Width(content), testWidth)
}
