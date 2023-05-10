package shell

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

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
	terragruntOptions.Writer = stdout
	terragruntOptions.ErrWriter = stderr

	cmd = RunShellCommand(terragruntOptions, "terraform", "--version")
	assert.Nil(t, cmd)

	assert.True(t, strings.Contains(stderr.String(), "Terraform"), "Output directed to stderr")
	assert.True(t, len(stdout.String()) == 0, "No output to stdout")
}

func TestRunShellCommandWithOutputInterrupt(t *testing.T) {
	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	errCh := make(chan error)
	expectedWait := 5

	go func() {
		_, err := RunShellCommandWithOutput(terragruntOptions, "", false, false, "../testdata/test_sigint_wait.sh", strconv.Itoa(expectedWait))
		errCh <- err
	}()

	time.AfterFunc(3*time.Second, func() {
		syscall.Kill(os.Getpid(), syscall.SIGINT)
	})

	expectedErr := fmt.Sprintf("exit status %d", expectedWait)
	assert.EqualError(t, <-errCh, expectedErr)
}
