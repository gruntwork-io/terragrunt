package tui_test

import (
	"strings"
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

	m := newModel(t, fs, root, tui.ColorDisabled)
	m = press(t, m, 'l') // into the unit, selecting terragrunt.hcl

	sel := m.Selected()
	require.NotNil(t, sel)
	assert.Equal(t, "terragrunt.hcl", sel.Name())
	assert.Equal(t, content, sel.Preview())
}

func TestFilePreviewColorHighlights(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/terragrunt.hcl", []byte("inputs = {}\n"), 0o644))

	root := tui.BuildTree("/repo", component.Components{component.NewUnit("/repo/vpc")})

	m := newModel(t, fs, root, tui.ColorEnabled)
	m = press(t, m, 'l')

	assert.Contains(t, m.Selected().Preview(), "\x1b[", "expected ANSI escapes in highlighted preview")
}

func TestFilePreviewBinary(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/terragrunt.hcl", []byte("inputs = {}\n"), 0o644))
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/blob", []byte("a\x00b"), 0o644))

	root := tui.BuildTree("/repo", component.Components{component.NewUnit("/repo/vpc")})

	m := newModel(t, fs, root, tui.ColorDisabled)
	m = press(t, m, 'l') // into the unit; "blob" sorts before "terragrunt.hcl"

	sel := m.Selected()
	require.NotNil(t, sel)
	assert.Equal(t, "blob", sel.Name())
	assert.Contains(t, sel.Preview(), "binary")
}

func TestFilePreviewReadsHeadOfLargeFile(t *testing.T) {
	t.Parallel()

	const head = "first line of a big file\n"

	// Comfortably past previewByteLimit (256 KiB), so the whole file is never read.
	big := head + strings.Repeat("A", 300<<10)

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/big.txt", []byte(big), 0o644))

	root := tui.BuildTree("/repo", component.Components{component.NewUnit("/repo/vpc")})

	m := newModel(t, fs, root, tui.ColorDisabled)
	m = press(t, m, 'l')

	sel := m.Selected()
	require.Equal(t, "big.txt", sel.Name())
	assert.Contains(t, sel.Preview(), head)
	assert.Less(t, len(sel.Preview()), len(big), "only the head should be read, not the whole file")
}

func TestMarkdownPreviewIsStyled(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/terragrunt.hcl", []byte("inputs = {}\n"), 0o644))
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/README.md", []byte("# Heading\n\nBody text.\n"), 0o644))

	root := tui.BuildTree("/repo", component.Components{component.NewUnit("/repo/vpc")})

	m := newModel(t, fs, root, tui.ColorEnabled)
	m = press(t, m, 'l')

	sel := m.Selected()
	require.Equal(t, "README.md", sel.Name())
	assert.Contains(t, sel.Preview(), "Heading")
	assert.Contains(t, sel.Preview(), "\x1b[", "markdown should be rendered with styling")
}

func TestFilePreviewLexesByContentWithoutExtension(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/terragrunt.hcl", []byte("inputs = {}\n"), 0o644))
	require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/bootstrap", []byte("#!/bin/bash\necho hello\n"), 0o644))

	root := tui.BuildTree("/repo", component.Components{component.NewUnit("/repo/vpc")})

	m := newModel(t, fs, root, tui.ColorEnabled)
	m = press(t, m, 'l')

	sel := m.Selected()
	require.Equal(t, "bootstrap", sel.Name())
	assert.Contains(t, sel.Preview(), "\x1b[", "a shebang should select a lexer by content and highlight it")
}
