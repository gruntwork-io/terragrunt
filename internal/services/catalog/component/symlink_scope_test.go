package component_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

// scopeFixture lays out a catalog clone with a secret sitting outside it:
//
//	root/clone/units/vpc   the component being scaffolded
//	root/clone/shared      catalog content outside the component
//	root/secret            a file belonging to whoever runs Terragrunt
func scopeFixture(t *testing.T) (fsys vfs.FS, clone, src, secret string) {
	t.Helper()

	fsys = vfs.NewOSFS()
	base := helpers.TmpDirWOSymlinks(t)

	clone = filepath.Join(base, "clone")
	src = filepath.Join(clone, "units", "vpc")
	secret = filepath.Join(base, "secret.txt")

	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(clone, "shared"), 0o755))
	require.NoError(t, os.WriteFile(secret, []byte("aws_secret_access_key = hunter2\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(src, "terragrunt.hcl"), []byte("# unit\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(clone, "shared", "common.hcl"), []byte("# shared\n"), 0o644))

	return fsys, clone, src, secret
}

func TestScaffoldRejectsAbsoluteEscapingSymlink(t *testing.T) {
	t.Parallel()

	fsys, clone, src, secret := scopeFixture(t)
	require.NoError(t, os.Symlink(secret, filepath.Join(src, "notes.md")))

	dst := t.TempDir()
	_, err := component.Scaffold(fsys, component.KindUnit, component.Paths{Root: clone, Src: src, Dst: dst}, nil)

	var escErr *component.SymlinkEscapesRootError

	require.ErrorAs(t, err, &escErr)
	assert.NoFileExists(t, filepath.Join(dst, "notes.md"))
	assert.NoFileExists(t, filepath.Join(dst, "terragrunt.hcl"), "nothing may be written")
}

func TestScaffoldRejectsRelativeEscapingSymlink(t *testing.T) {
	t.Parallel()

	fsys, clone, src, _ := scopeFixture(t)
	require.NoError(t, os.Symlink(filepath.Join("..", "..", "..", "secret.txt"),
		filepath.Join(src, "notes.md")))

	dst := t.TempDir()
	_, err := component.Scaffold(fsys, component.KindUnit, component.Paths{Root: clone, Src: src, Dst: dst}, nil)

	var escErr *component.SymlinkEscapesRootError

	require.ErrorAs(t, err, &escErr)
	assert.NoFileExists(t, filepath.Join(dst, "notes.md"))
}

// The middle link lives in the component, so the walk validates it directly.
func TestScaffoldRejectsChainedEscapeInsideComponent(t *testing.T) {
	t.Parallel()

	fsys, clone, src, secret := scopeFixture(t)
	require.NoError(t, os.Symlink("mid.txt", filepath.Join(src, "notes.md")))
	require.NoError(t, os.Symlink(secret, filepath.Join(src, "mid.txt")))

	dst := t.TempDir()
	_, err := component.Scaffold(fsys, component.KindUnit, component.Paths{Root: clone, Src: src, Dst: dst}, nil)

	var escErr *component.SymlinkEscapesRootError

	require.ErrorAs(t, err, &escErr)
	assert.NoFileExists(t, filepath.Join(dst, "notes.md"))
	assert.NoFileExists(t, filepath.Join(dst, "mid.txt"))
}

// The middle link lives outside the component, so the walk never visits it and
// only resolving the chain catches the escape.
func TestScaffoldRejectsChainedEscapeOutsideComponent(t *testing.T) {
	t.Parallel()

	fsys, clone, src, secret := scopeFixture(t)
	require.NoError(t, os.Symlink(secret, filepath.Join(clone, "shared", "mid.txt")))
	require.NoError(t, os.Symlink(filepath.Join("..", "..", "shared", "mid.txt"),
		filepath.Join(src, "notes.md")))

	dst := t.TempDir()
	_, err := component.Scaffold(fsys, component.KindUnit, component.Paths{Root: clone, Src: src, Dst: dst}, nil)

	var escErr *component.SymlinkEscapesRootError

	require.ErrorAs(t, err, &escErr)

	got, _ := os.ReadFile(filepath.Join(dst, "notes.md"))
	assert.NotContains(t, string(got), "hunter2")
}

// A link out of root whose target does not exist cannot be resolved, so only
// reading its stored target tells a hostile link from a merely broken one.
func TestScaffoldRejectsDanglingEscapingSymlink(t *testing.T) {
	t.Parallel()

	fsys, clone, src, _ := scopeFixture(t)
	require.NoError(t, os.Symlink("/nonexistent/secret", filepath.Join(src, "notes.md")))

	dst := t.TempDir()
	_, err := component.Scaffold(fsys, component.KindUnit, component.Paths{Root: clone, Src: src, Dst: dst}, nil)

	var escErr *component.SymlinkEscapesRootError

	require.ErrorAs(t, err, &escErr)
	assert.Equal(t, "/nonexistent/secret", escErr.Target)
}

func TestScaffoldMaterializesLinkElsewhereUnderRoot(t *testing.T) {
	t.Parallel()

	fsys, clone, src, _ := scopeFixture(t)
	require.NoError(t, os.Symlink(filepath.Join("..", "..", "shared", "common.hcl"),
		filepath.Join(src, "common.hcl")))

	dst := t.TempDir()
	_, err := component.Scaffold(fsys, component.KindUnit, component.Paths{Root: clone, Src: src, Dst: dst}, nil)
	require.NoError(t, err)

	copied := filepath.Join(dst, "common.hcl")

	info, err := os.Lstat(copied)
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink, "a link with no target in the copy is materialized")

	contents, err := os.ReadFile(copied)
	require.NoError(t, err)
	assert.Equal(t, "# shared\n", string(contents))
}
