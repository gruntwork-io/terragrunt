package tui_test

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// searchModel builds a model whose current directory holds the given unit names,
// sorted alphabetically. An empty filesystem keeps the children to just the
// discovered units.
func searchModel(t *testing.T, names ...string) tui.Model {
	t.Helper()

	comps := make(component.Components, len(names))
	for i, name := range names {
		comps[i] = component.NewUnit(filepath.Join("/work", name))
	}

	return newModel(t, vfs.NewMemMapFS(), tui.BuildTree("/work", comps), tui.ColorDisabled)
}

// typeKey sends a non-rune key such as enter or escape.
func typeKey(t *testing.T, m tui.Model, code rune) tui.Model {
	t.Helper()

	return update(t, m, tea.KeyPressMsg{Code: code})
}

func TestSearchJumpsToFirstMatch(t *testing.T) {
	t.Parallel()

	m := searchModel(t, "alpha", "bravo", "charlie", "delta")
	require.Equal(t, "alpha", m.Selected().Name())

	m = press(t, m, '/')
	m = press(t, m, 'c')

	assert.Equal(t, "charlie", m.Selected().Name())
}

func TestSearchCapturesNavigationKeys(t *testing.T) {
	t.Parallel()

	m := searchModel(t, "alpha", "bravo", "charlie", "delta")

	m = press(t, m, '/')
	m = press(t, m, 'j')

	assert.Equal(t, "alpha", m.Selected().Name())
}

func TestSearchEscapeRestoresCursor(t *testing.T) {
	t.Parallel()

	m := searchModel(t, "alpha", "bravo", "charlie", "delta")

	m = press(t, m, '/')
	m = press(t, m, 'd')
	require.Equal(t, "delta", m.Selected().Name())

	m = typeKey(t, m, tea.KeyEscape)

	assert.Equal(t, "alpha", m.Selected().Name())
}

func TestSearchBackspaceToEmptyRestoresCursor(t *testing.T) {
	t.Parallel()

	m := searchModel(t, "alpha", "bravo", "charlie", "delta")

	m = press(t, m, '/')
	m = press(t, m, 'd')
	require.Equal(t, "delta", m.Selected().Name())

	m = typeKey(t, m, tea.KeyBackspace)

	assert.Equal(t, "alpha", m.Selected().Name())
}

func TestNextAndPrevMatchCycleWithWrap(t *testing.T) {
	t.Parallel()

	// Sorted order: db, vpc-a, vpc-b, vpc-c.
	m := searchModel(t, "vpc-a", "vpc-b", "db", "vpc-c")

	m = press(t, m, '/')
	m = press(t, m, 'v')
	m = press(t, m, 'p')
	m = press(t, m, 'c')
	require.Equal(t, "vpc-a", m.Selected().Name())

	m = typeKey(t, m, tea.KeyEnter)

	// n walks forward through the matches.
	m = press(t, m, 'n')
	assert.Equal(t, "vpc-b", m.Selected().Name())

	m = press(t, m, 'n')
	assert.Equal(t, "vpc-c", m.Selected().Name())

	// Past the last match, n wraps around the non-matching db to the first match.
	m = press(t, m, 'n')
	assert.Equal(t, "vpc-a", m.Selected().Name())

	// N walks backward, wrapping the other way.
	m = press(t, m, 'N')
	assert.Equal(t, "vpc-c", m.Selected().Name())
}

func TestSearchMarksMatchesAndCount(t *testing.T) {
	t.Parallel()

	// Sorted order: db, subnet, vpc-a, vpc-b.
	m := searchModel(t, "vpc-a", "vpc-b", "db", "subnet")

	m = press(t, m, '/')
	m = press(t, m, 'v')
	m = press(t, m, 'p')
	m = press(t, m, 'c')

	content := m.View().Content
	assert.Contains(t, content, "▸", "matching rows should be marked")
	assert.Contains(t, content, "2 matches", "the footer should count the matches")
}

func TestCommittedSearchShowsStatus(t *testing.T) {
	t.Parallel()

	m := searchModel(t, "vpc-a", "vpc-b", "db")

	m = press(t, m, '/')
	m = press(t, m, 'v')
	m = press(t, m, 'p')
	m = press(t, m, 'c')
	m = typeKey(t, m, tea.KeyEnter)

	// After committing, the match marks persist and the footer summarizes the
	// search with the keys that cycle and clear it.
	content := m.View().Content
	assert.Contains(t, content, "▸")
	assert.Contains(t, content, "2 matches")
	assert.Contains(t, content, "n/N cycle")
}

func TestEscapeClearsCommittedSearch(t *testing.T) {
	t.Parallel()

	m := searchModel(t, "vpc-a", "vpc-b", "db")

	m = press(t, m, '/')
	m = press(t, m, 'v')
	m = typeKey(t, m, tea.KeyEnter)
	require.Contains(t, m.View().Content, "match")

	// Escape clears the committed search: the marks and status go away and the
	// help line returns.
	m = typeKey(t, m, tea.KeyEscape)

	content := m.View().Content
	assert.NotContains(t, content, "matches")
	assert.NotContains(t, content, "▸")
	assert.Contains(t, content, "quit")
}

func TestNavigatingClearsSearch(t *testing.T) {
	t.Parallel()

	root := tui.BuildTree("/work", component.Components{
		component.NewUnit(filepath.Join("/work", "env", "vpc")),
		component.NewUnit(filepath.Join("/work", "env", "db")),
	})

	m := newModel(t, vfs.NewMemMapFS(), root, tui.ColorDisabled)

	// Descend into env, then search and commit within it.
	m = press(t, m, 'l')
	m = press(t, m, '/')
	m = press(t, m, 'v')
	m = typeKey(t, m, tea.KeyEnter)
	require.Contains(t, m.View().Content, "match")

	m = press(t, m, 'h')

	assert.NotContains(t, m.View().Content, "n/N cycle")
}

func TestFooterShowsSearchHintAndPrompt(t *testing.T) {
	t.Parallel()

	m := searchModel(t, "alpha", "bravo")

	// The help line advertises the search key.
	assert.Contains(t, m.View().Content, "search")

	// Opening the search swaps the help line for the "/" prompt and shows the
	// query as it's typed.
	m = press(t, m, '/')
	m = press(t, m, 'a')

	assert.Contains(t, m.View().Content, "/a")
}

func TestNextMatchNoopWithoutCommittedSearch(t *testing.T) {
	t.Parallel()

	m := searchModel(t, "alpha", "bravo", "charlie")

	m = press(t, m, 'n')

	assert.Equal(t, "alpha", m.Selected().Name())
}
