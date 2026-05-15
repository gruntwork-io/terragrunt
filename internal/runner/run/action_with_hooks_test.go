package run_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunActionWithHooks_FiresBeforeActionAfterInOrder pins the
// lifecycle contract: before_hooks → action → after_hooks for the
// happy path, with all three observable through their subprocess
// dispatches.
func TestRunActionWithHooks_FiresBeforeActionAfterInOrder(t *testing.T) {
	t.Parallel()

	var order []string

	v := newHookVenv(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		order = append(order, inv.Name)
		return vexec.Result{}
	})
	l := logger.CreateLogger()

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			BeforeHooks: []runcfg.Hook{
				{Name: "before-1", Commands: []string{"plan"}, Execute: []string{"step-before-1"}, If: true},
				{Name: "before-2", Commands: []string{"plan"}, Execute: []string{"step-before-2"}, If: true},
			},
			AfterHooks: []runcfg.Hook{
				{Name: "after-1", Commands: []string{"plan"}, Execute: []string{"step-after-1"}, If: true},
			},
		},
	}

	actionFired := false
	action := func(_ context.Context) error {
		order = append(order, "ACTION")
		actionFired = true

		return nil
	}

	require.NoError(t, run.RunActionWithHooks(
		t.Context(), l, v, "plan", newHookOpts(), cfg, report.NewReport(), action,
	))

	assert.True(t, actionFired)
	assert.Equal(t, []string{"step-before-1", "step-before-2", "ACTION", "step-after-1"}, order,
		"hooks must fire before action, after hooks must fire after action, all in declaration order")
}

// TestRunActionWithHooks_BeforeHookFailureSkipsAction pins the
// contract that a failing before_hook prevents the wrapped action
// from running entirely; after_hooks still fire because they don't
// have RunOnError set the same way, and error_hooks see the failure.
func TestRunActionWithHooks_BeforeHookFailureSkipsAction(t *testing.T) {
	t.Parallel()

	var dispatched []string

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		dispatched = append(dispatched, inv.Name)

		if inv.Name == "bad-before" {
			return vexec.Result{ExitCode: 2, Stderr: []byte("hook failed")}
		}

		return vexec.Result{}
	})

	v := &run.Venv{Exec: exec, FS: vfs.NewMemMapFS(), Writers: writer.Writers{}}
	l := logger.CreateLogger()

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			BeforeHooks: []runcfg.Hook{
				{Name: "bad", Commands: []string{"plan"}, Execute: []string{"bad-before"}, If: true},
			},
			ErrorHooks: []runcfg.ErrorHook{
				{
					Name:     "on-bad",
					Commands: []string{"plan"},
					OnErrors: []string{".*hook failed.*"},
					Execute:  []string{"error-handler"},
				},
			},
		},
	}

	actionFired := false
	action := func(_ context.Context) error {
		actionFired = true
		return nil
	}

	err := run.RunActionWithHooks(t.Context(), l, v, "plan", newHookOpts(), cfg, report.NewReport(), action)
	require.Error(t, err, "before-hook failure must propagate")
	assert.False(t, actionFired, "action must be skipped when before_hooks fail")

	// before-hook fires, error-handler fires; action never does.
	assert.Equal(t, []string{"bad-before", "error-handler"}, dispatched)
}

// TestRunActionWithHooks_ActionFailureTriggersErrorHook pins the
// contract that an action error surfaces to error_hooks whose
// OnErrors regex matches the error message.
func TestRunActionWithHooks_ActionFailureTriggersErrorHook(t *testing.T) {
	t.Parallel()

	var errorHookCalls []vexec.Invocation

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if inv.Name == "panic-cleanup" {
			errorHookCalls = append(errorHookCalls, inv)
		}

		return vexec.Result{}
	})

	v := &run.Venv{Exec: exec, FS: vfs.NewMemMapFS(), Writers: writer.Writers{}}
	l := logger.CreateLogger()

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ErrorHooks: []runcfg.ErrorHook{
				{
					Name:     "cleanup-on-lock-error",
					Commands: []string{"plan"},
					OnErrors: []string{".*state lock.*"},
					Execute:  []string{"panic-cleanup"},
				},
				{
					Name:     "cleanup-on-network",
					Commands: []string{"plan"},
					OnErrors: []string{".*timeout.*"},
					Execute:  []string{"network-handler"},
				},
			},
		},
	}

	action := func(_ context.Context) error {
		return errors.New("Failed to acquire state lock on bucket")
	}

	err := run.RunActionWithHooks(t.Context(), l, v, "plan", newHookOpts(), cfg, report.NewReport(), action)
	require.Error(t, err, "action failure must propagate")

	require.Len(t, errorHookCalls, 1, "only the matching error_hook must fire")
	assert.Equal(t, "panic-cleanup", errorHookCalls[0].Name)
}

// TestRunActionWithHooks_AfterHooksSkipOnActionFailure pins the
// contract that after_hooks default to RunOnError=false, so an action
// failure suppresses them. error_hooks are the correct path for
// after-failure cleanup.
func TestRunActionWithHooks_AfterHooksSkipOnActionFailure(t *testing.T) {
	t.Parallel()

	var dispatched []string

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		dispatched = append(dispatched, inv.Name)
		return vexec.Result{}
	})

	v := &run.Venv{Exec: exec, FS: vfs.NewMemMapFS(), Writers: writer.Writers{}}
	l := logger.CreateLogger()

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			AfterHooks: []runcfg.Hook{
				{Name: "after-default", Commands: []string{"plan"}, Execute: []string{"normal-after"}, If: true},
				{Name: "after-roe", Commands: []string{"plan"}, Execute: []string{"roe-after"}, If: true, RunOnError: true},
			},
		},
	}

	action := func(_ context.Context) error {
		return errors.New("action exploded")
	}

	err := run.RunActionWithHooks(t.Context(), l, v, "plan", newHookOpts(), cfg, report.NewReport(), action)
	require.Error(t, err)

	// normal-after is suppressed by the action failure; roe-after fires because RunOnError=true.
	assert.Equal(t, []string{"roe-after"}, dispatched)
}

// TestRunActionWithHooks_NoHooksRunsActionDirectly pins the trivial
// path: an empty hook config invokes the action exactly once.
func TestRunActionWithHooks_NoHooksRunsActionDirectly(t *testing.T) {
	t.Parallel()

	var subprocessCalls int

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		subprocessCalls++
		return vexec.Result{}
	})

	v := &run.Venv{Exec: exec, FS: vfs.NewMemMapFS(), Writers: writer.Writers{}}
	l := logger.CreateLogger()

	actionFired := 0
	action := func(_ context.Context) error {
		actionFired++
		return nil
	}

	require.NoError(t, run.RunActionWithHooks(
		t.Context(), l, v, "plan", newHookOpts(), &runcfg.RunConfig{}, report.NewReport(), action,
	))

	assert.Equal(t, 1, actionFired, "action must fire exactly once")
	assert.Zero(t, subprocessCalls, "no hooks configured: subprocess must not fire")
}
