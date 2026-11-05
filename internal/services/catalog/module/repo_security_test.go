package module_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestNewRepoRejectsSymlinkRootBeforeCleanup(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cloneURL := "https://example.com/org/target.git"
	predictableRoot := filepath.Join(base, "catalog-predictable")
	repoName := "target"
	attackerParent := filepath.Join(base, "attacker-parent")
	outsideRepoPath := filepath.Join(attackerParent, repoName)
	require.NoError(t, os.MkdirAll(outsideRepoPath, 0o755))

	sentinel := filepath.Join(outsideRepoPath, "sentinel.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("do not remove\n"), 0o644))
	require.NoError(t, os.Symlink(attackerParent, predictableRoot))

	v := venvtest.NewOSWithEmptyEnv()
	v.HTTP = vhttp.NewNoNetworkClient()

	_, err := module.NewRepo(t.Context(), logger.CreateLogger(), v, &module.RepoOpts{
		CloneURL: cloneURL,
		Path:     predictableRoot,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "symlink")
	assert.FileExists(t, sentinel)
}

func TestNewRepoKeepsDotOnlyRepoNameInsideCloneRoot(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		cloneURL string
	}{
		{name: "double dot", cloneURL: "https://github.com/example/repo//.."},
		{name: "double dot with git suffix", cloneURL: "https://github.com/example/repo/...git"},
		{name: "single dot", cloneURL: "https://github.com/example/repo/."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.NewOSWithEmptyEnv()
			v.HTTP = vhttp.NewNoNetworkClient()

			base := t.TempDir()
			cloneRoot := filepath.Join(base, "catalog-clone")
			expectedPath := filepath.Join(cloneRoot, "temp")

			// Completed clones at the clone root and its parent must not be picked
			// up, only the one in the default "temp" directory.
			for _, dir := range []string{base, cloneRoot, expectedPath} {
				writeFakeRepo(t, v.FS, dir)
				require.NoError(t, os.WriteFile(filepath.Join(dir, module.CloneCompleteSentinel), nil, 0o644))
			}

			repo, err := module.NewRepo(t.Context(), logger.CreateLogger(), v, &module.RepoOpts{
				CloneURL: tc.cloneURL,
				Path:     cloneRoot,
			})
			require.NoError(t, err)
			assert.Equal(t, expectedPath, repo.Path())
		})
	}
}
