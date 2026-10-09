package runner_test

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/runner"

	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
)

// buildComponentUnits creates component units and wires dependencies based on path relationships.
func buildComponentUnits(paths []string, depMap map[string][]string) []*component.Unit {
	unitMap := make(map[string]*component.Unit)

	// First pass: create units
	for _, path := range paths {
		unitMap[path] = component.NewUnit(path)
	}

	// Second pass: wire dependencies
	for path, deps := range depMap {
		unit := unitMap[path]
		for _, depPath := range deps {
			if depUnit, ok := unitMap[depPath]; ok {
				unit.AddDependency(depUnit)
			}
		}
	}

	// Collect in order
	units := make([]*component.Unit, 0, len(paths))
	for _, path := range paths {
		units = append(units, unitMap[path])
	}

	return units
}

func TestRunnerPool_LinearDependency(t *testing.T) {
	t.Parallel()

	// A -> B -> C
	units := buildComponentUnits(
		[]string{"A", "B", "C"},
		map[string][]string{
			"B": {"A"},
			"C": {"B"},
		},
	)

	components := make(component.Components, len(units))
	for i, u := range units {
		components[i] = u
	}

	rnr := func(ctx context.Context, u *component.Unit) error {
		return nil
	}

	q, err := queue.NewQueue(components)
	require.NoError(t, err)

	dagRunner := runner.NewController(
		q,
		units,
		runner.WithRunner(rnr),
		runner.WithMaxConcurrency(2),
	)
	err = dagRunner.Run(t.Context(), logger.CreateLogger())
	require.NoError(t, err)
}

func TestRunnerPool_ParallelExecution(t *testing.T) {
	t.Parallel()
	//   A
	//  / \
	// B   C
	units := buildComponentUnits(
		[]string{"A", "B", "C"},
		map[string][]string{
			"B": {"A"},
			"C": {"A"},
		},
	)

	rnr := func(ctx context.Context, u *component.Unit) error {
		return nil
	}

	components := make(component.Components, len(units))
	for i, u := range units {
		components[i] = u
	}

	q, err := queue.NewQueue(components)
	require.NoError(t, err)

	dagRunner := runner.NewController(
		q,
		units,
		runner.WithRunner(rnr),
		runner.WithMaxConcurrency(2),
	)
	err = dagRunner.Run(t.Context(), logger.CreateLogger())
	require.NoError(t, err)
}

func TestRunnerPool_FailFast(t *testing.T) {
	t.Parallel()
	// A -> B -> C
	units := buildComponentUnits(
		[]string{"A", "B", "C"},
		map[string][]string{
			"B": {"A"},
			"C": {"B"},
		},
	)

	rnr := func(ctx context.Context, u *component.Unit) error {
		if u.Path() == "A" {
			return errors.New("unit A failed")
		}

		return nil
	}

	components := make(component.Components, len(units))
	for i, u := range units {
		components[i] = u
	}

	q, err := queue.NewQueue(components)
	require.NoError(t, err)

	q.FailFast = true
	dagRunner := runner.NewController(
		q,
		units,
		runner.WithRunner(rnr),
		runner.WithMaxConcurrency(2),
	)
	err = dagRunner.Run(t.Context(), logger.CreateLogger())
	require.Error(t, err)

	for _, want := range []string{"unit A failed", "Unit 'B' did not run", "Unit 'C' did not run"} {
		assert.Contains(t, err.Error(), want, "Expected error message '%s' in errors", want)
	}
}

// TestRunnerPool_RunnerNotSet pins the typed error returned when a controller runs without a unit runner.
func TestRunnerPool_RunnerNotSet(t *testing.T) {
	t.Parallel()

	units := buildComponentUnits([]string{"A"}, nil)

	q, err := queue.NewQueue(component.Components{units[0]})
	require.NoError(t, err)

	err = runner.NewController(q, units).Run(t.Context(), logger.CreateLogger())
	require.ErrorIs(t, err, runner.ErrRunnerNotSet)
}

// TestRunnerPool_NonPositiveConcurrencyRunsSerially pins that a non-positive concurrency is clamped to one worker.
func TestRunnerPool_NonPositiveConcurrencyRunsSerially(t *testing.T) {
	t.Parallel()

	// The units are independent, so only the concurrency clamp can keep them from overlapping.
	synctest.Test(t, func(t *testing.T) {
		units := buildComponentUnits([]string{"A", "B"}, nil)

		q, err := queue.NewQueue(component.Components{units[0], units[1]})
		require.NoError(t, err)

		var (
			mu        sync.Mutex
			active    int
			maxActive int
			ran       []string
		)

		// enter records a started unit and returns the callback marking it finished.
		enter := func(path string) func() {
			mu.Lock()
			defer mu.Unlock()

			active++
			maxActive = max(maxActive, active)

			ran = append(ran, path)

			return func() {
				mu.Lock()
				defer mu.Unlock()

				active--
			}
		}

		dagRunner := runner.NewController(
			q,
			units,
			runner.WithRunner(func(_ context.Context, u *component.Unit) error {
				done := enter(u.Path())
				defer done()

				// Fake time only advances once every worker is blocked here, so a second one would overlap.
				time.Sleep(time.Second)

				return nil
			}),
			runner.WithMaxConcurrency(0),
		)
		require.NoError(t, dagRunner.Run(t.Context(), logger.CreateLogger()))

		mu.Lock()
		defer mu.Unlock()

		assert.Equal(t, 1, maxActive, "a non-positive concurrency runs units one at a time")
		assert.ElementsMatch(t, []string{"A", "B"}, ran, "every unit still runs")
	})
}

// TestRunnerPool_UnitMissingFromDiscoveredUnits pins the typed error returned when a queue entry has no unit.
func TestRunnerPool_UnitMissingFromDiscoveredUnits(t *testing.T) {
	t.Parallel()

	units := buildComponentUnits([]string{"A"}, nil)

	q, err := queue.NewQueue(component.Components{units[0]})
	require.NoError(t, err)

	// The controller is handed no units, so the queue entry has nothing to run.
	dagRunner := runner.NewController(
		q,
		nil,
		runner.WithRunner(func(context.Context, *component.Unit) error { return nil }),
	)

	err = dagRunner.Run(t.Context(), logger.CreateLogger())

	var target runner.UnitNotDiscoveredError

	require.ErrorAs(t, err, &target)
	assert.Equal(t, "A", target.UnitPath, "the error carries the path that had no discovered unit")
}

func TestRunnerPool_ContextCancelled(t *testing.T) {
	t.Parallel()

	units := buildComponentUnits([]string{"A"}, nil)

	q, err := queue.NewQueue(component.Components{units[0]})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan struct{})
	release := make(chan struct{})

	dagRunner := runner.NewController(
		q,
		units,
		runner.WithRunner(func(context.Context, *component.Unit) error {
			close(started)
			<-release

			return nil
		}),
	)

	done := make(chan error, 1)

	go func() { done <- dagRunner.Run(ctx, logger.CreateLogger()) }()

	<-started
	// Cancelling while the only task is in flight leaves readyCh empty, so the
	// controller takes its cancellation branch and waits for the task.
	cancel()
	close(release)

	require.NoError(t, <-done)
}

// Helper to build a more complex dependency graph:
//
//	   A
//	  / \
//	 B   C
//	/ \
//
// D   E
func buildComplexUnits() []*component.Unit {
	return buildComponentUnits(
		[]string{"A", "B", "C", "D", "E"},
		map[string][]string{
			"B": {"A"},
			"C": {"A"},
			"D": {"B"},
			"E": {"B"},
		},
	)
}

func TestRunnerPool_ComplexDependency_BFails(t *testing.T) {
	t.Parallel()

	units := buildComplexUnits()

	rnr := func(ctx context.Context, u *component.Unit) error {
		if u.Path() == "B" {
			return errors.New("unit B failed")
		}

		return nil
	}

	components := make(component.Components, len(units))
	for i, u := range units {
		components[i] = u
	}

	q, err := queue.NewQueue(components)
	require.NoError(t, err)

	dagRunner := runner.NewController(
		q,
		units,
		runner.WithRunner(rnr),
		runner.WithMaxConcurrency(8),
	)
	err = dagRunner.Run(t.Context(), logger.CreateLogger())
	require.Error(t, err)

	for _, want := range []string{"unit B failed", "Unit 'D' did not run", "Unit 'E' did not run"} {
		assert.Contains(t, err.Error(), want, "Expected error message '%s' in errors", want)
	}
}

func TestRunnerPool_ComplexDependency_AFails_FailFast(t *testing.T) {
	t.Parallel()

	units := buildComplexUnits()

	rnr := func(ctx context.Context, u *component.Unit) error {
		if u.Path() == "A" {
			return errors.New("unit A failed")
		}

		return nil
	}

	components := make(component.Components, len(units))
	for i, u := range units {
		components[i] = u
	}

	q, err := queue.NewQueue(components)
	require.NoError(t, err)

	q.FailFast = true
	dagRunner := runner.NewController(
		q,
		units,
		runner.WithRunner(rnr),
		runner.WithMaxConcurrency(8),
	)
	err = dagRunner.Run(t.Context(), logger.CreateLogger())
	require.Error(t, err)

	for _, want := range []string{
		"unit A failed",
		"Unit 'B' did not run",
		"Unit 'C' did not run",
		"Unit 'D' did not run",
		"Unit 'E' did not run",
	} {
		assert.Contains(t, err.Error(), want, "Expected error message '%s' in errors", want)
	}
}

func TestRunnerPool_ComplexDependency_BFails_FailFast(t *testing.T) {
	t.Parallel()

	units := buildComplexUnits()

	rnr := func(ctx context.Context, u *component.Unit) error {
		if u.Path() == "B" {
			return errors.New("unit B failed")
		}

		return nil
	}

	components := make(component.Components, len(units))
	for i, u := range units {
		components[i] = u
	}

	q, err := queue.NewQueue(components)
	require.NoError(t, err)

	q.FailFast = true
	dagRunner := runner.NewController(
		q,
		units,
		runner.WithRunner(rnr),
		runner.WithMaxConcurrency(8),
	)
	err = dagRunner.Run(t.Context(), logger.CreateLogger())
	require.Error(t, err)

	for _, want := range []string{"unit B failed", "Unit 'D' did not run", "Unit 'E' did not run"} {
		assert.Contains(t, err.Error(), want, "Expected error message '%s' in errors", want)
	}
}

// A fails only once fake time advances, after the loop has claimed B and blocked on the one worker.
func TestRunnerPool_FailFastSkipsEntriesCancelledBeforeDispatch(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		units := buildComponentUnits([]string{"A", "B", "C"}, nil)

		q, err := queue.NewQueue(component.Components{units[0], units[1], units[2]})
		require.NoError(t, err)

		q.FailFast = true

		errA := errors.New("unit A failed")

		var (
			mu  sync.Mutex
			ran []string
		)

		record := func(path string) {
			mu.Lock()
			defer mu.Unlock()

			ran = append(ran, path)
		}

		dagRunner := runner.NewController(
			q,
			units,
			runner.WithRunner(func(_ context.Context, u *component.Unit) error {
				record(u.Path())

				if u.Path() != "A" {
					return nil
				}

				time.Sleep(time.Second)

				return errA
			}),
			runner.WithMaxConcurrency(1),
		)

		err = dagRunner.Run(t.Context(), logger.CreateLogger())
		require.ErrorIs(t, err, errA)

		var earlyExit runner.UnitEarlyExitError

		require.ErrorAs(t, err, &earlyExit)
		assert.Equal(t, "C", earlyExit.UnitPath)

		mu.Lock()
		defer mu.Unlock()

		assert.Equal(t, []string{"A", "B"}, ran, "B was claimed before A failed, so only C is skipped")
		assert.Equal(t, queue.StatusSucceeded, q.EntryByPath("B").Status)
	})
}

func TestRunnerPool_ReportsEntryFailedBeforeRun(t *testing.T) {
	t.Parallel()

	units := buildComponentUnits([]string{"A", "B"}, nil)

	q, err := queue.NewQueue(component.Components{units[0], units[1]})
	require.NoError(t, err)

	q.EntryByPath("A").Status = queue.StatusFailed

	var ran []string

	dagRunner := runner.NewController(
		q,
		units,
		runner.WithRunner(func(_ context.Context, u *component.Unit) error {
			ran = append(ran, u.Path())

			return nil
		}),
		runner.WithMaxConcurrency(1),
	)

	err = dagRunner.Run(t.Context(), logger.CreateLogger())

	var failed runner.UnitFailedError

	require.ErrorAs(t, err, &failed)
	assert.Equal(t, "A", failed.UnitPath)
	assert.Equal(t, []string{"B"}, ran, "an entry that already failed is not run again")
}

func TestRunnerPool_RecordsOutcomeCountsOnSpan(t *testing.T) {
	t.Parallel()

	units := buildComponentUnits([]string{"A", "B", "C"}, map[string][]string{"C": {"B"}})

	q, err := queue.NewQueue(component.Components{units[0], units[1], units[2]})
	require.NoError(t, err)

	errB := errors.New("unit B failed")

	buf, tlm := newConsoleTelemeter(t)
	ctx := telemetry.ContextWithTelemeter(t.Context(), tlm)

	dagRunner := runner.NewController(
		q,
		units,
		runner.WithRunner(func(_ context.Context, u *component.Unit) error {
			if u.Path() == "B" {
				return errB
			}

			return nil
		}),
	)

	require.ErrorIs(t, dagRunner.Run(ctx, logger.CreateLogger()), errB)
	require.NoError(t, tlm.Shutdown(ctx))

	spans := spansNamed(decodeSpans(t, buf), "runner_pool_controller")
	require.Len(t, spans, 1)

	counts := map[string]any{}
	for _, key := range []string{"tasks_succeeded", "tasks_failed", "tasks_early_exit"} {
		counts[key] = spans[0].Attrs[key]
	}

	assert.Equal(t, map[string]any{
		"tasks_succeeded":  float64(1),
		"tasks_failed":     float64(1),
		"tasks_early_exit": float64(1),
	}, counts)
}
