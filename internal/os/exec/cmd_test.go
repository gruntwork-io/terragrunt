package exec_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommandWithMemBackend verifies that the wrapper drives a mem-backed
// vexec.Exec end-to-end without forking a real process.
func TestCommandWithMemBackend(t *testing.T) {
	t.Parallel()

	var got vexec.Invocation

	e := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		got = inv

		return vexec.Result{Stdout: []byte("Plan: 0 to add\n")}
	})

	stdout := &bytes.Buffer{}

	cmd := exec.Command(t.Context(), e, "tofu", "plan")
	cmd.SetStdout(stdout)
	cmd.SetDir("/work")
	cmd.SetEnv([]string{"FOO=bar"})

	require.NoError(t, cmd.Run(logger.CreateLogger()))

	assert.Equal(t, "tofu", got.Name)
	assert.Equal(t, []string{"plan"}, got.Args)
	assert.Equal(t, "/work", got.Dir)
	assert.Equal(t, []string{"FOO=bar"}, got.Env)
	assert.Equal(t, "Plan: 0 to add\n", stdout.String())
	assert.Equal(t, "/work", cmd.Dir())
}

// TestCommandWithMemBackendExitCode verifies that handler-reported exit codes
// are recoverable via vexec.ExitCode.
func TestCommandWithMemBackendExitCode(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(context.Context, vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 7}
	})

	cmd := exec.Command(t.Context(), e, "tofu", "apply")

	err := cmd.Run(logger.CreateLogger())
	require.Error(t, err)

	assert.Equal(t, 7, vexec.ExitCode(err))
}

// TestCommandWithMemBackendPTYRejected verifies that requesting a PTY against
// a non-OS backend is refused at Start, rather than silently degrading.
func TestCommandWithMemBackendPTYRejected(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(context.Context, vexec.Invocation) vexec.Result {
		return vexec.Result{}
	})

	cmd := exec.Command(t.Context(), e, "tofu", "apply")
	cmd.Configure(exec.WithUsePTY(true))

	err := cmd.Run(logger.CreateLogger())
	assert.ErrorIs(t, err, exec.ErrPTYRequiresOSBackend)
}
