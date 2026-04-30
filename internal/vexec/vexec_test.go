// In-memory backend tests. OS-backed tests live in osexec_*_test.go behind
// the `exec` build tag.

package vexec_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestOSCmder_OSBackendSatisfies(t *testing.T) {
	t.Parallel()

	cmd := vexec.NewOSExec().Command(t.Context(), "some-binary", "arg1")

	oc, ok := cmd.(vexec.OSCmder)
	require.True(t, ok, "OS-backed Cmd must satisfy OSCmder")

	got := oc.OSCmd()
	require.NotNil(t, got)
	assert.Equal(t, []string{"some-binary", "arg1"}, got.Args)
}

func TestOSCmder_PropagatesSetters(t *testing.T) {
	t.Parallel()

	cmd := vexec.NewOSExec().Command(t.Context(), "some-binary", "arg1")

	stdin := strings.NewReader("input")
	cmd.SetStdin(stdin)
	cmd.SetEnv([]string{"FOO=bar"})
	cmd.SetDir("/work")

	got := cmd.(vexec.OSCmder).OSCmd()
	assert.Equal(t, []string{"some-binary", "arg1"}, got.Args)
	assert.Equal(t, []string{"FOO=bar"}, got.Env)
	assert.Equal(t, "/work", got.Dir)
	assert.Same(t, stdin, got.Stdin)
}

func TestOSCmder_MemBackendDoesNot(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{}
	})

	_, ok := e.Command(t.Context(), "echo").(vexec.OSCmder)
	assert.False(t, ok, "MemExec Cmd must not satisfy OSCmder")
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

// synctest bubbles the 25ms sleep so the test runs in fake time.
func TestMemExec_SetCancelFiresOnContextCancel(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		e := vexec.NewMemExec(func(ctx context.Context, _ vexec.Invocation) vexec.Result {
			<-ctx.Done()
			return vexec.Result{}
		})

		var called atomic.Bool

		cmd := e.Command(ctx, "blocker")
		cmd.SetCancel(func() error {
			called.Store(true)
			return nil
		})

		go func() {
			time.Sleep(25 * time.Millisecond)
			cancel()
		}()

		require.NoError(t, cmd.Run())
		assert.True(t, called.Load(), "cancel fn must have been invoked")
	})
}

func TestMemExec_SignalIsNoop(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{}
	})

	cmd := e.Command(t.Context(), "whatever")
	require.NoError(t, cmd.Signal(syscall.SIGTERM))
}
