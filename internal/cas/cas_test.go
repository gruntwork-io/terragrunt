package cas_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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

// TestCAS_CloneRepoWithSymlink pins the fix for the stored-blob permission
// bug: a git symlink entry (mode 120000) has no unix permission bits, so the
// permission-only view masks to 0. Without a fallback, the blob holding the
// symlink target was chmodded unreadable, and materializing the symlink later
// failed because linkTree could not read the target string.
func TestCAS_CloneRepoWithSymlink(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("real.txt", []byte("hello"), "add real.txt"))
	require.NoError(t, srv.CommitSymlink("link.txt", "real.txt", "add symlink"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")
	targetPath := filepath.Join(tempDir, "repo")

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	err = c.Clone(t.Context(), logger.CreateLogger(), &cas.CloneOptions{
		Dir:   targetPath,
		Depth: -1,
	}, repoURL)
	require.NoError(t, err)

	linkPath := filepath.Join(targetPath, "link.txt")

	info, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "link.txt is not a symlink (mode=%s)", info.Mode())

	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	assert.Equal(t, "real.txt", target)

	// Reading through the symlink exercises the stored-blob readability that
	// the fix preserves: the target file is hard-linked from the CAS, and the
	// symlink resolution had to read the symlink-blob's contents (the target
	// string) from a stored blob whose permission bits derived from gitPerm=0.
	data, err := os.ReadFile(linkPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

// TestCASRejectsNonOSFilesystem pins the early OS-filesystem gate
// in [cas.New]: a non-OS backing must fail at construction.
func TestCASRejectsNonOSFilesystem(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")

	_, err := cas.New(cas.WithFS(vfs.NewMemMapFS()), cas.WithStorePath(storePath))
	require.ErrorIs(t, err, cas.ErrGitStoreFSNotOS)
}
