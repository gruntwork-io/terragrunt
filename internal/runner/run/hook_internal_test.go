package run

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getExitError(t *testing.T, exitCode int) *exec.ExitError {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "sh", "-c", fmt.Sprintf("exit %d", exitCode))
	err := cmd.Run()
	require.Error(t, err)

	var exitErr *exec.ExitError

	require.True(t, errors.As(err, &exitErr))

	return exitErr
}

func TestHookErrorMessage_WithStderr(t *testing.T) {
	t.Parallel()

	var output util.CmdOutput
	output.Stderr.WriteString("resource missing required tags")

	err := util.ProcessExecutionError{
		Err:        getExitError(t, 2),
		Command:    "tflint",
		Args:       []string{"--config", ".tflint.hcl"},
		WorkingDir: "/tmp",
		Output:     output,
	}

	msg := hookErrorMessage("my-lint", errors.New(err))
	assert.Contains(t, msg, `Hook "my-lint"`)
	assert.Contains(t, msg, "tflint --config .tflint.hcl")
	assert.Contains(t, msg, "exit code 2")
	assert.Contains(t, msg, "resource missing required tags")
}

func TestHookErrorMessage_StdoutFallback(t *testing.T) {
	t.Parallel()

	var output util.CmdOutput
	output.Stdout.WriteString("warning: deprecated feature")

	err := util.ProcessExecutionError{
		Err:        getExitError(t, 1),
		Command:    "custom-lint",
		Args:       []string{"--fix"},
		WorkingDir: "/tmp",
		Output:     output,
	}

	msg := hookErrorMessage("lint-hook", errors.New(err))
	assert.Contains(t, msg, `Hook "lint-hook"`)
	assert.Contains(t, msg, "custom-lint --fix")
	assert.Contains(t, msg, "exit code 1")
	assert.Contains(t, msg, "warning: deprecated feature")
}

func TestHookErrorMessage_NoOutput(t *testing.T) {
	t.Parallel()

	err := util.ProcessExecutionError{
		Err:        getExitError(t, 3),
		Command:    "check",
		Args:       []string{"-strict"},
		WorkingDir: "/tmp",
	}

	msg := hookErrorMessage("my-hook", errors.New(err))
	assert.Contains(t, msg, `Hook "my-hook"`)
	assert.Contains(t, msg, "check -strict")
	assert.Contains(t, msg, "exit code 3")
}
