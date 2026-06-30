//go:build exec && windows

// OSExec tests that rely on Windows utilities (cmd.exe, findstr).
// Run with: go test -tags exec ./internal/vexec/...

package vexec_test

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSExec_Output_Windows(t *testing.T) {
	t.Parallel()

	out, err := vexec.Output(vexec.NewOSExec(), t.Context(), "cmd", "/C", "echo hello")

	require.NoError(t, err)
	assert.Equal(t, "hello", strings.TrimSpace(string(out)))
}

func TestOSExec_LookPath_Windows(t *testing.T) {
	t.Parallel()

	e := vexec.NewOSExec()

	path, err := e.LookPath("cmd")
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	_, err = e.LookPath("definitely-not-a-real-binary-xyz123.exe")
	require.Error(t, err)
	assert.ErrorIs(t, err, exec.ErrNotFound)
}

func TestOSExec_ExitCode_Windows(t *testing.T) {
	t.Parallel()

	err := vexec.Run(vexec.NewOSExec(), t.Context(), "cmd", "/C", "exit 3")

	require.Error(t, err)
	assert.Equal(t, 3, vexec.ExitCode(err))
}

func TestOSExec_StartWait_Windows(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	cmd := vexec.NewOSExec().Command(t.Context(), "cmd", "/C", "echo hello")
	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	require.NoError(t, cmd.Start())
	require.NoError(t, cmd.Wait())

	assert.Equal(t, "hello", strings.TrimSpace(stdout.String()))
	assert.Empty(t, stderr.String())
	assert.NotNil(t, cmd.ProcessState())
	assert.True(t, cmd.ProcessState().Success())
}

func TestOSExec_CombinedOutput_Windows(t *testing.T) {
	t.Parallel()

	out, err := vexec.NewOSExec().
		Command(t.Context(), "cmd", "/C", "echo out & echo err 1>&2 & exit 1").
		CombinedOutput()

	require.Error(t, err)
	assert.Equal(t, 1, vexec.ExitCode(err))
	assert.Contains(t, string(out), "out")
	assert.Contains(t, string(out), "err")
}

func TestOSExec_SetDir_Windows(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	var buf bytes.Buffer

	cmd := vexec.NewOSExec().Command(t.Context(), "cmd", "/C", "cd")
	cmd.SetDir(tempDir)
	cmd.SetStdout(&buf)

	require.NoError(t, cmd.Run())
	assert.True(t,
		strings.EqualFold(strings.TrimSpace(buf.String()), tempDir),
		"cwd %q must equal tempdir %q (case-insensitive)", strings.TrimSpace(buf.String()), tempDir,
	)
}

func TestOSExec_SetEnv_Windows(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	// SystemRoot is required for cmd.exe to start on Windows.
	cmd := vexec.NewOSExec().Command(t.Context(), "cmd", "/C", "echo %FOO%")
	cmd.SetEnv([]string{"FOO=bar", "SystemRoot=" + windowsSystemRoot()})
	cmd.SetStdout(&buf)

	require.NoError(t, cmd.Run())
	assert.Equal(t, "bar", strings.TrimSpace(buf.String()))
}

func TestOSExec_SetStdin_Windows(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer

	// findstr "^" matches every line and writes it back to stdout.
	cmd := vexec.NewOSExec().Command(t.Context(), "findstr", "^")
	cmd.SetStdin(strings.NewReader("hello from stdin\n"))
	cmd.SetStdout(&stdout)

	require.NoError(t, cmd.Run())
	assert.Equal(t, "hello from stdin", strings.TrimSpace(stdout.String()))
}

func TestParity_OSVsMem_Windows(t *testing.T) {
	t.Parallel()

	runParityCases(t, []parityCase{
		{
			desc:        "success with stdout",
			name:        "cmd",
			argv:        []string{"/C", "echo hello"},
			wantSuccess: true,
		},
		{
			desc:        "success with empty output",
			name:        "cmd",
			argv:        []string{"/C", "exit 0"},
			wantSuccess: true,
		},
		{
			desc:        "failure with stderr and non-zero exit",
			name:        "cmd",
			argv:        []string{"/C", "echo oops 1>&2 & exit 2"},
			wantSuccess: false,
		},
	})
}

// windowsSystemRoot returns SystemRoot; cmd.exe refuses to start without it.
func windowsSystemRoot() string {
	if v, ok := os.LookupEnv("SystemRoot"); ok {
		return v
	}

	return `C:\Windows`
}
