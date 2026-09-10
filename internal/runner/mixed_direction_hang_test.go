package runner_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestControllerFinishesAfterDownEntryFailsWithPendingUpChain drives the real Controller through
// the `run --all --filter-allow-destroy` hang, with the dispatch order the Controller actually uses
// (it claims every currently-ready entry as Running in one pass before any of them run, so the
// affected unit has to be one that was still gated at the moment of failure):
//
//	base (up) <- shared (up) <- consumer (up)
//	                shared (up) <- gone (down: plan -destroy of a deleted unit)
//
// Pass 1 claims `base` and `gone`. `gone` fails while `base` is still running, so `shared` is still
// Ready. Before the fix, FailEntry -> earlyExitDependencies marked `shared` EarlyExit across the
// direction boundary, `consumer` was gated forever on it, and Controller.Run never returned: the
// process sat silent after the last unit's output, with no child process alive, until killed from
// outside. With the fix, the failure stays on `gone`, the surviving chain completes, and Run returns
// the failure so it is actually reported.
func TestControllerFinishesAfterDownEntryFailsWithPendingUpChain(t *testing.T) {
	t.Parallel()

	base := component.NewUnit("/repo/base")
	shared := component.NewUnit("/repo/shared")
	consumer := component.NewUnit("/repo/consumer")
	shared.AddDependency(base)
	consumer.AddDependency(shared)

	gone := component.NewUnit("/wt/from/gone").WithDiscoveryContext(&component.DiscoveryContext{
		Cmd:  "plan",
		Args: []string{"-destroy"},
	})
	gone.AddDependency(shared)

	units := []*component.Unit{base, shared, consumer, gone}

	q, err := queue.NewQueue(component.Components{base, shared, consumer, gone})
	require.NoError(t, err)

	errGoneFailed := errors.New("plan -destroy failed")
	goneReturned := make(chan struct{})

	runUnit := func(ctx context.Context, u *component.Unit) error {
		switch u.Path() {
		case gone.Path():
			close(goneReturned)

			return errGoneFailed
		case base.Path():
			// Hold `base` open until `gone` has returned its error, so `shared` is still Ready
			// (gated on base, not Running) when the controller's FailEntry walks gone's
			// dependencies. FailEntry runs on the controller goroutine right after the runner
			// returns; the short pause lets it land. Entry statuses are only read after Run
			// returns, so this test never touches them concurrently with the controller.
			select {
			case <-goneReturned:
			case <-time.After(5 * time.Second):
				return errors.New("test setup: gone never ran")
			}

			time.Sleep(100 * time.Millisecond)

			return nil
		default:
			return nil
		}
	}

	ctrl := runner.NewController(q, units, runner.WithRunner(runUnit), runner.WithMaxConcurrency(4))

	done := make(chan error, 1)

	go func() { done <- ctrl.Run(t.Context(), logger.CreateLogger()) }()

	select {
	case err := <-done:
		require.ErrorIs(t, err, errGoneFailed, "the destroy failure must be reported, not lost")
	case <-time.After(10 * time.Second):
		t.Fatal("Controller.Run did not return: the queue is wedged (the pre-fix hang)")
	}

	assert.True(t, q.Finished())
	assert.Equal(t, queue.StatusFailed, q.EntryByPath(gone.Path()).Status)

	for _, u := range []*component.Unit{base, shared, consumer} {
		assert.Equal(t, queue.StatusSucceeded, q.EntryByPath(u.Path()).Status,
			"%s is a surviving unit and must run to completion regardless of the destroy failure", u.Path())
	}
}
