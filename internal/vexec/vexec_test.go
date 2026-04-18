package vexec_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSExec_Output(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	out, err := vexec.Output(vexec.NewOSExec(), t.Context(), "echo", "hello")

	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(out))
}

func TestOSExec_LookPath(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

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
	skipOnWindows(t)

	err := vexec.Run(vexec.NewOSExec(), t.Context(), "bash", "-c", "exit 3")

	require.Error(t, err)
	assert.Equal(t, 3, vexec.ExitCode(err))
}

func TestMemExec_HandlerReceivesInvocation(t *testing.T) {
	t.Parallel()

	var got vexec.Invocation

	e := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		got = inv
		buf, _ := io.ReadAll(inv.Stdin)

		return vexec.Result{Stdout: buf}
	})

	cmd := e.Command(t.Context(), "tofu", "plan", "-lock=false")
	cmd.SetEnv([]string{"FOO=bar"})
	cmd.SetDir("/work")
	cmd.SetStdin(strings.NewReader("input"))

	out, err := cmd.Output()
	require.NoError(t, err)

	assert.Equal(t, "tofu", got.Name)
	assert.Equal(t, []string{"plan", "-lock=false"}, got.Args)
	assert.Equal(t, []string{"FOO=bar"}, got.Env)
	assert.Equal(t, "/work", got.Dir)
	assert.Equal(t, []byte("input"), out)
}

func TestMemExec_CombinedOutput(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{
			Stdout: []byte("out-"),
			Stderr: []byte("err"),
		}
	})

	combined, err := vexec.CombinedOutput(e, t.Context(), "whatever")
	require.NoError(t, err)
	assert.Equal(t, []byte("out-err"), combined)
}

func TestMemExec_ExitCode(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 2}
	})

	err := vexec.Run(e, t.Context(), "fail")
	require.Error(t, err)
	assert.Equal(t, 2, vexec.ExitCode(err))
}

func TestMemExec_ResultErrShortCircuits(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("handler boom")

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Err: sentinel}
	})

	err := vexec.Run(e, t.Context(), "whatever")
	require.ErrorIs(t, err, sentinel)
}

func TestMemExec_StartWaitSplit(t *testing.T) {
	t.Parallel()

	calls := 0

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		calls++
		return vexec.Result{Stdout: []byte("ok")}
	})

	var buf bytes.Buffer

	cmd := e.Command(t.Context(), "whatever")
	cmd.SetStdout(&buf)

	require.NoError(t, cmd.Start())
	require.Equal(t, 1, calls)

	require.NoError(t, cmd.Wait())
	assert.Equal(t, "ok", buf.String())

	require.ErrorIs(t, cmd.Wait(), vexec.ErrAlreadyWaited)
	require.ErrorIs(t, cmd.Start(), vexec.ErrAlreadyStarted)
}

func TestMemExec_WaitBeforeStart(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{}
	})

	cmd := e.Command(t.Context(), "whatever")

	err := cmd.Wait()
	require.ErrorIs(t, err, vexec.ErrNotStarted)
}

func TestMemExec_OutputErrorsIfStdoutSet(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{}
	})

	cmd := e.Command(t.Context(), "whatever")
	cmd.SetStdout(&bytes.Buffer{})

	_, err := cmd.Output()
	require.ErrorIs(t, err, vexec.ErrStdoutAlreadySet)
}

func TestMemExec_CombinedOutputErrorsIfStreamsSet(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{}
	})

	cmd := e.Command(t.Context(), "whatever")
	cmd.SetStderr(&bytes.Buffer{})

	_, err := cmd.CombinedOutput()
	require.ErrorIs(t, err, vexec.ErrStderrAlreadySet)
}

func TestMemExec_LookPathDefault(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{}
	})

	path, err := e.LookPath("foo")
	require.NoError(t, err)
	assert.Equal(t, "foo", path)
}

func TestMemExec_LookPathOverride(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(
		func(_ context.Context, _ vexec.Invocation) vexec.Result { return vexec.Result{} },
		vexec.WithLookPath(func(file string) (string, error) {
			if file == "tofu" {
				return "/usr/local/bin/tofu", nil
			}

			return "", exec.ErrNotFound
		}),
	)

	path, err := e.LookPath("tofu")
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/tofu", path)

	_, err = e.LookPath("terraform")
	require.ErrorIs(t, err, exec.ErrNotFound)
}

func TestMemExec_NilHandlerPanics(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		vexec.NewMemExec(nil)
	})
}

func TestNoLookPathExec(t *testing.T) {
	t.Parallel()

	inner := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("hi")}
	})

	e := &vexec.NoLookPathExec{Exec: inner}

	_, err := e.LookPath("anything")
	require.Error(t, err)

	var execErr *exec.Error
	require.ErrorAs(t, err, &execErr)
	require.ErrorIs(t, execErr.Err, exec.ErrNotFound)

	out, err := vexec.Output(e, t.Context(), "anything")
	require.NoError(t, err)
	assert.Equal(t, []byte("hi"), out)
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, vexec.ExitCode(nil))
	assert.Equal(t, -1, vexec.ExitCode(errors.New("plain error")))
}

func TestMemExec_ExitErrorMessage(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 7}
	})

	err := vexec.Run(e, t.Context(), "fail")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "7")
}

func TestMemExec_ProcessStateIsNil(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("ok")}
	})

	cmd := e.Command(t.Context(), "whatever")
	require.NoError(t, cmd.Run())
	assert.Nil(t, cmd.ProcessState(), "memCmd must always report nil ProcessState")
}

// TestOSExec_StartWait exercises the Start/Wait split on the real backend
// and verifies that stdout/stderr writers are wired up and ProcessState is
// populated after Wait returns.
func TestOSExec_StartWait(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

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

// TestOSExec_CombinedOutput exercises CombinedOutput on the real backend
// against a command that writes to both stdout and stderr and exits non-zero.
func TestOSExec_CombinedOutput(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	out, err := vexec.NewOSExec().
		Command(t.Context(), "bash", "-c", "echo out; echo err >&2; exit 1").
		CombinedOutput()

	require.Error(t, err)
	assert.Equal(t, 1, vexec.ExitCode(err))
	assert.Contains(t, string(out), "out")
	assert.Contains(t, string(out), "err")
}

// TestOSExec_SetDir verifies SetDir changes the working directory of the
// spawned process by comparing bash's `pwd` against a tempdir.
func TestOSExec_SetDir(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

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

// TestOSExec_SetEnv verifies SetEnv propagates variables to the child process.
func TestOSExec_SetEnv(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	var buf bytes.Buffer

	cmd := vexec.NewOSExec().Command(t.Context(), "bash", "-c", `printf %s "$FOO"`)
	cmd.SetEnv([]string{"FOO=bar"})
	cmd.SetStdout(&buf)

	require.NoError(t, cmd.Run())
	assert.Equal(t, "bar", buf.String())
}

// TestOSExec_SetStdin verifies SetStdin pipes bytes into the child process
// via cat, which echoes stdin to stdout.
func TestOSExec_SetStdin(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	var stdout bytes.Buffer

	cmd := vexec.NewOSExec().Command(t.Context(), "cat")
	cmd.SetStdin(strings.NewReader("hello from stdin"))
	cmd.SetStdout(&stdout)

	require.NoError(t, cmd.Run())
	assert.Equal(t, "hello from stdin", stdout.String())
}

// TestParity_OSVsMem asserts that osCmd and memCmd are observably
// interchangeable for the behaviors most callers rely on: success/failure
// signaling, exit-code extraction, and stdout/stderr wiring. For each case
// we run the real command first, then replay its captured output through
// memCmd via a handler — identical observable behavior is the contract.
//
// Not asserted: ProcessState parity. osCmd returns a populated *os.ProcessState
// after Wait; memCmd always returns nil (documented). Callers that need
// ProcessState should not migrate to memCmd.
func TestParity_OSVsMem(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

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

type parityCase struct {
	desc        string
	name        string
	argv        []string
	wantSuccess bool
}

func runParityCases(t *testing.T, cases []parityCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			// Run real command, capture stdout/stderr/exit separately so the
			// handler can replay them faithfully.
			var osStdout, osStderr bytes.Buffer

			osCmd := vexec.NewOSExec().Command(t.Context(), tc.name, tc.argv...)
			osCmd.SetStdout(&osStdout)
			osCmd.SetStderr(&osStderr)
			osErr := osCmd.Run()

			capturedOut := osStdout.Bytes()
			capturedErr := osStderr.Bytes()
			capturedExit := vexec.ExitCode(osErr)

			// Replay through memCmd.
			memExec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
				return vexec.Result{
					Stdout:   capturedOut,
					Stderr:   capturedErr,
					ExitCode: capturedExit,
				}
			})

			var memStdout, memStderr bytes.Buffer

			memCmd := memExec.Command(t.Context(), tc.name, tc.argv...)
			memCmd.SetStdout(&memStdout)
			memCmd.SetStderr(&memStderr)
			memErr := memCmd.Run()

			// Success/failure shape must match.
			assert.Equal(t, tc.wantSuccess, osErr == nil, "os success mismatch")
			assert.Equal(t, tc.wantSuccess, memErr == nil, "mem success mismatch")

			// Exit codes must be extractable identically.
			assert.Equal(t, vexec.ExitCode(osErr), vexec.ExitCode(memErr), "exit code mismatch")

			// Stream wiring must match byte-for-byte (compare as strings to
			// avoid nil-vs-empty []byte distinctions from bytes.Buffer).
			assert.Equal(t, osStdout.String(), memStdout.String(), "stdout mismatch")
			assert.Equal(t, osStderr.String(), memStderr.String(), "stderr mismatch")

			// CombinedOutput parity: replay through a fresh pair and compare.
			osCombined, osCombErr := vexec.NewOSExec().
				Command(t.Context(), tc.name, tc.argv...).
				CombinedOutput()

			memCombined, memCombErr := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
				return vexec.Result{
					Stdout:   capturedOut,
					Stderr:   capturedErr,
					ExitCode: capturedExit,
				}
			}).Command(t.Context(), tc.name, tc.argv...).CombinedOutput()

			assert.Equal(t, tc.wantSuccess, osCombErr == nil)
			assert.Equal(t, tc.wantSuccess, memCombErr == nil)
			assert.Equal(t, vexec.ExitCode(osCombErr), vexec.ExitCode(memCombErr))
			assert.Equal(t, string(osCombined), string(memCombined), "combined output mismatch")
		})
	}
}

// -----------------------------------------------------------------------------
// Windows-specific tests.
//
// These mirror the POSIX osCmd tests above but drive the real backend through
// cmd.exe, which ships on every Windows installation. The in-memory backend
// is platform-agnostic, so the memCmd tests above already cover it on Windows.
// -----------------------------------------------------------------------------

func TestOSExec_Output_Windows(t *testing.T) {
	t.Parallel()
	skipNotOnWindows(t)

	out, err := vexec.Output(vexec.NewOSExec(), t.Context(), "cmd", "/C", "echo hello")

	require.NoError(t, err)
	assert.Equal(t, "hello", strings.TrimSpace(string(out)))
}

func TestOSExec_LookPath_Windows(t *testing.T) {
	t.Parallel()
	skipNotOnWindows(t)

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
	skipNotOnWindows(t)

	err := vexec.Run(vexec.NewOSExec(), t.Context(), "cmd", "/C", "exit 3")

	require.Error(t, err)
	assert.Equal(t, 3, vexec.ExitCode(err))
}

func TestOSExec_StartWait_Windows(t *testing.T) {
	t.Parallel()
	skipNotOnWindows(t)

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
	skipNotOnWindows(t)

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
	skipNotOnWindows(t)

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
	skipNotOnWindows(t)

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
	skipNotOnWindows(t)

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
	skipNotOnWindows(t)

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

// windowsSystemRoot returns the value of SystemRoot (e.g. C:\Windows), which
// cmd.exe requires to start. Falls back to C:\Windows.
func windowsSystemRoot() string {
	if v, ok := os.LookupEnv("SystemRoot"); ok {
		return v
	}

	return `C:\Windows`
}

// skipOnWindows skips tests that depend on POSIX binaries (echo, bash, cat).
func skipOnWindows(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX utilities (echo/bash/cat)")
	}
}

// skipNotOnWindows skips tests that depend on Windows binaries (cmd, findstr).
func skipNotOnWindows(t *testing.T) {
	t.Helper()

	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}
}
