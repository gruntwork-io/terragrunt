package shell_test

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/options"
)

func TestRunShellCommand(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	l := logger.CreateLogger()

	cmd := shell.RunCommand(t.Context(), l, terragruntOptions, "tofu", "--version")
	require.NoError(t, cmd)

	cmd = shell.RunCommand(t.Context(), l, terragruntOptions, "tofu", "not-a-real-command")
	require.Error(t, cmd)
}

func TestRunShellOutputToStderrAndStdout(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	terragruntOptions.TerraformCliArgs = append(terragruntOptions.TerraformCliArgs, "--version")
	terragruntOptions.Writer = stdout
	terragruntOptions.ErrWriter = stderr

	l := logger.CreateLogger()

	cmd := shell.RunCommand(t.Context(), l, terragruntOptions, "tofu", "--version")
	require.NoError(t, cmd)

	assert.Contains(t, stdout.String(), "OpenTofu", "Output directed to stdout")
	assert.Empty(t, stderr.String(), "No output to stderr")

	stdout = new(bytes.Buffer)
	stderr = new(bytes.Buffer)

	terragruntOptions.TerraformCliArgs = []string{}
	terragruntOptions.Writer = stderr
	terragruntOptions.ErrWriter = stderr

	cmd = shell.RunCommand(t.Context(), l, terragruntOptions, "tofu", "--version")
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
	c := cache.ContextCache[string](ctx, cache.RunCmdCacheContextKey)
	assert.NotNil(t, c)
	assert.Empty(t, c.Cache)

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	path := "."
	path1, err := shell.GitTopLevelDir(ctx, l, terragruntOptions, path)
	require.NoError(t, err)
	path2, err := shell.GitTopLevelDir(ctx, l, terragruntOptions, path)
	require.NoError(t, err)
	assert.Equal(t, path1, path2)
	assert.Len(t, c.Cache, 1)
}
