package git_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitRunner_GoLsTreeRecursive(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(t.TempDir())

	err = runner.Clone(ctx, "https://github.com/gruntwork-io/terragrunt.git", true, 1, "main")
	require.NoError(t, err)

	err = runner.GoOpenGitDir()
	require.NoError(t, err)

	defer runner.GoCloseStorage()

	tree, err := runner.GoLsTreeRecursive("HEAD")
	require.NoError(t, err)

	require.NotEmpty(t, tree)
}

func TestGitRunner_GoAdd(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("add single file", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner()
		require.NoError(t, err)

		workDir := t.TempDir()
		runner = runner.WithWorkDir(workDir)

		err = runner.Init(ctx)
		require.NoError(t, err)

		err = runner.GoOpenRepo()
		require.NoError(t, err)

		defer runner.GoCloseStorage()

		// Create a new file
		testFile := filepath.Join(workDir, "test-file.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0644)
		require.NoError(t, err)

		// Add the file
		err = runner.GoAdd("test-file.txt")
		require.NoError(t, err)

		// Verify file is staged by checking worktree status
		s, err := runner.GoStatus()
		require.NoError(t, err)

		fileStatus, ok := s["test-file.txt"]
		require.True(t, ok, "test-file.txt should be in status")
		assert.Equal(t, gogit.Added, fileStatus.Staging)
	})

	t.Run("add multiple files", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner()
		require.NoError(t, err)

		workDir := t.TempDir()
		runner = runner.WithWorkDir(workDir)

		err = runner.Init(t.Context())
		require.NoError(t, err)

		err = runner.GoOpenRepo()
		require.NoError(t, err)

		defer runner.GoCloseStorage()

		// Create multiple files
		testFile1 := filepath.Join(workDir, "test-file-1.txt")
		testFile2 := filepath.Join(workDir, "test-file-2.txt")
		err = os.WriteFile(testFile1, []byte("test content 1"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(testFile2, []byte("test content 2"), 0644)
		require.NoError(t, err)

		// Add both files
		err = runner.GoAdd("test-file-1.txt", "test-file-2.txt")
		require.NoError(t, err)

		// Verify both files are staged
		s, err := runner.GoStatus()
		require.NoError(t, err)

		fileStatus1, ok := s["test-file-1.txt"]
		require.True(t, ok, "test-file-1.txt should be in status")
		assert.Equal(t, gogit.Added, fileStatus1.Staging)

		fileStatus2, ok := s["test-file-2.txt"]
		require.True(t, ok, "test-file-2.txt should be in status")
		assert.Equal(t, gogit.Added, fileStatus2.Staging)
	})

	t.Run("add without open repo fails", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner()
		require.NoError(t, err)

		runner = runner.WithWorkDir(t.TempDir())

		err = runner.GoAdd("test-file.txt")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrNoGoRepo)
	})

	t.Run("add nonexistent file fails", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner()
		require.NoError(t, err)

		workDir := t.TempDir()
		runner = runner.WithWorkDir(workDir)

		err = runner.Init(ctx)
		require.NoError(t, err)

		err = runner.GoOpenRepo()
		require.NoError(t, err)

		defer runner.GoCloseStorage()

		err = runner.GoAdd("nonexistent-file.txt")
		require.Error(t, err)
	})
}

func TestGitRunner_GoCommit(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("commit staged changes", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner()
		require.NoError(t, err)

		workDir := t.TempDir()
		runner = runner.WithWorkDir(workDir)

		err = runner.Init(ctx)
		require.NoError(t, err)

		err = runner.GoOpenRepo()
		require.NoError(t, err)

		defer runner.GoCloseStorage()

		// Create and add a file
		testFile := filepath.Join(workDir, "test-file.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0644)
		require.NoError(t, err)

		err = runner.GoAdd("test-file.txt")
		require.NoError(t, err)

		// Commit the changes
		commitMessage := "test commit"
		err = runner.GoCommit(commitMessage, &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Test Author",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)

		// Verify commit was created
		head, err := runner.GoOpenRepoHead()
		require.NoError(t, err)

		commit, err := runner.GoOpenRepoCommitObject(head.Hash())
		require.NoError(t, err)

		assert.Equal(t, commitMessage, commit.Message)
	})

	t.Run("commit with options", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner()
		require.NoError(t, err)

		workDir := t.TempDir()
		runner = runner.WithWorkDir(workDir)

		err = runner.Init(ctx)
		require.NoError(t, err)

		err = runner.GoOpenRepo()
		require.NoError(t, err)

		defer runner.GoCloseStorage()

		// Create and add a file
		testFile := filepath.Join(workDir, "test-file.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0644)
		require.NoError(t, err)

		err = runner.GoAdd("test-file.txt")
		require.NoError(t, err)

		// Commit with options
		commitMessage := "test commit with options"
		opts := &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Test Author",
				Email: "test@example.com",
			},
		}
		err = runner.GoCommit(commitMessage, opts)
		require.NoError(t, err)

		// Verify commit was created with correct author
		head, err := runner.GoOpenRepoHead()
		require.NoError(t, err)

		commit, err := runner.GoOpenRepoCommitObject(head.Hash())
		require.NoError(t, err)

		assert.Equal(t, commitMessage, commit.Message)
		assert.Equal(t, "Test Author", commit.Author.Name)
		assert.Equal(t, "test@example.com", commit.Author.Email)
	})

	t.Run("commit without open repo fails", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner()
		require.NoError(t, err)

		runner = runner.WithWorkDir(t.TempDir())

		err = runner.GoCommit("test commit", nil)
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrNoGoRepo)
	})

	t.Run("commit without staged changes fails", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner()
		require.NoError(t, err)

		workDir := t.TempDir()
		runner = runner.WithWorkDir(workDir)

		err = runner.Init(ctx)
		require.NoError(t, err)

		err = runner.GoOpenRepo()
		require.NoError(t, err)

		defer runner.GoCloseStorage()

		// Try to commit without options
		err = runner.GoCommit("test commit", &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "cannot create empty commit: clean working tree")
	})

	t.Run("commit without options fails", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner()
		require.NoError(t, err)

		workDir := t.TempDir()
		runner = runner.WithWorkDir(workDir)

		err = runner.Init(ctx)
		require.NoError(t, err)

		err = runner.GoOpenRepo()
		require.NoError(t, err)

		defer runner.GoCloseStorage()

		testFile := filepath.Join(workDir, "test-file.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0644)
		require.NoError(t, err)

		err = runner.GoAdd("test-file.txt")
		require.NoError(t, err)

		// Try to commit without options
		err = runner.GoCommit("test commit", nil)
		require.Error(t, err)
	})
}
