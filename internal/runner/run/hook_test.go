package run_test

import (
	"context"
	"io"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessHooks_DispatchesCommandsForMatchingTerraformCommand(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := newHookVenv(rec.handler(vexec.Result{}))
	l := logger.CreateLogger()

	hooks := []runcfg.Hook{
		{
			Name:     "echo-hook",
			Commands: []string{"plan"},
			Execute:  []string{"echo", "before-plan"},
			If:       true,
		},
		{
			Name:     "apply-only",
			Commands: []string{"apply"},
			Execute:  []string{"echo", "should-not-run"},
			If:       true,
		},
	}

	err := run.ProcessHooks(t.Context(), l, v, hooks, newHookOpts(), &runcfg.RunConfig{}, nil, nil)
	require.NoError(t, err)

	calls := rec.snapshot()
	require.Len(t, calls, 1, "only the plan hook should fire on a plan command")
	assert.Equal(t, "echo", calls[0].Name)
	assert.Equal(t, []string{"before-plan"}, calls[0].Args)
}

func TestProcessHooks_SkipsHookWhenIfFalse(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := newHookVenv(rec.handler(vexec.Result{}))
	l := logger.CreateLogger()

	hooks := []runcfg.Hook{
		{
			Name:     "gated",
			Commands: []string{"plan"},
			Execute:  []string{"echo", "skip-me"},
			If:       false,
		},
	}

	require.NoError(t, run.ProcessHooks(t.Context(), l, v, hooks, newHookOpts(), &runcfg.RunConfig{}, nil, nil))
	assert.Empty(t, rec.snapshot(), "If=false should suppress dispatch")
}

func TestProcessHooks_RunOnErrorGate(t *testing.T) {
	t.Parallel()

	priorErr := new(errors.MultiError).Append(errors.New("prior failure"))

	testCases := []struct {
		name        string
		hook        runcfg.Hook
		wantInvoked bool
	}{
		{
			name: "default behavior: skip after prior error",
			hook: runcfg.Hook{
				Name: "after", Commands: []string{"plan"},
				Execute: []string{"echo", "skipped"}, If: true,
			},
			wantInvoked: false,
		},
		{
			name: "RunOnError opts back in",
			hook: runcfg.Hook{
				Name: "after", Commands: []string{"plan"},
				Execute: []string{"echo", "ran"}, If: true, RunOnError: true,
			},
			wantInvoked: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			v := newHookVenv(rec.handler(vexec.Result{}))
			l := logger.CreateLogger()

			require.NoError(t, run.ProcessHooks(
				t.Context(), l, v, []runcfg.Hook{tc.hook},
				newHookOpts(), &runcfg.RunConfig{}, priorErr, nil,
			))

			invoked := len(rec.snapshot()) == 1
			assert.Equal(t, tc.wantInvoked, invoked)
		})
	}
}

func TestProcessHooks_InjectsHookContextEnv(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := newHookVenv(rec.handler(vexec.Result{}))
	l := logger.CreateLogger()

	opts := newHookOpts()
	opts.TFPath = "/fake/tofu"
	opts.TerraformCommand = "plan"

	hooks := []runcfg.Hook{
		{
			Name:     "ctx-hook",
			Commands: []string{"plan"},
			Execute:  []string{"echo", "hi"},
			If:       true,
		},
	}

	require.NoError(t, run.ProcessHooks(
		t.Context(), l, v, hooks, opts, &runcfg.RunConfig{}, nil, nil,
	))

	calls := rec.snapshot()
	require.Len(t, calls, 1)

	env := envToMap(calls[0].Env)
	assert.Equal(t, "/fake/tofu", env[run.HookCtxTFPathEnvName])
	assert.Equal(t, "plan", env[run.HookCtxCommandEnvName])
	assert.Equal(t, "ctx-hook", env[run.HookCtxHookNameEnvName])

	// The opts.Env we passed in must not have been mutated.
	_, mutated := opts.Env[run.HookCtxTFPathEnvName]
	assert.False(t, mutated, "ProcessHooks must not mutate the caller's opts.Env")
}

func TestProcessHooks_PropagatesFailures(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := newHookVenv(rec.handler(vexec.Result{ExitCode: 7, Stderr: []byte("boom")}))
	l := logger.CreateLogger()

	hooks := []runcfg.Hook{
		{
			Name:     "fails",
			Commands: []string{"plan"},
			Execute:  []string{"failbin"},
			If:       true,
		},
	}

	err := run.ProcessHooks(t.Context(), l, v, hooks, newHookOpts(), &runcfg.RunConfig{}, nil, nil)
	require.Error(t, err)
	assert.NotEmpty(t, rec.snapshot())
}

func TestProcessHooks_TflintActionRoutesThroughTflint(t *testing.T) {
	t.Parallel()

	var (
		tflintInitCalls atomic.Int32
		tflintRunCalls  atomic.Int32
	)

	h := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if inv.Name != "tflint" {
			return vexec.Result{ExitCode: 127, Stderr: []byte("unexpected binary")}
		}

		if slices.Contains(inv.Args, "--init") {
			tflintInitCalls.Add(1)
		} else {
			tflintRunCalls.Add(1)
		}

		return vexec.Result{}
	}

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/work/.tflint.hcl", []byte("config {}"), 0o644))

	v := run.Venv{Exec: vexec.NewMemExec(h), FS: fs}
	l := logger.CreateLogger()

	hooks := []runcfg.Hook{
		{
			Name:     "tflint",
			Commands: []string{"plan"},
			Execute:  []string{"tflint"},
			If:       true,
		},
	}

	require.NoError(t, run.ProcessHooks(
		t.Context(), l, v, hooks, newHookOpts(), &runcfg.RunConfig{}, nil, nil,
	))

	assert.Equal(t, int32(1), tflintInitCalls.Load())
	assert.Equal(t, int32(1), tflintRunCalls.Load())
}

func TestProcessErrorHooks_NoopWhenNoPriorErrors(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	exec := vexec.NewMemExec(rec.handler(vexec.Result{}))
	l := logger.CreateLogger()

	hooks := []runcfg.ErrorHook{
		{Name: "on-anything", Commands: []string{"plan"}, OnErrors: []string{".*"}, Execute: []string{"echo", "x"}},
	}

	require.NoError(t, run.ProcessErrorHooks(t.Context(), l, exec, hooks, newHookOpts(), nil))
	assert.Empty(t, rec.snapshot())
}

func TestProcessErrorHooks_MatchesOnErrorsRegex(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	exec := vexec.NewMemExec(rec.handler(vexec.Result{}))
	l := logger.CreateLogger()

	priorErrs := new(errors.MultiError).Append(errors.New("oh no: permission denied on resource"))

	hooks := []runcfg.ErrorHook{
		{
			Name:     "match-perm",
			Commands: []string{"plan"},
			OnErrors: []string{".*permission denied.*"},
			Execute:  []string{"echo", "matched"},
		},
		{
			Name:     "match-throttling",
			Commands: []string{"plan"},
			OnErrors: []string{".*throttl.*"},
			Execute:  []string{"echo", "should-not-fire"},
		},
		{
			Name:     "command-mismatch",
			Commands: []string{"apply"},
			OnErrors: []string{".*"},
			Execute:  []string{"echo", "wrong-command"},
		},
	}

	require.NoError(t, run.ProcessErrorHooks(t.Context(), l, exec, hooks, newHookOpts(), priorErrs))

	calls := rec.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"matched"}, calls[0].Args)
}

// recorder is a thread-safe accumulator for in-memory subprocess invocations.
type recorder struct {
	calls []vexec.Invocation
	mu    sync.Mutex
}

func (r *recorder) handler(result vexec.Result) vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		r.mu.Lock()
		defer r.mu.Unlock()

		r.calls = append(r.calls, vexec.Invocation{
			Name: inv.Name,
			Dir:  inv.Dir,
			Args: slices.Clone(inv.Args),
			Env:  slices.Clone(inv.Env),
		})

		return result
	}
}

func (r *recorder) snapshot() []vexec.Invocation {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.calls)
}

func newHookOpts() *run.Options {
	return &run.Options{
		Env:               map[string]string{},
		WorkingDir:        "/work",
		RootWorkingDir:    "/work",
		TerraformCommand:  "plan",
		TFPath:            "/fake/tofu",
		MaxFoldersToCheck: 5,
		Writers:           writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}
}

func newHookVenv(h vexec.Handler) run.Venv {
	return run.Venv{Exec: vexec.NewMemExec(h), FS: vfs.NewMemMapFS()}
}

func envToMap(env []string) map[string]string {
	out := make(map[string]string, len(env))

	for _, kv := range env {
		for i := range len(kv) {
			if kv[i] == '=' {
				out[kv[:i]] = kv[i+1:]
				break
			}
		}
	}

	return out
}
