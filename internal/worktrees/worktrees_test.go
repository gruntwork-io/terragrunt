package worktrees_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

func TestNewWorktrees(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	helpers.CreateGitRepo(t, tmpDir)

	gitCommit(t, tmpDir, "Initial commit", "--allow-empty")

	gitCommit(t, tmpDir, "Second commit", "--allow-empty")

	filters, err := filter.ParseFilterQueries([]string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	w, err := worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		tmpDir,
		filters.UniqueGitFilters(),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.Background(), logger.CreateLogger())
		require.NoError(t, cleanupErr)
	})

	require.NotEmpty(t, w.RefsToPaths)
	require.NotEmpty(t, w.GitExpressionsToDiffs)
}

func TestNewWorktreesWithInvalidReference(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Initialize Git repository
	helpers.CreateGitRepo(t, tmpDir)

	gitCommit(t, tmpDir, "Initial commit", "--allow-empty")

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Parse filter with invalid Git reference
	filters, err := filter.ParseFilterQueries([]string{"[nonexistent-branch]"})
	require.NoError(t, err) // Parsing should succeed

	_, err = worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		tmpDir,
		filters.UniqueGitFilters(),
	)
	require.Error(t, err)
}

// TODO: Move this out to the `internal/git` package

// gitCommit creates a Git commit
func gitCommit(t *testing.T, dir, message string, extraArgs ...string) {
	t.Helper()

	args := []string{"commit", "--no-gpg-sign", "-m", message}
	args = append(args, extraArgs...)
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	// Set git config for test environment
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "Error creating git commit: %v\n%s", err, string(output))
}
