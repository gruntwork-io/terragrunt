package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Module List View ---

func TestModuleListView_RendersTitle(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()

	m := tui.NewModel(l, opts, svc)
	updated, _ := m.Update(windowSize)
	m = updated.(tui.Model)

	view := m.View()
	content := stripANSI(view.Content)

	assert.True(t, view.AltScreen, "list view should use alt screen")
	assert.Contains(t, content, "List of Modules", "should render list title")
	assert.NotContains(t, content, "(loading...)", "non-streaming model should not show loading indicator")
}

// --- Module Pager View ---

func TestModulePagerView_RendersFooterAndButtons(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()

	m := tui.NewModel(l, opts, svc)

	// Set window size first
	updated, _ := m.Update(windowSize)
	m = updated.(tui.Model)

	// Press Enter to select first module and transition to PagerState
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(tui.Model)

	assert.Equal(t, tui.PagerState, m.State, "should be in pager state after pressing Enter")

	view := m.View()
	content := stripANSI(view.Content)

	assert.True(t, view.AltScreen, "pager view should use alt screen")
	assert.Contains(t, content, "%", "pager should show scroll percentage")
	assert.Contains(t, content, "Scaffold", "pager should show Scaffold button")
}

// windowSize is a convenience WindowSizeMsg used across view tests.
var windowSize = tea.WindowSizeMsg{Width: 120, Height: 40}

// stripANSI removes ANSI escape sequences from a string so assertions
// can match on plain text.
func stripANSI(s string) string {
	var out strings.Builder

	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			// Skip until we hit a letter (the terminator of the escape sequence).
			for i < len(s) && !isLetter(s[i]) {
				i++
			}

			continue
		}

		out.WriteByte(s[i])
	}

	return out.String()
}

func isLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
