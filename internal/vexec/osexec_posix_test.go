//go:build exec && !windows

// OSExec tests that rely on POSIX utilities (bash, echo, cat, sleep).
// Run with: go test -tags exec ./internal/vexec/...

package vexec_test

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSExec_Output(t *testing.T) {
	t.Parallel()

	out, err := vexec.Output(vexec.NewOSExec(), t.Context(), "echo", "hello")

	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(out))
}

func TestOSExec_LookPath(t *testing.T) {
	t.Parallel()

	e := vexec.NewOSExec()

	path, err := e.LookPath("sh")
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	_, err = e.LookPath("definitely-not-a-real-binary-xyz123")
	require.Error(t, err)
	assert.ErrorIs(t, err, exec.ErrNotFound)
}

func TestOSExec_ExitCode(t *testing.T) {
	t.Parallel()

	err := vexec.Run(vexec.NewOSExec(), t.Context(), "bash", "-c", "exit 3")

	require.Error(t, err)
	assert.Equal(t, 3, vexec.ExitCode(err))
}

func TestOSExec_StartWait(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	cmd := vexec.NewOSExec().Command(t.Context(), "echo", "hello")
	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	require.NoError(t, cmd.Start())
	require.NoError(t, cmd.Wait())

	assert.Equal(t, "hello\n", stdout.String())
	assert.Empty(t, stderr.String())
	assert.NotNil(t, cmd.ProcessState(), "ProcessState must be populated after Wait")
	assert.True(t, cmd.ProcessState().Success())
}

func TestOSExec_CombinedOutput(t *testing.T) {
	t.Parallel()

	out, err := vexec.NewOSExec().
		Command(t.Context(), "bash", "-c", "echo out; echo err >&2; exit 1").
		CombinedOutput()

	require.Error(t, err)
	assert.Equal(t, 1, vexec.ExitCode(err))
	assert.Contains(t, string(out), "out")
	assert.Contains(t, string(out), "err")
}

func TestOSExec_SetDir(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	var buf bytes.Buffer

	cmd := vexec.NewOSExec().Command(t.Context(), "bash", "-c", "pwd")
	cmd.SetDir(tempDir)
	cmd.SetStdout(&buf)

	require.NoError(t, cmd.Run())

	// macOS symlinks /tmp to /private/tmp; match via suffix.
	assert.True(t,
		strings.HasSuffix(strings.TrimSpace(buf.String()), tempDir),
		"pwd %q must end with tempdir %q", strings.TrimSpace(buf.String()), tempDir,
	)
}

func TestOSExec_SetEnv(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	cmd := vexec.NewOSExec().Command(t.Context(), "bash", "-c", `printf %s "$FOO"`)
	cmd.SetEnv([]string{"FOO=bar"})
	cmd.SetStdout(&buf)

	require.NoError(t, cmd.Run())
	assert.Equal(t, "bar", buf.String())
}

func TestOSExec_SetStdin(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer

	cmd := vexec.NewOSExec().Command(t.Context(), "cat")
	cmd.SetStdin(strings.NewReader("hello from stdin"))
	cmd.SetStdout(&stdout)

	require.NoError(t, cmd.Run())
	assert.Equal(t, "hello from stdin", stdout.String())
}

// ProcessState parity is deliberately not asserted: osCmd populates it after
// Wait, memCmd always returns nil. Callers that need it should not migrate.
func TestParity_OSVsMem(t *testing.T) {
	t.Parallel()

	runParityCases(t, []parityCase{
		{
			desc:        "success with stdout",
			name:        "echo",
			argv:        []string{"hello"},
			wantSuccess: true,
		},
		{
			desc:        "success with empty output",
			name:        "bash",
			argv:        []string{"-c", "true"},
			wantSuccess: true,
		},
		{
			desc:        "failure with stderr and non-zero exit",
			name:        "bash",
			argv:        []string{"-c", "echo oops >&2; exit 2"},
			wantSuccess: false,
		},
	})
}

func TestOSExec_SetCancelFiresOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var called atomic.Bool

	cmd := vexec.NewOSExec().Command(ctx, "sleep", "30")
	cmd.SetCancel(func() error {
		called.Store(true)
		return cmd.Signal(syscall.SIGINT)
	})

	require.NoError(t, cmd.Start())

	cancel()

	_ = cmd.Wait()

	assert.True(t, called.Load(), "cancel fn must have been invoked")
}

func TestOSExec_SignalBeforeStartReturnsErrProcessNotStarted(t *testing.T) {
	t.Parallel()

	cmd := vexec.NewOSExec().Command(t.Context(), "echo", "hi")

	err := cmd.Signal(syscall.SIGTERM)
	require.ErrorIs(t, err, vexec.ErrProcessNotStarted)
}
