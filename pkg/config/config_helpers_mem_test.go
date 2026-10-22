package config_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
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
	v := venvtest.New().WithExec(exec)
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)

	out, err := config.RunCommand(ctx, l, v, pctx, []string{"echoer", "hello"})
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
	v := venvtest.New().WithExec(exec)
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)

	args := []string{"expensive-cmd", "--flag"}

	for range 4 {
		out, err := config.RunCommand(ctx, l, v, pctx, args)
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
	v := venvtest.New().WithExec(exec)
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)

	for range 3 {
		_, err := config.RunCommand(ctx, l, v, pctx, []string{"--terragrunt-no-cache", "cmd"})
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
	v := venvtest.New().WithExec(exec)
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)

	_, err := config.RunCommand(ctx, l, v, pctx, []string{"failing-cmd"})
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
	v := venvtest.New().WithExec(exec)
	ctx, pctxA := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)

	vB := venvtest.New().WithExec(exec)
	_, pctxB := newTestParsingContext(t, t.TempDir())

	args := []string{"--terragrunt-global-cache", "cmd"}

	_, err := config.RunCommand(ctx, l, v, pctxA, args)
	require.NoError(t, err)

	_, err = config.RunCommand(ctx, l, vB, pctxB, args)
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
			v := venvtest.New().WithExec(exec)
			ctx, pctx := newTestParsingContext(t, t.TempDir())
			ctx = config.WithConfigValues(ctx)

			_, err := config.RunCommand(ctx, l, v, pctx, tc.args)
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
	v := venvtest.New().WithExec(exec)
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)

	args := []string{"--terragrunt-quiet", "--terragrunt-global-cache", "cmd", "subarg"}
	want := slices.Clone(args)

	_, err := config.RunCommand(ctx, l, v, pctx, args)
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
	v := venvtest.New().WithExec(exec)
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)

	_, err := config.RunCommand(ctx, l, v, pctx, nil)
	require.Error(t, err)
	require.ErrorAs(t, err, new(config.EmptyStringNotAllowedError))
}

// TestRunCommandReceivesVenvEnv pins that v.Env propagates into the
// subprocess environment via shellRunOptsFromPctx. The mem backend
// exposes the Env slice directly, so a regression that drops env
// propagation is observable here.
func TestRunCommandReceivesVenvEnv(t *testing.T) {
	t.Parallel()

	var got atomic.Value // []string

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		got.Store(append([]string(nil), inv.Env...))
		return vexec.Result{Stdout: []byte("ok\n")}
	})

	l := logger.CreateLogger()
	v := venvtest.New().WithExec(exec)
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)
	v.Env = map[string]string{"TG_TEST_TOKEN": "abc123"}

	_, err := config.RunCommand(ctx, l, v, pctx, []string{"reader"})
	require.NoError(t, err)

	env, _ := got.Load().([]string)
	assert.Contains(
		t,
		env,
		"TG_TEST_TOKEN=abc123",
		"v.Env must propagate to the spawned subprocess environment",
	)
}

// TestRunCommandCacheKeyKeepsArgBoundaries pins that the cache key preserves
// argv element boundaries: ("x y") and ("x", "y") are different commands and
// must not share one cache entry.
func TestRunCommandCacheKeyKeepsArgBoundaries(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		calls.Add(1)
		return vexec.Result{Stdout: []byte(strings.Join(inv.Args, "|") + "\n")}
	})

	l := logger.CreateLogger()
	v := venvtest.New().WithExec(exec)
	ctx, pctx := newTestParsingContext(t, t.TempDir())
	ctx = config.WithConfigValues(ctx)

	joined, err := config.RunCommand(ctx, l, v, pctx, []string{"printf", "%s", "x y"})
	require.NoError(t, err)
	assert.Equal(t, "%s|x y", joined)

	split, err := config.RunCommand(ctx, l, v, pctx, []string{"printf", "%s", "x", "y"})
	require.NoError(t, err)
	assert.Equal(t, "%s|x|y", split, "a different argv must not replay another command's output")

	assert.Equal(t, int32(2), calls.Load(), "distinct argv must not share a cache entry")
}

// TestRunCommandCacheKeyIncludesEnv pins that the cache key covers the
// environment the subprocess inherits. Two units reading one shared file via
// read_terragrunt_config evaluate run_cmd under the same config path. With
// per-unit credentials in v.Env (--auth-provider-cmd), each must get the output
// its own environment produces; identical environments still share one run.
// The same holds under --terragrunt-global-cache.
func TestRunCommandCacheKeyIncludesEnv(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{
			name: "default scope",
			args: []string{"whoami"},
		},
		{
			name: "global cache",
			args: []string{"--terragrunt-global-cache", "whoami"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32

			exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
				calls.Add(1)

				for _, kv := range inv.Env {
					if account, ok := strings.CutPrefix(kv, "ACCOUNT="); ok {
						return vexec.Result{Stdout: []byte(account + "\n")}
					}
				}

				return vexec.Result{ExitCode: 1, Stderr: []byte("ACCOUNT unset\n")}
			})

			l := logger.CreateLogger()
			sharedConfig := filepath.Join(t.TempDir(), "common.hcl")
			ctx, pctx := newTestParsingContext(t, sharedConfig)
			ctx = config.WithConfigValues(ctx)

			unitA := venvtest.New().WithExec(exec).WithEnv(map[string]string{"ACCOUNT": "111"})
			unitB := venvtest.New().WithExec(exec).WithEnv(map[string]string{"ACCOUNT": "222"})

			gotA, err := config.RunCommand(ctx, l, unitA, pctx, tc.args)
			require.NoError(t, err)
			assert.Equal(t, "111", gotA)

			gotB, err := config.RunCommand(ctx, l, unitB, pctx, tc.args)
			require.NoError(t, err)
			assert.Equal(t, "222", gotB, "unit B must not receive unit A's cached output")

			assert.Equal(t, int32(2), calls.Load(), "a different environment must run the command again")

			unitC := venvtest.New().WithExec(exec).WithEnv(map[string]string{"ACCOUNT": "111"})

			gotC, err := config.RunCommand(ctx, l, unitC, pctx, tc.args)
			require.NoError(t, err)
			assert.Equal(t, "111", gotC)
			assert.Equal(t, int32(2), calls.Load(), "an identical environment must reuse the cached result")
		})
	}
}

// TestRunCommandConcurrentMissesRunOnce pins the flight around the cache
// miss window. Goroutines that miss on the same key must run the command once
// and all see that one result, instead of each spawning the subprocess and
// racing to Put. synctest parks every worker
// before the command is released, so the one-call check proves that the
// others joined the in-flight attempt rather than arriving after it finished.
func TestRunCommandConcurrentMissesRunOnce(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const workers = 8

		var calls atomic.Int32

		release := make(chan struct{})

		exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
			calls.Add(1)

			<-release

			return vexec.Result{Stdout: []byte("shared\n")}
		})

		l := logger.CreateLogger()
		ctx, _ := newTestParsingContext(t, t.TempDir())
		ctx = config.WithConfigValues(ctx)

		results := make([]string, workers)

		var wg sync.WaitGroup

		for i := range workers {
			v := venvtest.New().WithExec(exec)
			_, pctx := newTestParsingContext(t, t.TempDir())

			wg.Go(func() {
				out, err := config.RunCommand(ctx, l, v, pctx, []string{"--terragrunt-global-cache", "slow-cmd"})
				assert.NoError(t, err)

				results[i] = out
			})
		}

		synctest.Wait()
		require.Equal(t, int32(1), calls.Load(), "every worker must be parked on one in-flight command")

		close(release)
		wg.Wait()

		assert.Equal(t, int32(1), calls.Load(), "concurrent misses on one key must coalesce into a single run")

		for i, got := range results {
			assert.Equal(t, "shared", got, "worker %d must see the single shared result", i)
		}
	})
}

// TestRunCommandConcurrentFailuresShareOneAttempt pins that concurrent
// callers of one key share a single failing attempt and each receive its error,
// instead of retrying the command one after another. Under
// --terragrunt-global-cache the callers sit in different directories, so a
// waiter's error names its own directory next to the one the command ran in.
// In the default scope the directory is part of the key, so every caller gets
// the plain error. A failure is not stored, so a later independent call runs
// the command again.
func TestRunCommandConcurrentFailuresShareOneAttempt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		args      []string
		sharedDir bool
	}{
		{
			name: "global cache names each waiter's directory",
			args: []string{"--terragrunt-global-cache", "failing-cmd"},
		},
		{
			name:      "default scope returns the leader's error",
			args:      []string{"failing-cmd"},
			sharedDir: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				const workers = 8

				var (
					calls     atomic.Int32
					leaderDir string
				)

				release := make(chan struct{})

				exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
					if calls.Add(1) == 1 {
						leaderDir = inv.Dir
					}

					<-release

					return vexec.Result{ExitCode: 1, Stderr: []byte("boom\n")}
				})

				l := logger.CreateLogger()
				ctx, _ := newTestParsingContext(t, t.TempDir())
				ctx = config.WithConfigValues(ctx)

				sharedDir := t.TempDir()
				dirs := make([]string, workers)
				errs := make([]error, workers)

				var wg sync.WaitGroup

				for i := range workers {
					v := venvtest.New().WithExec(exec)

					dirs[i] = t.TempDir()
					if tc.sharedDir {
						dirs[i] = sharedDir
					}

					_, pctx := newTestParsingContext(t, filepath.Join(dirs[i], "terragrunt.hcl"))

					wg.Go(func() {
						_, errs[i] = config.RunCommand(ctx, l, v, pctx, tc.args)
					})
				}

				synctest.Wait()
				require.Equal(t, int32(1), calls.Load(), "every worker must be parked on one in-flight command")

				close(release)
				wg.Wait()

				assert.Equal(t, int32(1), calls.Load(), "concurrent callers must share one failing attempt")
				assert.Contains(t, dirs, leaderDir, "the command must run in one of the workers' directories")

				for i, err := range errs {
					require.Error(t, err, "worker %d must receive the shared failure", i)
					require.ErrorContains(t, err, "Failed to execute \"failing-cmd\" in "+leaderDir, "worker %d", i)

					var shared config.RunCmdSharedAttemptError

					if dirs[i] == leaderDir {
						assert.NotErrorAs(t, err, &shared, "worker %d shares the leader's directory", i)

						continue
					}

					require.ErrorAs(t, err, &shared, "worker %d", i)
					assert.Equal(t, dirs[i], shared.Dir, "worker %d must see its own directory", i)
				}

				v := venvtest.New().WithExec(exec)
				_, pctx := newTestParsingContext(t, filepath.Join(sharedDir, "terragrunt.hcl"))

				_, err := config.RunCommand(ctx, l, v, pctx, tc.args)
				require.Error(t, err)
				assert.Equal(t, int32(2), calls.Load(), "a failure must not be cached, so a later call runs the command again")
			})
		})
	}
}
