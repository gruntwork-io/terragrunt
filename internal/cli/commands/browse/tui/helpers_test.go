package tui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/require"
)

const (
	// testWidth and testHeight are a generous terminal size, large enough that
	// the preview pane has room to render.
	testWidth  = 120
	testHeight = 40
)

// newModel builds a Model over root and feeds it an initial window size, the
// point at which the TUI becomes ready and loads the surrounding entries.
func newModel(t *testing.T, fs vfs.FS, root *tui.Node, shouldColor bool) tui.Model {
	t.Helper()

	m := tui.NewModel(fs, root, shouldColor)

	return update(t, m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})
}

// update applies a message and returns the resulting Model.
func update(t *testing.T, m tui.Model, msg tea.Msg) tui.Model {
	t.Helper()

	next, _ := m.Update(msg)

	out, ok := next.(tui.Model)
	require.Truef(t, ok, "Update returned %T, want tui.Model", next)

	return out
}

// press sends a single rune key press, mirroring how bubbletea delivers keys.
func press(t *testing.T, m tui.Model, r rune) tui.Model {
	t.Helper()

	return update(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
}
