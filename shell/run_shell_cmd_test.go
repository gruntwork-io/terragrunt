package shell

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestRunShellCommand(t *testing.T) {
	t.Parallel()

	terragruntOptions := options.NewTerragruntOptionsForTest("")
	cmd := RunShellCommand(terragruntOptions, "terraform", "--version")
	assert.Nil(t, cmd)

	cmd = RunShellCommand(terragruntOptions, "terraform", "not-a-real-command")
	assert.Error(t, cmd)
}
