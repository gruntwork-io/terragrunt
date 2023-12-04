package shell

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestRunShellCommand(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	cmd := RunShellCommand(terragruntOptions, "terraform", "--version")
	assert.Nil(t, cmd)

	cmd = RunShellCommand(terragruntOptions, "terraform", "not-a-real-command")
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

	cmd := RunShellCommand(terragruntOptions, "terraform", "--version")
	assert.Nil(t, cmd)

	assert.True(t, strings.Contains(stdout.String(), "Terraform"), "Output directed to stdout")
	assert.True(t, len(stderr.String()) == 0, "No output to stderr")

	stdout = new(bytes.Buffer)
	stderr = new(bytes.Buffer)

	terragruntOptions.TerraformCliArgs = []string{}
	terragruntOptions.Writer = stderr
	terragruntOptions.ErrWriter = stderr

	cmd = RunShellCommand(terragruntOptions, "terraform", "--version")
	assert.Nil(t, cmd)

	assert.True(t, strings.Contains(stderr.String(), "Terraform"), "Output directed to stderr")
	assert.True(t, len(stdout.String()) == 0, "No output to stdout")
}

func TestGitLevelTopDirCaching(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	path := "./"

	cmd, err := RunShellCommandWithOutput(terragruntOptions, path, true, false, "git", "rev-parse", "--show-toplevel")
	assert.NoError(t, err)
	expectedResult := strings.TrimSpace(cmd.Stdout)

	actualResult, err := GitTopLevelDir(terragruntOptions, path)
	assert.NoError(t, err, "Unexpected error executing GitTopLevelDir: %v", err)
	assert.Equal(t, expectedResult, actualResult)

	cachedResult, found := gitTopLevelDirs.Get(path)
	assert.True(t, found)
	assert.Equal(t, expectedResult, cachedResult)

	delete(gitTopLevelDirs.Cache, path)
}

func BenchmarkPerformanceOfGitTopLevelDir(b *testing.B) {
	for i := 0; i <= b.N; i++ {
		terragruntOptions, err := options.NewTerragruntOptionsForTest("")
		assert.NoError(b, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

		_, err = GitTopLevelDir(terragruntOptions, "")
		assert.NoError(b, err, "Unexpected error running GitTopLevelDir: %v", err)
	}
}
