package shell_test

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunCommandMemBackendWithRacing verifies that passing a mem-backed
// vexec.Exec into RunCommand intercepts every subprocess invocation so neither
// tofu, terraform, nor any other binary is actually forked.
func TestRunCommandMemBackendWithRacing(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	e := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		calls.Add(1)

		switch {
		case inv.Name == "tofu" && len(inv.Args) > 0 && inv.Args[0] == "plan":
			return vexec.Result{Stdout: []byte("Plan: 0 to add\n")}
		case inv.Name == "terraform" && len(inv.Args) > 0 && inv.Args[0] == "apply":
			return vexec.Result{ExitCode: 1, Stderr: []byte("boom\n")}
		}

		return vexec.Result{ExitCode: 127, Stderr: []byte("unknown\n")}
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	opts := shell.NewShellOptions()

	v := &venv.Venv{
		Exec:    e,
		Env:     map[string]string{},
		Writers: &writer.Writers{Writer: stdout, ErrWriter: stderr},
	}

	l := logger.CreateLogger()

	require.NoError(t, shell.RunCommand(t.Context(), l, v, opts, "tofu", "plan"))
	assert.Contains(t, stdout.String(), "Plan: 0 to add")

	stdout.Reset()
	stderr.Reset()

	err := shell.RunCommand(t.Context(), l, v, opts, "terraform", "apply")
	require.Error(t, err)
	assert.Contains(t, stderr.String(), "boom")

	assert.Equal(t, int32(2), calls.Load(), "expected exactly two intercepted invocations")
}

// TestRunCommandRoutesStdoutAndStderrSeparately pins the contract that
// subprocess stdout writes hit the configured Writer and stderr writes
// hit the configured ErrWriter when the two are distinct buffers, and
// that both stream to the same buffer when Writer == ErrWriter. The
// previous OS-backed test exercised this by spawning real tofu; the
// mem backend lets us inject canned stdout/stderr directly.
func TestRunCommandRoutesStdoutAndStderrSeparately(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{
			Stdout: []byte("out-line\n"),
			Stderr: []byte("err-line\n"),
		}
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	opts := shell.NewShellOptions()

	v := &venv.Venv{
		Exec:    e,
		Env:     map[string]string{},
		Writers: &writer.Writers{Writer: stdout, ErrWriter: stderr},
	}

	require.NoError(t, shell.RunCommand(t.Context(), logger.CreateLogger(), v, opts, "tool"))
	assert.Contains(t, stdout.String(), "out-line", "subprocess stdout must reach Writer")
	assert.Contains(t, stderr.String(), "err-line", "subprocess stderr must reach ErrWriter")
	assert.NotContains(t, stdout.String(), "err-line", "stderr must not leak into Writer when ErrWriter is separate")
	assert.NotContains(t, stderr.String(), "out-line", "stdout must not leak into ErrWriter when Writer is separate")

	// Same buffer for both writers: each line still appears, both in the shared buffer.
	merged := &bytes.Buffer{}
	mergedV := &venv.Venv{
		Exec:    e,
		Env:     map[string]string{},
		Writers: &writer.Writers{Writer: merged, ErrWriter: merged},
	}

	require.NoError(t, shell.RunCommand(t.Context(), logger.CreateLogger(), mergedV, opts, "tool"))
	assert.Contains(t, merged.String(), "out-line")
	assert.Contains(t, merged.String(), "err-line")
}
