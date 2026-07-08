package tui_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilePreviewRendersSelectedFile(t *testing.T) {
	t.Parallel()

	const content = "inputs = {}\n"

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/terragrunt.hcl", []byte(content), 0o644))

	root := tui.BuildTree("/repo", component.Components{component.NewUnit("/repo/vpc")})

	m := newModel(t, fs, root, false) // color off
	m = press(t, m, 'l')              // into the unit, selecting terragrunt.hcl

	sel := m.Selected()
	require.NotNil(t, sel)
	assert.Equal(t, "terragrunt.hcl", sel.Name())
	// Color is off, so the preview is the raw file content.
	assert.Equal(t, content, sel.Preview())
}

func TestFilePreviewColorHighlights(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/terragrunt.hcl", []byte("inputs = {}\n"), 0o644))

	root := tui.BuildTree("/repo", component.Components{component.NewUnit("/repo/vpc")})

	m := newModel(t, fs, root, true) // color on
	m = press(t, m, 'l')

	assert.Contains(t, m.Selected().Preview(), "\x1b[", "expected ANSI escapes in highlighted preview")
}

func TestFilePreviewBinary(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/terragrunt.hcl", []byte("inputs = {}\n"), 0o644))
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/blob", []byte("a\x00b"), 0o644))

	root := tui.BuildTree("/repo", component.Components{component.NewUnit("/repo/vpc")})

	m := newModel(t, fs, root, false)
	m = press(t, m, 'l') // into the unit; "blob" sorts before "terragrunt.hcl"

	sel := m.Selected()
	require.NotNil(t, sel)
	assert.Equal(t, "blob", sel.Name())
	assert.Contains(t, sel.Preview(), "binary")
}
