package run

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetupWorkingDirRemovesTopLevelManifest pins that a forged .terragrunt-module-manifest at the top of a freshly-downloaded source is stripped before util.CopyFolderContents reads it.
func TestSetupWorkingDirRemovesTopLevelManifest(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	root := "/cache"

	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "main.tf"), []byte("# main"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, ModuleManifestName), []byte("forged"), 0o644))

	require.NoError(t, setupWorkingDir(fsys, root))

	exists, err := vfs.FileExists(fsys, filepath.Join(root, ModuleManifestName))
	require.NoError(t, err)
	assert.False(t, exists, "top-level forged manifest must be removed")

	exists, err = vfs.FileExists(fsys, filepath.Join(root, "main.tf"))
	require.NoError(t, err)
	assert.True(t, exists, "non-manifest files must be preserved")
}

// TestSetupWorkingDirRemovesNestedManifests pins that forged manifests planted in subdirectories of a downloaded source are also stripped.
func TestSetupWorkingDirRemovesNestedManifests(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	root := "/cache"

	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "sub", "main.tf"), []byte("# sub"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "sub", ModuleManifestName), []byte("forged sub"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "sub", "deep", ModuleManifestName), []byte("forged deep"), 0o644))

	require.NoError(t, setupWorkingDir(fsys, root))

	exists, err := vfs.FileExists(fsys, filepath.Join(root, "sub", ModuleManifestName))
	require.NoError(t, err)
	assert.False(t, exists, "nested manifest at depth 1 must be removed")

	exists, err = vfs.FileExists(fsys, filepath.Join(root, "sub", "deep", ModuleManifestName))
	require.NoError(t, err)
	assert.False(t, exists, "nested manifest at depth 2 must be removed")

	exists, err = vfs.FileExists(fsys, filepath.Join(root, "sub", "main.tf"))
	require.NoError(t, err)
	assert.True(t, exists, "non-manifest files in subdirs must be preserved")
}

// TestSetupWorkingDirMissingRootIsNoOp pins that a missing root is a no-op rather than an error so the caller does not have to pre-check.
func TestSetupWorkingDirMissingRootIsNoOp(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()

	require.NoError(t, setupWorkingDir(fsys, "/does-not-exist"))
}
