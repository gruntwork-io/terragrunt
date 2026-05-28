package runnerpool_test

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/gruntwork-io/terragrunt/pkg/config"

	"github.com/gruntwork-io/terragrunt/internal/queue"
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

	runner := func(ctx context.Context, u *component.Unit) error {
		return nil
	}

	q, err := queue.NewQueue(components)
	require.NoError(t, err)

	dagRunner := runnerpool.NewController(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(2),
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

	runner := func(ctx context.Context, u *component.Unit) error {
		return nil
	}

	components := make(component.Components, len(units))
	for i, u := range units {
		components[i] = u
	}

	q, err := queue.NewQueue(components)
	require.NoError(t, err)

	dagRunner := runnerpool.NewController(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(2),
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

	runner := func(ctx context.Context, u *component.Unit) error {
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
	dagRunner := runnerpool.NewController(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(2),
	)
	err = dagRunner.Run(t.Context(), logger.CreateLogger())
	require.Error(t, err)

	for _, want := range []string{"unit A failed", "Unit 'B' did not run", "Unit 'C' did not run"} {
		assert.Contains(t, err.Error(), want, "Expected error message '%s' in errors", want)
	}
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

	runner := func(ctx context.Context, u *component.Unit) error {
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

	dagRunner := runnerpool.NewController(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(8),
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

	runner := func(ctx context.Context, u *component.Unit) error {
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
	dagRunner := runnerpool.NewController(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(8),
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

	runner := func(ctx context.Context, u *component.Unit) error {
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
	dagRunner := runnerpool.NewController(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(8),
	)
	err = dagRunner.Run(t.Context(), logger.CreateLogger())
	require.Error(t, err)

	for _, want := range []string{"unit B failed", "Unit 'D' did not run", "Unit 'E' did not run"} {
		assert.Contains(t, err.Error(), want, "Expected error message '%s' in errors", want)
	}
}

// buildWeightedUnits creates units with run_weight set via config.
func buildWeightedUnits(paths []string, weights map[string]int, depMap map[string][]string) []*component.Unit {
	unitMap := make(map[string]*component.Unit)

	for _, path := range paths {
		u := component.NewUnit(path)
		if w, ok := weights[path]; ok {
			cfg := &config.TerragruntConfig{RunWeight: &w}
			u.WithConfig(cfg)
		}

		unitMap[path] = u
	}

	for path, deps := range depMap {
		unit := unitMap[path]
		for _, depPath := range deps {
			if depUnit, ok := unitMap[depPath]; ok {
				unit.AddDependency(depUnit)
			}
		}
	}

	units := make([]*component.Unit, 0, len(paths))
	for _, path := range paths {
		units = append(units, unitMap[path])
	}

	return units
}

// runWeightedController is a helper that wires up units into a queue and controller, then runs it.
func runWeightedController(t *testing.T, units []*component.Unit, concurrency int, runner runnerpool.UnitRunner) error {
	t.Helper()

	components := make(component.Components, len(units))
	for i, u := range units {
		components[i] = u
	}

	q, err := queue.NewQueue(components)
	require.NoError(t, err)

	ctrl := runnerpool.NewController(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(concurrency),
	)

	return ctrl.Run(t.Context(), logger.CreateLogger())
}

func TestRunnerPool_DefaultWeightBackwardsCompatWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// Units without run_weight should default to 1, preserving existing behavior.
		units := buildComponentUnits([]string{"A", "B", "C"}, map[string][]string{})

		var mu sync.Mutex

		var current, maxConcurrent int

		err := runWeightedController(t, units, 3, func(ctx context.Context, u *component.Unit) error {
			mu.Lock()

			current++
			if current > maxConcurrent {
				maxConcurrent = current
			}

			mu.Unlock()

			time.Sleep(50 * time.Millisecond)

			mu.Lock()
			current--
			mu.Unlock()

			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 3, maxConcurrent, "All 3 default-weight units should run concurrently with budget 3")
	})
}

func TestRunnerPool_WeightedBudgetAdmissionWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// Budget=10, heavy units weight=5, light units weight=1.
		units := buildWeightedUnits(
			[]string{"heavy1", "heavy2", "heavy3", "light1", "light2", "light3"},
			map[string]int{"heavy1": 5, "heavy2": 5, "heavy3": 5, "light1": 1, "light2": 1, "light3": 1},
			map[string][]string{},
		)

		var mu sync.Mutex

		var maxWeight, currentWeight int

		err := runWeightedController(t, units, 10, func(ctx context.Context, u *component.Unit) error {
			w := u.RunWeight()

			mu.Lock()

			currentWeight += w
			if currentWeight > maxWeight {
				maxWeight = currentWeight
			}

			cw := currentWeight
			mu.Unlock()

			assert.LessOrEqual(t, cw, 10, "In-flight weight should never exceed budget of 10")

			time.Sleep(50 * time.Millisecond)

			mu.Lock()
			currentWeight -= w
			mu.Unlock()

			return nil
		})
		require.NoError(t, err)
		assert.LessOrEqual(t, maxWeight, 10, "Peak in-flight weight must not exceed budget")
		assert.Greater(t, maxWeight, 1, "Should have achieved some concurrency")
	})
}

func TestRunnerPool_OversizedWeightRunsSoloWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// A unit with weight > budget should still run (solo, when pool is empty).
		units := buildWeightedUnits(
			[]string{"huge", "small"},
			map[string]int{"huge": 20, "small": 1},
			map[string][]string{},
		)

		var executedPaths sync.Map

		err := runWeightedController(t, units, 5, func(ctx context.Context, u *component.Unit) error {
			executedPaths.Store(u.Path(), true)

			return nil
		})
		require.NoError(t, err)

		_, hugeRan := executedPaths.Load("huge")
		_, smallRan := executedPaths.Load("small")

		assert.True(t, hugeRan, "Oversized unit should still execute")
		assert.True(t, smallRan, "Small unit should also execute")
	})
}

func TestRunnerPool_HeavyUnitDoesNotBlockLightUnitsWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// Budget=10. All units independent (no deps).
		// first(3) runs and holds 3 of 10. heavy(9) can't fit (3+9=12 > 10) but
		// light units (1 each) should skip ahead since they do fit (3+1=4 <= 10).
		// Without skip-ahead logic, heavy would block the loop and light units would starve.
		units := buildWeightedUnits(
			[]string{"first", "heavy", "light1", "light2"},
			map[string]int{"first": 3, "heavy": 9, "light1": 1, "light2": 1},
			map[string][]string{},
		)

		var mu sync.Mutex

		var maxWeight, currentWeight int

		lightRanWhileFirstRunning := false

		err := runWeightedController(t, units, 10, func(ctx context.Context, u *component.Unit) error {
			w := u.RunWeight()

			mu.Lock()

			currentWeight += w
			if currentWeight > maxWeight {
				maxWeight = currentWeight
			}

			// If a light unit is running while first is still in-flight, skip-ahead worked
			if w == 1 && currentWeight > 1 {
				lightRanWhileFirstRunning = true
			}

			mu.Unlock()

			time.Sleep(50 * time.Millisecond)

			mu.Lock()
			currentWeight -= w
			mu.Unlock()

			return nil
		})
		require.NoError(t, err)
		assert.True(t, lightRanWhileFirstRunning, "Light units should run concurrently with first, not blocked behind heavy")
	})
}
