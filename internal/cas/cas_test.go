package cas_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

func TestCAS_Clone(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("clone new repository", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		storePath := filepath.Join(tempDir, "store")
		targetPath := filepath.Join(tempDir, "repo")

		c, err := cas.New(cas.Options{
			StorePath: storePath,
		})
		require.NoError(t, err)

		err = c.Clone(t.Context(), l, &cas.CloneOptions{
			Dir: targetPath,
		}, "https://github.com/gruntwork-io/terragrunt.git")
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
		tempDir := t.TempDir()
		storePath := filepath.Join(tempDir, "store")
		targetPath := filepath.Join(tempDir, "repo")

		c, err := cas.New(cas.Options{
			StorePath: storePath,
		})
		require.NoError(t, err)

		err = c.Clone(t.Context(), l, &cas.CloneOptions{
			Dir:    targetPath,
			Branch: "main",
		}, "https://github.com/gruntwork-io/terragrunt.git")
		require.NoError(t, err)

		// Verify repository was cloned
		_, err = os.Stat(filepath.Join(targetPath, "README.md"))
		require.NoError(t, err)
	})

	t.Run("clone with included git files", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		storePath := filepath.Join(tempDir, "store")
		targetPath := filepath.Join(tempDir, "repo")

		c, err := cas.New(cas.Options{
			StorePath: storePath,
		})
		require.NoError(t, err)

		err = c.Clone(t.Context(), l, &cas.CloneOptions{
			Dir:              targetPath,
			IncludedGitFiles: []string{"HEAD", "config"},
		}, "https://github.com/gruntwork-io/terragrunt.git")
		require.NoError(t, err)

		// Verify repository was cloned
		_, err = os.Stat(filepath.Join(targetPath, ".git", "HEAD"))
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(targetPath, ".git", "config"))
		require.NoError(t, err)
	})
}
