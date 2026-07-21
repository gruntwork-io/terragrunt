package getproviders_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tf/getproviders"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const readOnlyLockfileContents = `provider "registry.opentofu.org/hashicorp/null" {
  version     = "3.2.2"
  constraints = "3.2.2"
  hashes = [
    "h1:IMVAUHKoydFrlPrl9OzasDnw/8ntZFerCC9iXw1rXQY=",
  ]
}
`

// TestUpdateLockfileReplacesReadOnlyLockfile verifies that updating a lock
// file CAS materialized as read-only replaces it instead of failing with a
// permission error.
func TestUpdateLockfileReplacesReadOnlyLockfile(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("read-only permission bits are not meaningfully observable on Windows")
	}

	workingDir := helpers.TmpDirWOSymlinks(t)
	lockfilePath := filepath.Join(workingDir, tf.TerraformLockFile)

	require.NoError(t, os.WriteFile(lockfilePath, []byte(readOnlyLockfileContents), 0644))
	require.NoError(t, os.Chmod(lockfilePath, 0444))

	require.NoError(t, getproviders.UpdateLockfile(t.Context(), workingDir, nil))

	content, err := os.ReadFile(lockfilePath)
	require.NoError(t, err)
	assert.Equal(t, readOnlyLockfileContents, string(content))

	info, err := os.Stat(lockfilePath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode().Perm()&0200, "rewritten lock file must be owner-writable")
}

// TestUpdateLockfileDoesNotMutateHardlinkedStore verifies that updating a
// lock file hard-linked into the CAS store breaks the link instead of
// mutating the shared blob.
func TestUpdateLockfileDoesNotMutateHardlinkedStore(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("read-only permission bits are not meaningfully observable on Windows")
	}

	testDir := helpers.TmpDirWOSymlinks(t)
	workingDir := filepath.Join(testDir, "working")
	require.NoError(t, os.Mkdir(workingDir, 0755))

	storePath := filepath.Join(testDir, "store-blob")
	lockfilePath := filepath.Join(workingDir, tf.TerraformLockFile)

	require.NoError(t, os.WriteFile(storePath, []byte(readOnlyLockfileContents), 0644))
	require.NoError(t, os.Chmod(storePath, 0444))
	require.NoError(t, os.Link(storePath, lockfilePath))

	storeInfoBefore, err := os.Stat(storePath)
	require.NoError(t, err)

	require.NoError(t, getproviders.UpdateLockfile(t.Context(), workingDir, nil))

	storeContent, err := os.ReadFile(storePath)
	require.NoError(t, err)
	assert.Equal(
		t,
		readOnlyLockfileContents,
		string(storeContent),
		"store blob content must stay intact",
	)

	storeInfoAfter, err := os.Stat(storePath)
	require.NoError(t, err)
	assert.Equal(
		t,
		os.FileMode(0444),
		storeInfoAfter.Mode().Perm(),
		"store blob must stay read-only",
	)

	targetInfo, err := os.Stat(lockfilePath)
	require.NoError(t, err)
	assert.False(t, os.SameFile(storeInfoBefore, targetInfo),
		"lock file must get a fresh inode instead of sharing the store blob")
	assert.NotZero(t, targetInfo.Mode().Perm()&0200, "rewritten lock file must be owner-writable")
}
