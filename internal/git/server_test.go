package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	t.Parallel()

	t.Run("start and clone", func(t *testing.T) {
		t.Parallel()

		srv, err := git.NewServer()
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Close() })

		require.NoError(t, srv.CommitFile("README.md", []byte("# test repo"), "initial commit"))

		url, err := srv.Start()
		require.NoError(t, err)

		cloneDir := helpers.TmpDirWOSymlinks(t)
		runner, err := git.NewGitRunner()
		require.NoError(t, err)
		runner = runner.WithWorkDir(cloneDir)

		err = runner.Clone(t.Context(), url, false, 0, "")
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(cloneDir, "README.md"))
		require.NoError(t, err)
		assert.Equal(t, "# test repo", string(data))
	})

	t.Run("ls-remote returns HEAD", func(t *testing.T) {
		t.Parallel()

		srv, err := git.NewServer()
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Close() })

		require.NoError(t, srv.CommitFile("file.txt", []byte("content"), "commit"))

		url, err := srv.Start()
		require.NoError(t, err)

		runner, err := git.NewGitRunner()
		require.NoError(t, err)

		results, err := runner.LsRemote(t.Context(), url, "HEAD")
		require.NoError(t, err)
		require.NotEmpty(t, results)
		assert.Regexp(t, "^[0-9a-f]{40}$", results[0].Hash)
	})

	t.Run("multiple files", func(t *testing.T) {
		t.Parallel()

		srv, err := git.NewServer()
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Close() })

		require.NoError(t, srv.CommitFile("a.txt", []byte("aaa"), "add a"))
		require.NoError(t, srv.CommitFile("dir/b.txt", []byte("bbb"), "add b"))

		url, err := srv.Start()
		require.NoError(t, err)

		cloneDir := helpers.TmpDirWOSymlinks(t)
		runner, err := git.NewGitRunner()
		require.NoError(t, err)
		runner = runner.WithWorkDir(cloneDir)

		err = runner.Clone(t.Context(), url, false, 0, "")
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(cloneDir, "a.txt"))
		require.NoError(t, err)
		assert.Equal(t, "aaa", string(data))

		data, err = os.ReadFile(filepath.Join(cloneDir, "dir", "b.txt"))
		require.NoError(t, err)
		assert.Equal(t, "bbb", string(data))
	})

	t.Run("clone bare", func(t *testing.T) {
		t.Parallel()

		srv, err := git.NewServer()
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Close() })

		require.NoError(t, srv.CommitFile("file.txt", []byte("content"), "commit"))

		url, err := srv.Start()
		require.NoError(t, err)

		cloneDir := helpers.TmpDirWOSymlinks(t)
		runner, err := git.NewGitRunner()
		require.NoError(t, err)
		runner = runner.WithWorkDir(cloneDir)

		err = runner.Clone(t.Context(), url, true, 0, "")
		require.NoError(t, err)

		// Bare clone has HEAD file at root
		_, err = os.Stat(filepath.Join(cloneDir, "HEAD"))
		require.NoError(t, err)
	})
}
