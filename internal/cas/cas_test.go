package cas_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/stretchr/testify/require"
)

func TestCAS_Clone(t *testing.T) {
	t.Parallel()

	t.Run("clone new repository", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		storePath := filepath.Join(tempDir, "store")
		targetPath := filepath.Join(tempDir, "repo")

		cas, err := cas.New(
			"https://github.com/gruntwork-io/terragrunt.git",
			cas.Options{
				Dir:       targetPath,
				StorePath: storePath,
			},
		)
		require.NoError(t, err)

		err = cas.Clone(context.TODO())
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

		cas, err := cas.New(
			"https://github.com/gruntwork-io/terragrunt.git",
			cas.Options{
				Dir:       targetPath,
				Branch:    "main",
				StorePath: storePath,
			},
		)
		require.NoError(t, err)

		err = cas.Clone(context.TODO())
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

		cas, err := cas.New(
			"https://github.com/gruntwork-io/terragrunt.git",
			cas.Options{
				Dir:              targetPath,
				IncludedGitFiles: []string{"HEAD", "config"},
				StorePath:        storePath,
			},
		)
		require.NoError(t, err)

		err = cas.Clone(context.TODO())
		require.NoError(t, err)

		// Verify repository was cloned
		_, err = os.Stat(filepath.Join(targetPath, ".git", "HEAD"))
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(targetPath, ".git", "config"))
		require.NoError(t, err)
	})
}
