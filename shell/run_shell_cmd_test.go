package shell_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/options"
)

func TestRunShellCommand(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	cmd := shell.RunShellCommand(context.Background(), terragruntOptions, "terraform", "--version")
	require.NoError(t, cmd)

	cmd = shell.RunShellCommand(context.Background(), terragruntOptions, "terraform", "not-a-real-command")
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

	cmd := shell.RunShellCommand(context.Background(), terragruntOptions, "terraform", "--version")
	require.NoError(t, cmd)

	assert.True(t, strings.Contains(stdout.String(), "Terraform"), "Output directed to stdout")
	assert.Empty(t, stderr.String(), "No output to stderr")

	stdout = new(bytes.Buffer)
	stderr = new(bytes.Buffer)

	terragruntOptions.TerraformCliArgs = []string{}
	terragruntOptions.Writer = stderr
	terragruntOptions.ErrWriter = stderr

	cmd = shell.RunShellCommand(context.Background(), terragruntOptions, "terraform", "--version")
	require.NoError(t, cmd)

	assert.True(t, strings.Contains(stderr.String(), "Terraform"), "Output directed to stderr")
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
	ctx := context.Background()
	ctx = shell.ContextWithTerraformCommandHook(ctx, nil)
	c := cache.ContextCache[string](ctx, shell.RunCmdCacheContextKey)
	assert.NotNil(t, c)
	assert.Empty(t, c.Cache)
	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	path := "."
	path1, err := shell.GitTopLevelDir(ctx, terragruntOptions, path)
	require.NoError(t, err)
	path2, err := shell.GitTopLevelDir(ctx, terragruntOptions, path)
	require.NoError(t, err)
	assert.Equal(t, path1, path2)
	assert.Len(t, c.Cache, 1)
}
