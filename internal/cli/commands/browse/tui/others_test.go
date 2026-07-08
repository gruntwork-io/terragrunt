package tui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSurroundingEntriesAreShown(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/terragrunt.hcl", nil, 0o644))
	require.NoError(t, vfs.WriteFile(fs, "/repo/README.md", []byte("# repo\n"), 0o644))
	require.NoError(t, fs.MkdirAll("/repo/scripts", 0o755))

	root := tui.NewRoot("/repo")

	m := newModel(t, fs, root, false)

	// The initial window-size event reads the working directory: the README file
	// and scripts dir appear alongside vpc, which a cheap stat classifies as a
	// unit from its terragrunt.hcl, all sorted by name.
	type entry struct {
		name string
		kind tui.Kind
	}

	want := []entry{
		{name: "README.md", kind: tui.KindFile},
		{name: "scripts", kind: tui.KindDir},
		{name: "vpc", kind: tui.KindUnit},
	}

	children := m.Current().Children()
	got := make([]entry, len(children))

	for i, c := range children {
		got[i] = entry{name: c.Name(), kind: c.Kind()}
	}

	assert.Equal(t, want, got)
}

func TestStackClassifiedFromFilesystem(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/network/terragrunt.stack.hcl", nil, 0o644))
	require.NoError(t, vfs.WriteFile(fs, "/repo/db/terragrunt.hcl", nil, 0o644))
	require.NoError(t, fs.MkdirAll("/repo/plain", 0o755))

	m := newModel(t, fs, tui.NewRoot("/repo"), false)

	// With no discovery, kinds come from the cheap stat alone.
	type entry struct {
		name string
		kind tui.Kind
	}

	want := []entry{
		{name: "db", kind: tui.KindUnit},
		{name: "network", kind: tui.KindStack},
		{name: "plain", kind: tui.KindDir},
	}

	children := m.Current().Children()
	got := make([]entry, len(children))

	for i, c := range children {
		got[i] = entry{name: c.Name(), kind: c.Kind()}
	}

	assert.Equal(t, want, got)
}

func TestIgnorableDirsClassifiedAsPlain(t *testing.T) {
	t.Parallel()

	// A terragrunt.hcl inside .terragrunt-cache is a cache copy discovery never
	// scans; neither the cache dir nor anything beneath it may classify as a unit.
	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/.terragrunt-cache/xyz/terragrunt.hcl", nil, 0o644))

	m := newModel(t, fs, tui.NewRoot("/repo"), false)

	children := m.Current().Children()
	require.Len(t, children, 1)
	require.Equal(t, ".terragrunt-cache", children[0].Name())
	assert.Equal(t, tui.KindDir, children[0].Kind())

	// Descending into the cache classifies its contents the same way.
	m = press(t, m, 'l')
	require.Equal(t, ".terragrunt-cache", m.Current().Name())

	children = m.Current().Children()
	require.Len(t, children, 1)
	require.Equal(t, "xyz", children[0].Name())
	assert.Equal(t, tui.KindDir, children[0].Kind())
}

func TestSurroundingEntriesLoadedOnce(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/terragrunt.hcl", nil, 0o644))
	require.NoError(t, vfs.WriteFile(fs, "/repo/README.md", nil, 0o644))

	root := tui.BuildTree("/repo", component.Components{component.NewUnit("/repo/vpc")})

	m := newModel(t, fs, root, false)
	first := len(m.Current().Children())

	// A second window-size event must not duplicate the loaded entries.
	m = update(t, m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})

	assert.Len(t, m.Current().Children(), first)
}

func TestSurroundingEntriesBestEffortOnError(t *testing.T) {
	t.Parallel()

	// The working directory doesn't exist on the filesystem, so loading its
	// entries fails; the tree keeps just the discovered component.
	fs := vfs.NewMemMapFS()
	root := tui.BuildTree("/missing", component.Components{component.NewUnit("/missing/vpc")})

	m := newModel(t, fs, root, false)

	children := m.Current().Children()
	require.Len(t, children, 1)
	assert.Equal(t, "vpc", children[0].Name())
}

func TestJumpToTopAndBottom(t *testing.T) {
	t.Parallel()

	m := searchModel(t, "alpha", "bravo", "charlie", "delta")
	require.Equal(t, "alpha", m.Selected().Name())

	// The jumps are deliberately absent from the footer hints.
	assert.NotContains(t, m.View().Content, "gg")

	m = press(t, m, 'G')
	assert.Equal(t, "delta", m.Selected().Name())

	m = press(t, m, 'g')
	m = press(t, m, 'g')
	assert.Equal(t, "alpha", m.Selected().Name())

	// end and home jump the same way, and home does so on a single press.
	m = typeKey(t, m, tea.KeyEnd)
	assert.Equal(t, "delta", m.Selected().Name())

	m = typeKey(t, m, tea.KeyHome)
	assert.Equal(t, "alpha", m.Selected().Name())
}

func TestPendingTopJumpDisarmsOnOtherKey(t *testing.T) {
	t.Parallel()

	m := searchModel(t, "alpha", "bravo", "charlie")

	// A g followed by another key is not a jump: the j moves down as usual.
	m = press(t, m, 'g')
	m = press(t, m, 'j')
	assert.Equal(t, "bravo", m.Selected().Name())

	// The j also disarmed the chord, so a single g afterward does nothing.
	m = press(t, m, 'g')
	assert.Equal(t, "bravo", m.Selected().Name())
}

func TestHiddenDirectoriesDimmed(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/repo/.github", 0o755))
	require.NoError(t, fs.MkdirAll("/repo/avisible", 0o755))
	require.NoError(t, fs.MkdirAll("/repo/bvisible", 0o755))
	require.NoError(t, vfs.WriteFile(fs, "/repo/zz.txt", nil, 0o644))

	m := newModel(t, fs, tui.NewRoot("/repo"), false)

	// The hidden directory sorts first and starts out selected; move off it so
	// its own style is visible.
	m = press(t, m, 'j')

	content := m.View().Content
	assert.Equal(t, styleBefore(t, content, "zz.txt"), styleBefore(t, content, ".github/"),
		"a hidden directory should be dimmed like a plain file")
	assert.NotEqual(t, styleBefore(t, content, "bvisible/"), styleBefore(t, content, ".github/"),
		"a hidden directory should not render like a visible one")
}
