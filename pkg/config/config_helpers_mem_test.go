package config_test

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunCommandMemExec exercises the run_cmd HCL helper end-to-end on a
// mem-backed exec. The existing TestRunCommand skips on Windows because
// it shells out to /bin/bash; this variant runs everywhere because the
// subprocess is intercepted by the mem backend.
func TestRunCommandMemExec(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		assert.Equal(t, "echoer", inv.Name)
		assert.Equal(t, []string{"hello"}, inv.Args)

		return vexec.Result{Stdout: []byte("hello\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)
	pctx.Venv = &venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec, Writers: writer.Writers{}}

	out, err := config.RunCommand(ctx, pctx, l, []string{"echoer", "hello"})
	require.NoError(t, err)
	assert.Equal(t, "hello", out, "trailing newline must be trimmed from run_cmd output")
}

// TestRunCommandCacheHitsCollapseSubprocessForks pins the run_cmd cache
// invariant: a repeated call with identical args (and the default cache
// scope) must reuse the prior result rather than re-fork the subprocess.
// Without the threaded Venv this was previously testable only by
// scripting an external process to observe its own invocation count.
func TestRunCommandCacheHitsCollapseSubprocessForks(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		calls.Add(1)
		return vexec.Result{Stdout: []byte("computed\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)
	pctx.Venv = &venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec, Writers: writer.Writers{}}

	args := []string{"expensive-cmd", "--flag"}

	for range 4 {
		out, err := config.RunCommand(ctx, pctx, l, args)
		require.NoError(t, err)
		assert.Equal(t, "computed", out)
	}

	assert.Equal(t, int32(1), calls.Load(), "run_cmd cache must collapse repeated invocations to a single subprocess fork")
}

// TestRunCommandNoCacheRefuses pins the contract that
// --terragrunt-no-cache forces re-execution on every call.
func TestRunCommandNoCacheRefuses(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		calls.Add(1)
		return vexec.Result{Stdout: []byte("fresh\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)
	pctx.Venv = &venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec, Writers: writer.Writers{}}

	for range 3 {
		_, err := config.RunCommand(ctx, pctx, l, []string{"--terragrunt-no-cache", "cmd"})
		require.NoError(t, err)
	}

	assert.Equal(t, int32(3), calls.Load(), "--terragrunt-no-cache must force a subprocess fork every call")
}

// TestRunCommandSurfacesSubprocessFailure pins the contract that a
// non-zero subprocess exit translates to an error from run_cmd.
func TestRunCommandSurfacesSubprocessFailure(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 2, Stderr: []byte("nope\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)
	pctx.Venv = &venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec, Writers: writer.Writers{}}

	_, err := config.RunCommand(ctx, pctx, l, []string{"failing-cmd"})
	require.Error(t, err)
}

// TestRunCommandGlobalCacheSharesAcrossWorkingDirs pins that the
// --terragrunt-global-cache flag makes the cache scope path-agnostic:
// two RunCommand calls in different working dirs with the same args
// collapse to a single subprocess fork.
func TestRunCommandGlobalCacheSharesAcrossWorkingDirs(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		calls.Add(1)
		return vexec.Result{Stdout: []byte("shared\n")}
	})

	l := logger.CreateLogger()
	ctx, pctxA := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)
	pctxA.Venv = &venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec, Writers: writer.Writers{}}

	_, pctxB := newTestParsingContext(t, t.TempDir())
	pctxB.Venv = &venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec, Writers: writer.Writers{}}

	args := []string{"--terragrunt-global-cache", "cmd"}

	_, err := config.RunCommand(ctx, pctxA, l, args)
	require.NoError(t, err)

	_, err = config.RunCommand(ctx, pctxB, l, args)
	require.NoError(t, err)

	assert.Equal(t, int32(1), calls.Load(), "--terragrunt-global-cache must collapse calls across distinct working dirs")
}

// TestRunCommandConflictingCacheFlags pins the validation error returned
// when --terragrunt-no-cache and --terragrunt-global-cache are combined.
// The error is surfaced before any subprocess fork, so the test wires
// a Handler that fails if it is ever called.
func TestRunCommandConflictingCacheFlags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{
			name: "no-cache before global-cache",
			args: []string{"--terragrunt-no-cache", "--terragrunt-global-cache", "cmd"},
		},
		{
			name: "global-cache before no-cache",
			args: []string{"--terragrunt-global-cache", "--terragrunt-no-cache", "cmd"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
				assert.Fail(t, "conflicting cache flags must error before any subprocess fork")
				return vexec.Result{}
			})

			l := logger.CreateLogger()
			ctx, pctx := newTestParsingContext(t, t.TempDir())
			ctx = config.WithConfigValues(ctx)
			pctx.Venv = &venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec, Writers: writer.Writers{}}

			_, err := config.RunCommand(ctx, pctx, l, tc.args)
			require.Error(t, err)
			assertErrorType(t, config.ConflictingRunCmdCacheOptionsError{}, err)
		})
	}
}

// TestRunCommandDoesNotMutateCallerArgs pins the contract that
// runCommandImpl clones its args before stripping terragrunt-prefixed
// flags. Without the clone, slices.Delete would shift the caller's
// backing array and a subsequent call (or HCL evaluator re-entry) would
// see post-strip residue.
func TestRunCommandDoesNotMutateCallerArgs(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("ok\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)
	pctx.Venv = &venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec, Writers: writer.Writers{}}

	args := []string{"--terragrunt-quiet", "--terragrunt-global-cache", "cmd", "subarg"}
	want := slices.Clone(args)

	_, err := config.RunCommand(ctx, pctx, l, args)
	require.NoError(t, err)

	assert.Equal(t, want, args, "RunCommand must not mutate the caller's args slice")
}

// TestRunCommandEmptyParamsErrors pins the validation that run_cmd with
// no arguments returns EmptyStringNotAllowedError, again before any
// subprocess fork.
func TestRunCommandEmptyParamsErrors(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		assert.Fail(t, "empty run_cmd args must error before any subprocess fork")
		return vexec.Result{}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)
	pctx.Venv = &venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec, Writers: writer.Writers{}}

	_, err := config.RunCommand(ctx, pctx, l, nil)
	require.Error(t, err)
	assertErrorType(t, config.EmptyStringNotAllowedError(""), err)
}

// TestRunCommandReceivesPctxEnv pins that pctx.Venv.Env propagates into the
// subprocess environment via shellRunOptsFromPctx. The mem backend
// exposes the Env slice directly, so a regression that drops env
// propagation is observable here.
func TestRunCommandReceivesPctxEnv(t *testing.T) {
	t.Parallel()

	var got atomic.Value // []string

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		got.Store(append([]string(nil), inv.Env...))
		return vexec.Result{Stdout: []byte("ok\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)
	pctx.Venv = &venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec, Writers: writer.Writers{}}
	pctx.Venv.Env = map[string]string{"TG_TEST_TOKEN": "abc123"}

	_, err := config.RunCommand(ctx, pctx, l, []string{"reader"})
	require.NoError(t, err)

	env, _ := got.Load().([]string)
	assert.Contains(t, env, "TG_TEST_TOKEN=abc123", "pctx.Venv.Env must propagate to the spawned subprocess environment")
}
