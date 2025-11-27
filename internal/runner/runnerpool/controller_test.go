package runnerpool_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"

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

	for _, want := range []string{"unit A failed", "unit B did not run due to early exit", "unit C did not run due to early exit"} {
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

	for _, want := range []string{"unit B failed", "unit D did not run due to early exit", "unit E did not run due to early exit"} {
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
		"unit B did not run due to early exit",
		"unit C did not run due to early exit",
		"unit D did not run due to early exit",
		"unit E did not run due to early exit",
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

	for _, want := range []string{"unit B failed", "unit D did not run due to early exit", "unit E did not run due to early exit"} {
		assert.Contains(t, err.Error(), want, "Expected error message '%s' in errors", want)
	}
}
