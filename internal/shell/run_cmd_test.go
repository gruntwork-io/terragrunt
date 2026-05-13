package shell_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLastReleaseTag(t *testing.T) {
	t.Parallel()

	var tags = []string{
		"refs/tags/v0.0.1",
		"refs/tags/v0.0.2",
		"refs/tags/v0.10.0",
		"refs/tags/v20.0.1",
		"refs/tags/v0.3.1",
		"refs/tags/v20.1.2",
		"refs/tags/v0.5.1",
	}

	lastTag := shell.LastReleaseTag(tags)
	assert.NotEmpty(t, lastTag)
	assert.Equal(t, "v20.1.2", lastTag)
}

// TestGitTopLevelDirPrefixHit asserts that a descendant query is served from
// the cache. The seeded root is synthetic, so a non-cached answer would have
// to come from `git rev-parse` and would not equal the seeded root.
func TestGitTopLevelDirPrefixHit(t *testing.T) {
	t.Parallel()

	root := helpers.TmpDirWOSymlinks(t)
	subdir := filepath.Join(root, "a", "b", "c")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	ctx := cache.ContextWithCache(t.Context())
	c := cache.ContextRepoRootCache(ctx, cache.RepoRootCacheContextKey)
	c.Add(ctx, root)

	got, err := shell.GitTopLevelDir(ctx, logger.CreateLogger(), venv.OSVenv(), subdir)
	require.NoError(t, err)
	assert.Equal(t, root, got)
}

// TestGitTopLevelDirNestedRepoBypass asserts that a `.git` entry between the
// query path and the cached root forces a fallthrough to `git`. The synthetic
// outer root is not a real repo, so the test passes whether `git` errors or
// returns a different root, as long as the outer root is not returned.
func TestGitTopLevelDirNestedRepoBypass(t *testing.T) {
	t.Parallel()

	root := helpers.TmpDirWOSymlinks(t)
	nested := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(filepath.Join(nested, ".git"), 0o755))

	deep := filepath.Join(nested, "inner")
	require.NoError(t, os.MkdirAll(deep, 0o755))

	ctx := cache.ContextWithCache(t.Context())
	c := cache.ContextRepoRootCache(ctx, cache.RepoRootCacheContextKey)
	c.Add(ctx, root)

	got, err := shell.GitTopLevelDir(ctx, logger.CreateLogger(), venv.OSVenv(), deep)
	if err == nil {
		assert.NotEqual(t, root, got, "guard should not return the outer root when a nested .git exists")
	}
}
