package module_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initBareRepoWithTags creates a bare git repo with the given tags and
// returns its path.
func initBareRepoWithTags(t *testing.T, tags []string) string {
	t.Helper()

	bareDir := filepath.Join(t.TempDir(), "remote.git")
	require.NoError(t, os.MkdirAll(bareDir, 0755))

	gitEnv := append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)

	runIn := func(dir string, args ...string) {
		t.Helper()

		cmd := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // test helper, args are hardcoded
		cmd.Dir = dir
		cmd.Env = gitEnv

		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, out)
	}

	runIn(bareDir, "init", "--bare", "--initial-branch=main")

	workDir := t.TempDir()
	runIn(workDir, "clone", bareDir, ".")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# Test\nA test module."), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "main.tf"), []byte("# terraform"), 0644))
	runIn(workDir, "add", ".")
	runIn(workDir, "commit", "-m", "init")

	for _, tag := range tags {
		runIn(workDir, "tag", tag)
	}

	runIn(workDir, "push", "origin", "main", "--tags")

	return bareDir
}

func TestResolveLatestTag_FindsHighestSemver(t *testing.T) {
	t.Parallel()

	bareDir := initBareRepoWithTags(t, []string{"v1.0.0", "v1.10.2", "v1.5.0", "not-semver"})
	l := logger.CreateLogger()

	repo := &module.Repo{
		RemoteURL: bareDir,
	}

	repo.ResolveLatestTag(t.Context(), l)

	assert.Equal(t, "v1.10.2", repo.LatestTag)
}

func TestResolveLatestTag_NoTags(t *testing.T) {
	t.Parallel()

	bareDir := initBareRepoWithTags(t, nil)
	l := logger.CreateLogger()

	repo := &module.Repo{
		RemoteURL: bareDir,
	}

	repo.ResolveLatestTag(t.Context(), l)

	assert.Empty(t, repo.LatestTag)
}

func TestResolveLatestTag_EmptyRemoteURL(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	repo := &module.Repo{}

	repo.ResolveLatestTag(t.Context(), l)

	assert.Empty(t, repo.LatestTag)
}
