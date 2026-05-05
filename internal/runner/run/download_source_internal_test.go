package run

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetupWorkingDirRemovesAllManifests verifies setupWorkingDir removes every .terragrunt-module-manifest under the root, including nested ones, while leaving other files untouched.
func TestSetupWorkingDirRemovesAllManifests(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()

	root := "/cache"

	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "main.tf"), []byte("# main"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, ModuleManifestName), []byte("top"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "sub", "main.tf"), []byte("# sub"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "sub", ModuleManifestName), []byte("sub"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "sub", "nested", ModuleManifestName), []byte("nested"), 0o644))

	require.NoError(t, setupWorkingDir(fsys, root))

	exists, err := vfs.FileExists(fsys, filepath.Join(root, ModuleManifestName))
	require.NoError(t, err)
	assert.False(t, exists, "top-level manifest must be removed")

	exists, err = vfs.FileExists(fsys, filepath.Join(root, "sub", ModuleManifestName))
	require.NoError(t, err)
	assert.False(t, exists, "nested manifest at depth 1 must be removed")

	exists, err = vfs.FileExists(fsys, filepath.Join(root, "sub", "nested", ModuleManifestName))
	require.NoError(t, err)
	assert.False(t, exists, "nested manifest at depth 2 must be removed")

	exists, err = vfs.FileExists(fsys, filepath.Join(root, "main.tf"))
	require.NoError(t, err)
	assert.True(t, exists, "non-manifest files must be preserved")

	exists, err = vfs.FileExists(fsys, filepath.Join(root, "sub", "main.tf"))
	require.NoError(t, err)
	assert.True(t, exists, "non-manifest files in subdirs must be preserved")
}

// TestSetupWorkingDirMissingRootIsNoOp verifies setupWorkingDir treats a missing root as a no-op rather than an error.
func TestSetupWorkingDirMissingRootIsNoOp(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()

	require.NoError(t, setupWorkingDir(fsys, "/does-not-exist"))
}
