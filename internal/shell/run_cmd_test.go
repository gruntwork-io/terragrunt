package shell_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func TestRunShellCommand(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	l := logger.CreateLogger()

	cmd := shell.RunCommand(t.Context(), l, vexec.NewOSExec(), configbridge.ShellRunOptsFromOpts(terragruntOptions), "tofu", "--version")
	require.NoError(t, cmd)

	cmd = shell.RunCommand(t.Context(), l, vexec.NewOSExec(), configbridge.ShellRunOptsFromOpts(terragruntOptions), "tofu", "not-a-real-command")
	require.Error(t, cmd)
}

func TestRunShellOutputToStderrAndStdout(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	terragruntOptions.TerraformCliArgs.AppendFlag("--version")
	terragruntOptions.Writers.Writer = stdout
	terragruntOptions.Writers.ErrWriter = stderr

	l := logger.CreateLogger()

	cmd := shell.RunCommand(t.Context(), l, vexec.NewOSExec(), configbridge.ShellRunOptsFromOpts(terragruntOptions), "tofu", "--version")
	require.NoError(t, cmd)

	assert.Contains(t, stdout.String(), "OpenTofu", "Output directed to stdout")
	assert.Empty(t, stderr.String(), "No output to stderr")

	stdout = new(bytes.Buffer)
	stderr = new(bytes.Buffer)

	terragruntOptions.TerraformCliArgs = iacargs.New()
	terragruntOptions.Writers.Writer = stderr
	terragruntOptions.Writers.ErrWriter = stderr

	cmd = shell.RunCommand(t.Context(), l, vexec.NewOSExec(), configbridge.ShellRunOptsFromOpts(terragruntOptions), "tofu", "--version")
	require.NoError(t, cmd)

	assert.Contains(t, stderr.String(), "OpenTofu", "Output directed to stderr")
	assert.Empty(t, stdout.String(), "No output to stdout")
}

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

func TestGitLevelTopDirCaching(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ctx = cache.ContextWithCache(ctx)
	c := cache.ContextRepoRootCache(ctx, cache.RepoRootCacheContextKey)
	assert.NotNil(t, c)
	assert.Equal(t, 0, c.Len())

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	path := "."
	path1, err := shell.GitTopLevelDir(ctx, l, terragruntOptions.Env, path)
	require.NoError(t, err)
	path2, err := shell.GitTopLevelDir(ctx, l, terragruntOptions.Env, path)
	require.NoError(t, err)
	assert.Equal(t, path1, path2)
	assert.Equal(t, 1, c.Len())
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

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	got, err := shell.GitTopLevelDir(ctx, logger.CreateLogger(), terragruntOptions.Env, subdir)
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

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	got, err := shell.GitTopLevelDir(ctx, logger.CreateLogger(), terragruntOptions.Env, deep)
	if err == nil {
		assert.NotEqual(t, root, got, "guard should not return the outer root when a nested .git exists")
	}
}
