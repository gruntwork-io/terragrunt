package vexec_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSExec_Output(t *testing.T) {
	t.Parallel()

	e := vexec.NewOSExec()

	out, err := vexec.Output(e, t.Context(), "go", "version")

	require.NoError(t, err)
	assert.Contains(t, string(out), "go version")
}

func TestOSExec_LookPath(t *testing.T) {
	t.Parallel()

	e := vexec.NewOSExec()

	path, err := e.LookPath("go")
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	_, err = e.LookPath("definitely-not-a-real-binary-xyz123")
	require.Error(t, err)
	assert.ErrorIs(t, err, exec.ErrNotFound)
}

func TestOSExec_ExitCode(t *testing.T) {
	t.Parallel()

	e := vexec.NewOSExec()

	err := vexec.Run(e, t.Context(), "go", "this-is-not-a-go-subcommand")

	require.Error(t, err)
	assert.NotZero(t, vexec.ExitCode(err))
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
