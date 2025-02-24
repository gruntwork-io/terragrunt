package clngo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clngo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_CloneAndReuse(t *testing.T) {
	t.Parallel()

	t.Run("clone same repo twice uses store", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		storePath := filepath.Join(tempDir, "store")

		// First clone
		firstClonePath := filepath.Join(tempDir, "first")
		cln1, err := clngo.New(
			"https://github.com/yhakbar/cln.git",
			clngo.Options{
				Dir:       firstClonePath,
				StorePath: storePath,
			},
		)
		require.NoError(t, err)
		require.NoError(t, cln1.Clone())

		// Get info about first clone
		firstReadme := filepath.Join(firstClonePath, "README.md")
		firstStat, err := os.Stat(firstReadme)
		require.NoError(t, err)

		// Second clone
		secondClonePath := filepath.Join(tempDir, "second")
		cln2, err := clngo.New(
			"https://github.com/yhakbar/cln.git",
			clngo.Options{
				Dir:       secondClonePath,
				StorePath: storePath,
			},
		)
		require.NoError(t, err)
		require.NoError(t, cln2.Clone())

		// Get info about second clone
		secondReadme := filepath.Join(secondClonePath, "README.md")
		secondStat, err := os.Stat(secondReadme)
		require.NoError(t, err)

		// Verify both files exist
		assert.FileExists(t, firstReadme)
		assert.FileExists(t, secondReadme)

		// Verify they're hard links using os.SameFile instead of comparing entire Stat_t
		assert.True(t, os.SameFile(firstStat, secondStat))
	})

	t.Run("clone with nonexistent branch fails gracefully", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()

		cln, err := clngo.New(
			"https://github.com/yhakbar/cln.git",
			clngo.Options{
				Dir:       filepath.Join(tempDir, "repo"),
				Branch:    "nonexistent-branch",
				StorePath: filepath.Join(tempDir, "store"),
			},
		)
		require.NoError(t, err)

		err = cln.Clone()
		require.Error(t, err)
		assert.ErrorIs(t, err.(*clngo.WrappedError).Err, clngo.ErrNoMatchingReference)
	})

	t.Run("clone with invalid repository fails gracefully", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()

		cln, err := clngo.New(
			"https://github.com/yhakbar/nonexistent-repo.git",
			clngo.Options{
				Dir:       filepath.Join(tempDir, "repo"),
				StorePath: filepath.Join(tempDir, "store"),
			},
		)
		require.NoError(t, err)

		err = cln.Clone()
		require.Error(t, err)
		var wrappedErr *clngo.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, clngo.ErrCommandSpawn)
	})
}

func TestIntegration_TreeStorage(t *testing.T) {
	t.Parallel()

	t.Run("stores tree objects", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		storePath := filepath.Join(tempDir, "store")

		// First clone to populate store
		cln, err := clngo.New(
			"https://github.com/yhakbar/cln.git",
			clngo.Options{
				Dir:       filepath.Join(tempDir, "repo"),
				StorePath: storePath,
			},
		)
		require.NoError(t, err)
		require.NoError(t, cln.Clone())

		// Get the commit hash
		git := clngo.NewGitRunner().WithWorkDir(filepath.Join(tempDir, "repo"))
		results, err := git.LsRemote("https://github.com/yhakbar/cln.git", "HEAD")
		require.NoError(t, err)
		require.NotEmpty(t, results)
		commitHash := results[0].Hash

		// Verify the tree object is stored
		store, err := clngo.NewStore(storePath)
		require.NoError(t, err)
		assert.True(t, store.HasContent(commitHash), "Tree object should be stored")

		// Verify we can read the tree content
		content := clngo.NewContent(store)
		treeData, err := content.Read(commitHash)
		require.NoError(t, err)

		// Parse the tree data to confirm it's valid
		tree, err := clngo.ParseTree(string(treeData), "")
		require.NoError(t, err)
		assert.NotEmpty(t, tree.Entries(), "Tree should have entries")
	})
}
