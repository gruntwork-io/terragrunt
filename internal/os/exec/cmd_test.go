package exec_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

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

	cmd := exec.Command(t.Context(), venvtest.New().WithExec(e), "tofu", "plan")
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

// TestCommandDefaultsStreamsToTheVenv pins where an unconfigured command's
// three standard streams come from. A subprocess that prompts must read the
// console the rest of the run reads, not whatever the process was launched
// with, or a driven run blocks on the invoking terminal.
func TestCommandDefaultsStreamsToTheVenv(t *testing.T) {
	t.Parallel()

	var stdin io.Reader

	e := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		stdin = inv.Stdin

		return vexec.Result{Stdout: []byte("out"), Stderr: []byte("err")}
	})

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	v := venvtest.New().
		WithExec(e).
		WithStdin(strings.NewReader("yes\n")).
		WithWriter(stdout).
		WithErrWriter(stderr)

	require.NoError(t, exec.Command(t.Context(), v, "tofu", "apply").Run(logger.CreateLogger()))

	require.NotNil(t, stdin)
	consumed, err := io.ReadAll(stdin)
	require.NoError(t, err)

	assert.Equal(t, "yes\n", string(consumed))
	assert.Equal(t, "out", stdout.String())
	assert.Equal(t, "err", stderr.String())
}

// TestCommandPassesStdinThroughUnwrapped pins that the child gets the venv's
// stdin itself, not a wrapper around it. Anything that buffered in between
// would hold read-ahead no other consumer can reach: os/exec copies a non-file
// stdin into the child's pipe and that copy runs to EOF whether the child reads
// or not, which is enough for one incidental `tofu -version` to swallow input
// that a later prompt, or `hcl fmt --stdin`, still needs.
func TestCommandPassesStdinThroughUnwrapped(t *testing.T) {
	t.Parallel()

	var handed io.Reader

	e := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		handed = inv.Stdin

		return vexec.Result{}
	})

	stdin := strings.NewReader("console input\n")
	v := venvtest.New().WithExec(e).WithStdin(stdin)

	require.NoError(t, exec.Command(t.Context(), v, "tofu", "-version").Run(logger.CreateLogger()))

	assert.Same(t, stdin, handed)
}

// TestCommandWithMemBackendExitCode verifies that handler-reported exit codes
// are recoverable via vexec.ExitCode.
func TestCommandWithMemBackendExitCode(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(context.Context, vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 7}
	})

	cmd := exec.Command(t.Context(), venvtest.New().WithExec(e), "tofu", "apply")

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

	cmd := exec.Command(t.Context(), venvtest.New().WithExec(e), "tofu", "apply")
	cmd.Configure(exec.WithUsePTY(true))

	err := cmd.Run(logger.CreateLogger())
	assert.ErrorIs(t, err, exec.ErrPTYRequiresOSBackend)
}
