package cln_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cln"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCln_Clone(t *testing.T) {
	t.Parallel()

	t.Run("clone new repository", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		storePath := filepath.Join(tempDir, "store")
		targetPath := filepath.Join(tempDir, "repo")

		cln, err := cln.New(
			"https://github.com/yhakbar/cln.git",
			cln.Options{
				Dir:       targetPath,
				StorePath: storePath,
			},
		)
		require.NoError(t, err)

		err = cln.Clone()
		require.NoError(t, err)

		// Verify repository was cloned
		_, err = os.Stat(filepath.Join(targetPath, "README.md"))
		assert.NoError(t, err)
	})

	t.Run("clone with specific branch", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		storePath := filepath.Join(tempDir, "store")
		targetPath := filepath.Join(tempDir, "repo")

		cln, err := cln.New(
			"https://github.com/yhakbar/cln.git",
			cln.Options{
				Dir:       targetPath,
				Branch:    "main",
				StorePath: storePath,
			},
		)
		require.NoError(t, err)

		err = cln.Clone()
		require.NoError(t, err)

		// Verify repository was cloned
		_, err = os.Stat(filepath.Join(targetPath, "README.md"))
		assert.NoError(t, err)
	})
}
