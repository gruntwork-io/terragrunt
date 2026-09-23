package config_test

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunCommandMemExec exercises the run_cmd HCL helper end-to-end on a
// mem-backed exec. The existing TestTFRunCommand skips on Windows because
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
	ctx, pctx := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())
	ctx = config.WithConfigValues(ctx)

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
	ctx, pctx := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())
	ctx = config.WithConfigValues(ctx)

	args := []string{"expensive-cmd", "--flag"}

	for range 4 {
		out, err := config.RunCommand(ctx, pctx, l, args)
		require.NoError(t, err)
		assert.Equal(t, "computed", out)
	}

	assert.Equal(
		t,
		int32(1),
		calls.Load(),
		"run_cmd cache must collapse repeated invocations to a single subprocess fork",
	)
}

// TestRunCommandConcurrentCallersSpawnOnceWithRacing pins that units
// evaluating the same run_cmd at once share one subprocess. Discovery parses
// units concurrently, and units share a run_cmd result under
// --terragrunt-global-cache or when the call is in a file they read with
// read_terragrunt_config.
//
// The first subprocess keeps running until every caller is blocked, so the
// others arrive while it is still running.
func TestRunCommandConcurrentCallersSpawnOnceWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const numCallers = 2

		var (
			calls   atomic.Int32
			release = make(chan struct{})
		)

		exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
			calls.Add(1)

			<-release

			return vexec.Result{Stdout: []byte("shared\n")}
		})

		l := logger.CreateLogger()
		ctx, pctx := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())
		ctx = config.WithConfigValues(ctx)

		var (
			wg   sync.WaitGroup
			outs = make([]string, numCallers)
			errs = make([]error, numCallers)
		)

		for i := range numCallers {
			wg.Go(func() {
				outs[i], errs[i] = config.RunCommand(ctx, pctx, l, []string{"shared-cmd"})
			})
		}

		synctest.Wait()
		close(release)

		wg.Wait()

		for i := range numCallers {
			require.NoError(t, errs[i])
			assert.Equal(t, "shared", outs[i])
		}

		assert.Equal(t, int32(1), calls.Load())
	})
}

// TestRunCommandWaiterStopsWhenContextEndsWithRacing pins that a caller
// waiting on an identical run_cmd returns its context's error when the
// context ends, while the first run carries on.
func TestRunCommandWaiterStopsWhenContextEndsWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var (
			calls   atomic.Int32
			release = make(chan struct{})
		)

		exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
			calls.Add(1)

			<-release

			return vexec.Result{Stdout: []byte("slow\n")}
		})

		l := logger.CreateLogger()
		ctx, pctx := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())
		ctx = config.WithConfigValues(ctx)

		waiterCtx, cancelWaiter := context.WithCancel(ctx)

		var (
			wg        sync.WaitGroup
			firstOut  string
			firstErr  error
			waiterErr error
		)

		wg.Go(func() {
			firstOut, firstErr = config.RunCommand(ctx, pctx, l, []string{"slow-cmd"})
		})

		synctest.Wait()

		wg.Go(func() {
			_, waiterErr = config.RunCommand(waiterCtx, pctx, l, []string{"slow-cmd"})
		})

		synctest.Wait()
		cancelWaiter()
		synctest.Wait()

		require.ErrorIs(t, waiterErr, context.Canceled)

		close(release)
		wg.Wait()

		require.NoError(t, firstErr)
		assert.Equal(t, "slow", firstOut)
		assert.Equal(t, int32(1), calls.Load())
	})
}

// TestRunCommandFailedRunIsNotSharedWithRacing pins that a caller waiting on
// an identical run_cmd runs the command itself when the first run fails. A
// run can fail for reasons private to its caller, such as a cancelled context.
func TestRunCommandFailedRunIsNotSharedWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var (
			calls   atomic.Int32
			release = make(chan struct{})
		)

		exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
			if calls.Add(1) > 1 {
				return vexec.Result{Stdout: []byte("recovered\n")}
			}

			<-release

			return vexec.Result{ExitCode: 1, Stderr: []byte("transient\n")}
		})

		l := logger.CreateLogger()
		ctx, pctx := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())
		ctx = config.WithConfigValues(ctx)

		var (
			wg        sync.WaitGroup
			firstErr  error
			secondOut string
			secondErr error
		)

		wg.Go(func() {
			_, firstErr = config.RunCommand(ctx, pctx, l, []string{"flaky-cmd"})
		})

		synctest.Wait()

		wg.Go(func() {
			secondOut, secondErr = config.RunCommand(ctx, pctx, l, []string{"flaky-cmd"})
		})

		synctest.Wait()
		close(release)

		wg.Wait()

		require.Error(t, firstErr)
		require.NoError(t, secondErr)
		assert.Equal(t, "recovered", secondOut)
		assert.Equal(t, int32(2), calls.Load())
	})
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
	ctx, pctx := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())
	ctx = config.WithConfigValues(ctx)

	for range 3 {
		_, err := config.RunCommand(ctx, pctx, l, []string{"--terragrunt-no-cache", "cmd"})
		require.NoError(t, err)
	}

	assert.Equal(
		t,
		int32(3),
		calls.Load(),
		"--terragrunt-no-cache must force a subprocess fork every call",
	)
}

// TestRunCommandSurfacesSubprocessFailure pins the contract that a
// non-zero subprocess exit translates to an error from run_cmd.
func TestRunCommandSurfacesSubprocessFailure(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 2, Stderr: []byte("nope\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())
	ctx = config.WithConfigValues(ctx)

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
	ctx, pctxA := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())
	ctx = config.WithConfigValues(ctx)

	_, pctxB := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())

	args := []string{"--terragrunt-global-cache", "cmd"}

	_, err := config.RunCommand(ctx, pctxA, l, args)
	require.NoError(t, err)

	_, err = config.RunCommand(ctx, pctxB, l, args)
	require.NoError(t, err)

	assert.Equal(
		t,
		int32(1),
		calls.Load(),
		"--terragrunt-global-cache must collapse calls across distinct working dirs",
	)
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
			ctx, pctx := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())
			ctx = config.WithConfigValues(ctx)

			_, err := config.RunCommand(ctx, pctx, l, tc.args)
			require.Error(t, err)
			require.ErrorAs(t, err, new(config.ConflictingRunCmdCacheOptionsError))
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
	ctx, pctx := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())
	ctx = config.WithConfigValues(ctx)

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
	ctx, pctx := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())
	ctx = config.WithConfigValues(ctx)

	_, err := config.RunCommand(ctx, pctx, l, nil)
	require.Error(t, err)
	require.ErrorAs(t, err, new(config.EmptyStringNotAllowedError))
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
	ctx, pctx := newTestParsingContext(t, venvtest.New().WithExec(exec), t.TempDir())
	ctx = config.WithConfigValues(ctx)
	pctx.Venv.Env = map[string]string{"TG_TEST_TOKEN": "abc123"}

	_, err := config.RunCommand(ctx, pctx, l, []string{"reader"})
	require.NoError(t, err)

	env, _ := got.Load().([]string)
	assert.Contains(
		t,
		env,
		"TG_TEST_TOKEN=abc123",
		"pctx.Venv.Env must propagate to the spawned subprocess environment",
	)
}
