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

// TestSetupWorkingDirMissingRootIsNoOp pins that a missing cacheDir is a silent no-op with no filesystem side-effects.
func TestSetupWorkingDirMissingRootIsNoOp(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	missing := "/does-not-exist"

	require.NoError(t, setupWorkingDir(fsys, missing))

	exists, err := vfs.FileExists(fsys, missing)
	require.NoError(t, err)
	assert.False(t, exists, "missing cacheDir must not be created by the scrub")
}

// TestSetupWorkingDirRemovesManifestNamedDirectory pins that a downloaded module containing a directory whose name matches ModuleManifestName is removed entirely so a later open-as-file does not fail with EISDIR.
func TestSetupWorkingDirRemovesManifestNamedDirectory(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	cacheDir := "/cache"
	manifestDir := filepath.Join(cacheDir, ModuleManifestName)

	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(manifestDir, "trapped.tf"), []byte("# trap"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(cacheDir, "main.tf"), []byte("# main"), 0o644))

	require.NoError(t, setupWorkingDir(fsys, cacheDir))

	exists, err := vfs.FileExists(fsys, manifestDir)
	require.NoError(t, err)
	assert.False(t, exists, "directory whose name matches ModuleManifestName must be removed entirely")

	exists, err = vfs.FileExists(fsys, filepath.Join(cacheDir, "main.tf"))
	require.NoError(t, err)
	assert.True(t, exists, "non-manifest files must be preserved")
}

// TestSetupWorkingDirRemovesNestedManifestNamedDirectory pins that the directory-as-manifest case is also handled at depth.
func TestSetupWorkingDirRemovesNestedManifestNamedDirectory(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	cacheDir := "/cache"
	nestedManifestDir := filepath.Join(cacheDir, "sub", "deep", ModuleManifestName)

	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(nestedManifestDir, "trapped.tf"), []byte("# trap"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(cacheDir, "sub", "main.tf"), []byte("# sub"), 0o644))

	require.NoError(t, setupWorkingDir(fsys, cacheDir))

	exists, err := vfs.FileExists(fsys, nestedManifestDir)
	require.NoError(t, err)
	assert.False(t, exists, "nested directory matching ModuleManifestName must be removed entirely")

	exists, err = vfs.FileExists(fsys, filepath.Join(cacheDir, "sub", "main.tf"))
	require.NoError(t, err)
	assert.True(t, exists, "non-manifest files in subdirs must be preserved")
}
