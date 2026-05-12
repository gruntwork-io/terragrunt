package cas_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCAS_Clone(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	repoURL := startTestServer(t)

	t.Run("clone new repository", func(t *testing.T) {
		t.Parallel()
		tempDir := helpers.TmpDirWOSymlinks(t)
		storePath := filepath.Join(tempDir, "store")
		targetPath := filepath.Join(tempDir, "repo")

		c, err := cas.New(cas.WithStorePath(storePath))
		require.NoError(t, err)

		err = c.Clone(t.Context(), l, &cas.CloneOptions{
			Dir:   targetPath,
			Depth: -1,
		}, repoURL)
		require.NoError(t, err)

		// Verify repository was cloned
		_, err = os.Stat(filepath.Join(targetPath, "README.md"))
		require.NoError(t, err)

		// Verify nested files were linked
		_, err = os.Stat(filepath.Join(targetPath, "test", "integration_test.go"))
		require.NoError(t, err)
	})

	t.Run("clone with specific branch", func(t *testing.T) {
		t.Parallel()
		tempDir := helpers.TmpDirWOSymlinks(t)
		storePath := filepath.Join(tempDir, "store")
		targetPath := filepath.Join(tempDir, "repo")

		c, err := cas.New(cas.WithStorePath(storePath))
		require.NoError(t, err)

		err = c.Clone(t.Context(), l, &cas.CloneOptions{
			Dir:    targetPath,
			Branch: "main",
			Depth:  -1,
		}, repoURL)
		require.NoError(t, err)

		// Verify repository was cloned
		_, err = os.Stat(filepath.Join(targetPath, "README.md"))
		require.NoError(t, err)
	})

	t.Run("clone with included git files", func(t *testing.T) {
		t.Parallel()
		tempDir := helpers.TmpDirWOSymlinks(t)
		storePath := filepath.Join(tempDir, "store")
		targetPath := filepath.Join(tempDir, "repo")

		c, err := cas.New(cas.WithStorePath(storePath))
		require.NoError(t, err)

		err = c.Clone(t.Context(), l, &cas.CloneOptions{
			Dir:              targetPath,
			IncludedGitFiles: []string{"HEAD", "config"},
			Depth:            -1,
		}, repoURL)
		require.NoError(t, err)

		// Verify repository was cloned
		_, err = os.Stat(filepath.Join(targetPath, ".git", "HEAD"))
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(targetPath, ".git", "config"))
		require.NoError(t, err)
	})
}

func TestCAS_FallbackWhenGitStoreFails(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")
	targetPath := filepath.Join(tempDir, "repo")

	// Occupy the per-URL bare-repo path with a regular file so the central
	// store cannot create its directory. CAS must still complete the clone via
	// the temporary fallback path.
	gitStoreRoot := filepath.Join(storePath, "git")
	require.NoError(t, os.MkdirAll(gitStoreRoot, 0o755))

	entry := cas.EntryPathForURL(gitStoreRoot, repoURL)
	require.NoError(t, os.WriteFile(entry, []byte("not a directory"), 0o644))

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	err = c.Clone(t.Context(), logger.CreateLogger(), &cas.CloneOptions{
		Dir:   targetPath,
		Depth: -1,
	}, repoURL)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(targetPath, "README.md"))
	require.NoError(t, err)

	// The blocking file must still be in place. The fallback path is allowed
	// to bypass the central store, not to repair it.
	info, err := os.Stat(entry)
	require.NoError(t, err)
	assert.False(t, info.IsDir(), "fallback should not have replaced the blocking file")
}
