package module_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
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

	_, err := module.NewRepo(t.Context(), logger.CreateLogger(), *venv.OSVenv(), &module.RepoOpts{
		CloneURL: cloneURL,
		Path:     predictableRoot,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "symlink")
	assert.FileExists(t, sentinel)
}
