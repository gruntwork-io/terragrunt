package tui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/list/tui"
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

	root := tui.BuildTree("/repo", component.Components{component.NewUnit("/repo/vpc")})

	m := newModel(t, fs, root, false)

	// The initial window-size event loads the working directory's filesystem
	// entries: the README file and scripts dir appear as dimmed "other" entries
	// alongside the discovered vpc unit, sorted by name.
	type entry struct {
		name  string
		kind  tui.Kind
		other bool
	}

	want := []entry{
		{name: "README.md", kind: tui.KindFile, other: true},
		{name: "scripts", kind: tui.KindDir, other: true},
		{name: "vpc", kind: tui.KindUnit, other: false},
	}

	children := m.Current().Children()
	got := make([]entry, len(children))

	for i, c := range children {
		got[i] = entry{name: c.Name(), kind: c.Kind(), other: c.Other()}
	}

	assert.Equal(t, want, got)
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
