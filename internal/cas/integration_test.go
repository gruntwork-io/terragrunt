package cas_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_CloneAndReuse(t *testing.T) {
	t.Parallel()

	l := log.New()

	t.Run("clone same repo twice uses store", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		storePath := filepath.Join(tempDir, "store")

		// First clone
		firstClonePath := filepath.Join(tempDir, "first")
		cas1, err := cas.New(cas.Options{
			StorePath: storePath,
		})
		require.NoError(t, err)
		require.NoError(t, cas1.Clone(context.TODO(), &l, cas.CloneOptions{
			Dir: firstClonePath,
		}, "https://github.com/gruntwork-io/terragrunt.git"))

		// Get info about first clone
		firstReadme := filepath.Join(firstClonePath, "README.md")
		firstStat, err := os.Stat(firstReadme)
		require.NoError(t, err)

		// Second clone
		secondClonePath := filepath.Join(tempDir, "second")
		cas2, err := cas.New(cas.Options{
			StorePath: storePath,
		})
		require.NoError(t, err)
		require.NoError(t, cas2.Clone(context.TODO(), &l, cas.CloneOptions{
			Dir: secondClonePath,
		}, "https://github.com/gruntwork-io/terragrunt.git"))

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

		c, err := cas.New(cas.Options{
			StorePath: filepath.Join(tempDir, "store"),
		})
		require.NoError(t, err)

		err = c.Clone(context.TODO(), &l, cas.CloneOptions{
			Dir:    filepath.Join(tempDir, "repo"),
			Branch: "nonexistent-branch",
		}, "https://github.com/gruntwork-io/terragrunt.git")
		require.Error(t, err)
		var wrappedErr *cas.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, cas.ErrNoMatchingReference)
	})

	t.Run("clone with invalid repository fails gracefully", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()

		c, err := cas.New(cas.Options{
			StorePath: filepath.Join(tempDir, "store"),
		})
		require.NoError(t, err)

		err = c.Clone(context.TODO(), &l, cas.CloneOptions{
			Dir: filepath.Join(tempDir, "repo"),
		}, "https://github.com/yhakbar/nonexistent-repo.git")
		require.Error(t, err)
		var wrappedErr *cas.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, cas.ErrCommandSpawn)
	})
}

func TestIntegration_TreeStorage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	l := log.New()

	t.Run("stores tree objects", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		storePath := filepath.Join(tempDir, "store")

		// First clone to populate store
		c, err := cas.New(cas.Options{
			StorePath: storePath,
		})
		require.NoError(t, err)
		require.NoError(t, c.Clone(ctx, &l, cas.CloneOptions{
			Dir: filepath.Join(tempDir, "repo"),
		}, "https://github.com/gruntwork-io/terragrunt.git"))

		// Get the commit hash
		git := cas.NewGitRunner().WithWorkDir(filepath.Join(tempDir, "repo"))
		results, err := git.LsRemote(ctx, "https://github.com/gruntwork-io/terragrunt.git", "HEAD")
		require.NoError(t, err)
		require.NotEmpty(t, results)
		commitHash := results[0].Hash

		// Verify the tree object is stored
		store := cas.NewStore(storePath)
		require.NoError(t, err)
		assert.False(t, store.NeedsWrite(commitHash, time.Now()), "Tree object should be stored")

		// Verify we can read the tree content
		content := cas.NewContent(store)
		treeData, err := content.Read(commitHash)
		require.NoError(t, err)

		// Parse the tree data to confirm it's valid
		tree, err := cas.ParseTree(string(treeData), "")
		require.NoError(t, err)
		assert.NotEmpty(t, tree.Entries(), "Tree should have entries")
	})
}
