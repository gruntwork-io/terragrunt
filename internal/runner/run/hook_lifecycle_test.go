package run_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProcessHooks_FiresHooksInDeclarationOrder pins the contract that
// ProcessHooks dispatches hooks in the order they appear in the config,
// not in any other deterministic-but-arbitrary order. Users expect
// before_hook { "a" } and before_hook { "b" } to run a-then-b.
func TestProcessHooks_FiresHooksInDeclarationOrder(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := newHookVenv(rec.handler(vexec.Result{}))
	l := logger.CreateLogger()

	hooks := []runcfg.Hook{
		{Name: "first", Commands: []string{"plan"}, Execute: []string{"step", "1"}, If: true},
		{Name: "second", Commands: []string{"plan"}, Execute: []string{"step", "2"}, If: true},
		{Name: "third", Commands: []string{"plan"}, Execute: []string{"step", "3"}, If: true},
	}

	require.NoError(t, run.ProcessHooks(t.Context(), l, v, hooks, newHookOpts(), &runcfg.RunConfig{}, nil, nil))

	calls := rec.snapshot()
	require.Len(t, calls, 3)
	assert.Equal(t, []string{"1"}, calls[0].Args)
	assert.Equal(t, []string{"2"}, calls[1].Args)
	assert.Equal(t, []string{"3"}, calls[2].Args)
}

// TestProcessHooks_AccumulatesErrorsAcrossHooks pins the contract that
// when an earlier hook fails (but doesn't have RunOnError set), later
// hooks DO NOT run (the per-hook error becomes a prior error for the
// next iteration). RunOnError hooks DO run after a failure.
func TestProcessHooks_AccumulatesErrorsAcrossHooks(t *testing.T) {
	t.Parallel()

	rec := &recorder{}

	h := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		rec.mu.Lock()
		rec.calls = append(rec.calls, vexec.Invocation{Name: inv.Name, Args: append([]string(nil), inv.Args...)})
		rec.mu.Unlock()

		if inv.Name == "failure" {
			return vexec.Result{ExitCode: 1, Stderr: []byte("boom")}
		}

		return vexec.Result{}
	}

	v := newHookVenv(h)
	l := logger.CreateLogger()

	hooks := []runcfg.Hook{
		{Name: "first", Commands: []string{"plan"}, Execute: []string{"failure"}, If: true},
		// Default RunOnError=false: should NOT run because first hook failed.
		{Name: "second", Commands: []string{"plan"}, Execute: []string{"second-cmd"}, If: true},
		// RunOnError=true: SHOULD run despite the prior failure.
		{Name: "third", Commands: []string{"plan"}, Execute: []string{"third-cmd"}, If: true, RunOnError: true},
	}

	err := run.ProcessHooks(t.Context(), l, v, hooks, newHookOpts(), &runcfg.RunConfig{}, nil, nil)
	require.Error(t, err, "hook failure must propagate")

	calls := rec.snapshot()

	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, c.Name)
	}

	assert.Equal(t, []string{"failure", "third-cmd"}, names,
		"second-cmd must be skipped after prior failure (default RunOnError=false); third-cmd must run with RunOnError=true")
}

// TestProcessHooks_PropagatesWorkingDir pins that the hook's WorkingDir
// is the directory the subprocess sees, not the surrounding opts'
// WorkingDir. This matters for hooks that operate in module-relative
// paths (e.g. `before_hook { working_dir = "infra" }`).
func TestProcessHooks_PropagatesWorkingDir(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := newHookVenv(rec.handler(vexec.Result{}))
	l := logger.CreateLogger()

	opts := newHookOpts()
	opts.WorkingDir = "/work/unit"

	hooks := []runcfg.Hook{
		{
			Name:       "hook-in-subdir",
			Commands:   []string{"plan"},
			Execute:    []string{"do-something"},
			WorkingDir: "/work/unit/scripts",
			If:         true,
		},
	}

	require.NoError(t, run.ProcessHooks(t.Context(), l, v, hooks, opts, &runcfg.RunConfig{}, nil, nil))

	calls := rec.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, "/work/unit/scripts", calls[0].Dir, "hook WorkingDir must be the subprocess CWD")
}

// TestProcessErrorHooks_FiresAllMatchingHooks pins the contract that
// multiple error_hooks whose OnErrors regex matches all run when an
// error occurs. There is no first-match-wins semantics.
func TestProcessErrorHooks_FiresAllMatchingHooks(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	exec := vexec.NewMemExec(rec.handler(vexec.Result{}))
	l := logger.CreateLogger()

	priorErrs := new(errors.MultiError).Append(errors.New("AWS: AccessDenied for action s3:GetObject"))

	hooks := []runcfg.ErrorHook{
		{
			Name:     "log-aws-error",
			Commands: []string{"plan"},
			OnErrors: []string{".*AccessDenied.*"},
			Execute:  []string{"logger", "aws"},
		},
		{
			Name:     "notify-s3",
			Commands: []string{"plan"},
			OnErrors: []string{".*s3:.*"},
			Execute:  []string{"notify", "s3"},
		},
		{
			Name:     "non-matching",
			Commands: []string{"plan"},
			OnErrors: []string{".*throttl.*"},
			Execute:  []string{"should-not", "run"},
		},
	}

	require.NoError(t, run.ProcessErrorHooks(t.Context(), l, hookVenv(exec), hooks, newHookOpts(), priorErrs))

	calls := rec.snapshot()
	require.Len(t, calls, 2, "both matching error_hooks must fire")
	assert.Equal(t, "logger", calls[0].Name)
	assert.Equal(t, "notify", calls[1].Name)
}

// TestProcessErrorHooks_AccumulatesFailures pins the contract that when
// multiple error_hooks fire and some of them fail, ProcessErrorHooks
// returns a multi-error rather than short-circuiting on the first
// failure.
func TestProcessErrorHooks_AccumulatesFailures(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if inv.Name == "failing-hook" {
			return vexec.Result{ExitCode: 1, Stderr: []byte("hook broke")}
		}

		return vexec.Result{}
	})

	l := logger.CreateLogger()

	priorErrs := new(errors.MultiError).Append(errors.New("triggering error"))

	hooks := []runcfg.ErrorHook{
		{Name: "fail-1", Commands: []string{"plan"}, OnErrors: []string{".*"}, Execute: []string{"failing-hook"}},
		{Name: "succeed", Commands: []string{"plan"}, OnErrors: []string{".*"}, Execute: []string{"ok"}},
		{Name: "fail-2", Commands: []string{"plan"}, OnErrors: []string{".*"}, Execute: []string{"failing-hook"}},
	}

	err := run.ProcessErrorHooks(t.Context(), l, hookVenv(exec), hooks, newHookOpts(), priorErrs)
	require.Error(t, err, "any failing error_hook must surface as a returned error")
}
