package tui_test

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// crowdedModel builds a model over a directory holding count plain
// subdirectories, more than fit any test pane, so scrolling is exercised.
func crowdedModel(t *testing.T, count int) tui.Model {
	t.Helper()

	fs := vfs.NewMemMapFS()
	for i := range count {
		require.NoError(t, fs.MkdirAll(fmt.Sprintf("/repo/dir-%03d", i), 0o755))
	}

	return newModel(t, fs, tui.NewRoot("/repo"), false)
}

func TestViewFillsTerminalExactly(t *testing.T) {
	t.Parallel()

	m := crowdedModel(t, 100)

	content := m.View().Content
	assert.Equal(t, testHeight, lipgloss.Height(content), "the view should fill the terminal height exactly")
	assert.Equal(t, testWidth, lipgloss.Width(content), "the view should fill the terminal width exactly")
}

func TestColumnScrollsToKeepCursorVisible(t *testing.T) {
	t.Parallel()

	const height = 20

	m := crowdedModel(t, 100)
	m = update(t, m, tea.WindowSizeMsg{Width: testWidth, Height: height})

	for range 50 {
		m = press(t, m, 'j')
	}

	require.Equal(t, "dir-050", m.Selected().Name())

	content := m.View().Content
	assert.Equal(t, height, lipgloss.Height(content), "an overfull directory must not grow the view")
	assert.Contains(t, content, "dir-050", "the highlighted entry should be scrolled into view")
	assert.NotContains(t, content, "dir-000", "entries far above the cursor should be scrolled out")
}

func TestTinyTerminalRendersWithoutGarbling(t *testing.T) {
	t.Parallel()

	m := crowdedModel(t, 10)
	m = update(t, m, tea.WindowSizeMsg{Width: 8, Height: 5})

	// The layout can't fit, but rendering must stay bounded rather than spraying
	// full-width rows from negative pane sizes.
	content := m.View().Content
	assert.NotContains(t, content, "dir-000/")
}

func TestPageKeysMoveByAPage(t *testing.T) {
	t.Parallel()

	m := crowdedModel(t, 100)

	// A page is the column's visible rows: forward lands between the ends,
	// not one entry down and not on the bottom.
	m = typeKey(t, m, tea.KeyPgDown)
	afterPage := m.Selected().Name()
	require.NotEqual(t, "dir-000", afterPage)
	require.NotEqual(t, "dir-001", afterPage)
	require.NotEqual(t, "dir-099", afterPage)

	// Paging back clamps at the top.
	m = typeKey(t, m, tea.KeyPgUp)
	assert.Equal(t, "dir-000", m.Selected().Name())

	// Repeated paging clamps at the bottom.
	for range 20 {
		m = typeKey(t, m, tea.KeyPgDown)
	}

	assert.Equal(t, "dir-099", m.Selected().Name())
}
