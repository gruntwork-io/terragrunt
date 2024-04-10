package shell

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestRunShellCommand(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	cmd := RunShellCommand(context.Background(), terragruntOptions, "terraform", "--version")
	assert.Nil(t, cmd)

	cmd = RunShellCommand(context.Background(), terragruntOptions, "terraform", "not-a-real-command")
	assert.Error(t, cmd)
}

func TestRunShellOutputToStderrAndStdout(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	terragruntOptions.TerraformCliArgs = append(terragruntOptions.TerraformCliArgs, "--version")
	terragruntOptions.Writer = stdout
	terragruntOptions.ErrWriter = stderr

	cmd := RunShellCommand(context.Background(), terragruntOptions, "terraform", "--version")
	assert.Nil(t, cmd)

	assert.True(t, strings.Contains(stdout.String(), "Terraform"), "Output directed to stdout")
	assert.True(t, len(stderr.String()) == 0, "No output to stderr")

	stdout = new(bytes.Buffer)
	stderr = new(bytes.Buffer)

	terragruntOptions.TerraformCliArgs = []string{}
	terragruntOptions.Writer = stderr
	terragruntOptions.ErrWriter = stderr

	cmd = RunShellCommand(context.Background(), terragruntOptions, "terraform", "--version")
	assert.Nil(t, cmd)

	assert.True(t, strings.Contains(stderr.String(), "Terraform"), "Output directed to stderr")
	assert.True(t, len(stdout.String()) == 0, "No output to stdout")
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
	lastTag := lastReleaseTag(tags)
	assert.NotEmpty(t, lastTag)
	assert.Equal(t, "v20.1.2", lastTag)
}
