package git_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/stretchr/testify/require"
)

func BenchmarkGitOperations(b *testing.B) {
	// Setup a git repository for testing
	repoDir := b.TempDir()

	g, err := git.NewGitRunner()
	require.NoError(b, err)

	g = g.WithWorkDir(repoDir)

	ctx := b.Context()

	err = g.Clone(ctx, "https://github.com/gruntwork-io/terragrunt.git", false, 1, "main")
	require.NoError(b, err)

	// This makes it so that the comparison isn't exactly apples to apples, but we're OK with giving the go-git library
	// any advantage it can get.
	err = g.GoOpenGitDir()
	require.NoError(b, err)

	defer g.GoCloseStorage()

	b.Run("ls-remote", func(b *testing.B) {
		for b.Loop() {
			_, err = g.LsRemote(ctx, "https://github.com/gruntwork-io/terragrunt.git", "HEAD")
			require.NoError(b, err)
		}
	})

	b.Run("ls-tree -r", func(b *testing.B) {
		for b.Loop() {
			_, err = g.LsTreeRecursive(ctx, "HEAD")
			require.NoError(b, err)
		}
	})

	b.Run("go-ls-tree -r", func(b *testing.B) {
		for b.Loop() {
			_, err = g.GoLsTreeRecursive("HEAD")
			require.NoError(b, err)
		}
	})
}
