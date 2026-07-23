package tui_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sgrPattern matches an ANSI SGR (color/style) escape sequence.
var sgrPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

// styleBefore returns the last SGR escape preceding name in content, i.e. the
// color a rendered entry is drawn in. Comparing these relatively avoids pinning
// exact color codes.
func styleBefore(t *testing.T, content, name string) string {
	t.Helper()

	idx := strings.Index(content, name)
	require.GreaterOrEqualf(t, idx, 0, "expected %q in rendered content", name)

	locs := sgrPattern.FindAllStringIndex(content[:idx], -1)
	require.NotEmptyf(t, locs, "expected a style escape before %q", name)

	last := locs[len(locs)-1]

	return content[last[0]:last[1]]
}

func TestDiscoveryResolvesCountsAndClearsLoading(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/group/db/terragrunt.hcl", nil, 0o644))

	m := newModel(t, fs, tui.NewRoot("/repo"), tui.ColorDisabled)

	// Before discovery the selected group directory's counts are placeholders and
	// the footer advertises that discovery is still running.
	before := m.View().Content
	assert.Contains(t, before, "discovering…")
	assert.Contains(t, before, "Units")
	assert.NotContains(t, before, "Units: 1")

	m = update(t, m, tui.DiscoveryResult{
		Components: component.Components{component.NewUnit("/repo/group/db")},
	})

	// Once discovery completes the count resolves and the indicator clears.
	after := m.View().Content
	assert.Contains(t, after, "Units: 1")
	assert.NotContains(t, after, "discovering…")
}

func TestDiscoveryFailureSurfacedAsToast(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/group/db/terragrunt.hcl", nil, 0o644))

	m := newModel(t, fs, tui.NewRoot("/repo"), tui.ColorDisabled)

	m = update(t, m, tui.DiscoveryResult{Err: errors.New("discovery blew up")})

	// A failed discovery must not look like a clean, empty estate: the
	// "discovering…" indicator clears and a toast flags the failure.
	content := m.View().Content
	assert.NotContains(t, content, "discovering…")
	assert.Contains(t, content, "discovery failed")

	// The failure toast expires like any other toast.
	m = update(t, m, viewtui.ToastExpired{ID: 1})
	assert.NotContains(t, m.View().Content, "discovery failed")
}

func TestReadFilesHighlightedAfterDiscovery(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/db/terragrunt.hcl", nil, 0o644))
	require.NoError(t, vfs.WriteFile(fs, "/repo/read.tfvars", nil, 0o644))
	require.NoError(t, vfs.WriteFile(fs, "/repo/other.txt", nil, 0o644))
	require.NoError(t, fs.MkdirAll("/repo/dir", 0o755))
	// aaa sorts first, so it's the selected row, keeping the comparison entries
	// unselected and their preview free of the read-file name.
	require.NoError(t, fs.MkdirAll("/repo/aaa", 0o755))

	m := newModel(t, fs, tui.NewRoot("/repo"), tui.ColorDisabled)

	// Before discovery, a read file and an unrelated file are both dimmed like
	// files, distinct from the white plain directory.
	before := m.View().Content
	assert.Equal(t, styleBefore(t, before, "other.txt"), styleBefore(t, before, "read.tfvars"),
		"both files should be dimmed before discovery")
	assert.NotEqual(t, styleBefore(t, before, "dir/"), styleBefore(t, before, "read.tfvars"),
		"a file should not match a plain directory before discovery")

	m = update(t, m, tui.DiscoveryResult{
		Components: component.Components{
			component.NewUnit("/repo/db").WithReading("/repo/read.tfvars"),
		},
	})

	// After discovery, the read file is promoted to the white "relevant" style,
	// matching the directory and diverging from the unrelated file.
	after := m.View().Content
	assert.Equal(t, styleBefore(t, after, "dir/"), styleBefore(t, after, "read.tfvars"),
		"a read file should render like a relevant entry after discovery")
	assert.NotEqual(t, styleBefore(t, after, "other.txt"), styleBefore(t, after, "read.tfvars"),
		"an unrelated file should stay dimmed after discovery")
}
