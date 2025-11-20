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

// mockUnit creates a component.Unit with the given path and dependencies.
func mockUnit(path string, deps ...*component.Unit) *component.Unit {
	unit := component.NewUnit(path)
	for _, dep := range deps {
		unit.AddDependency(dep)
	}

	return unit
}

// Add a helper to convert units to discovered components
func discoveryFromUnits(units []*component.Unit) component.Components {
	discovered := make(component.Components, 0, len(units))
	for _, u := range units {
		discovered = append(discovered, u)
	}

	return discovered
}

func TestRunnerPool_LinearDependency(t *testing.T) {
	t.Parallel()

	// A -> B -> C
	// Build Component objects directly
	compA := component.NewUnit("A")
	compB := component.NewUnit("B")
	compB.AddDependency(compA)

	compC := component.NewUnit("C")
	compC.AddDependency(compB)
	components := component.Components{compA, compB, compC}

	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	unitC := mockUnit("C", unitB)
	units := []*component.Unit{unitA, unitB, unitC}

	runner := func(ctx context.Context, u *component.Unit) error {
		return nil
	}

	q, err := queue.NewQueue(components)
	require.NoError(t, err)

	dagRunner := runnerpool.NewController(
		q,
		discoveryFromUnits(units),
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
	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	unitC := mockUnit("C", unitA)
	units := []*component.Unit{unitA, unitB, unitC}

	runner := func(ctx context.Context, u *component.Unit) error {
		return nil
	}

	q, err := queue.NewQueue(discoveryFromUnits(units))
	require.NoError(t, err)

	dagRunner := runnerpool.NewController(
		q,
		discoveryFromUnits(units),
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(2),
	)
	err = dagRunner.Run(t.Context(), logger.CreateLogger())
	require.NoError(t, err)
}

func TestRunnerPool_FailFast(t *testing.T) {
	t.Parallel()
	// A -> B -> C
	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	unitC := mockUnit("C", unitB)
	units := []*component.Unit{unitA, unitB, unitC}

	runner := func(ctx context.Context, u *component.Unit) error {
		if u.Path() == "A" {
			return errors.New("unit A failed")
		}

		return nil
	}

	q, err := queue.NewQueue(discoveryFromUnits(units))
	require.NoError(t, err)

	q.FailFast = true
	dagRunner := runnerpool.NewController(
		q,
		discoveryFromUnits(units),
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
	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	unitC := mockUnit("C", unitA)
	unitD := mockUnit("D", unitB)
	unitE := mockUnit("E", unitB)

	return []*component.Unit{unitA, unitB, unitC, unitD, unitE}
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

	q, err := queue.NewQueue(discoveryFromUnits(units))
	require.NoError(t, err)

	dagRunner := runnerpool.NewController(
		q,
		discoveryFromUnits(units),
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

	q, err := queue.NewQueue(discoveryFromUnits(units))
	require.NoError(t, err)

	q.FailFast = true
	dagRunner := runnerpool.NewController(
		q,
		discoveryFromUnits(units),
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

	q, err := queue.NewQueue(discoveryFromUnits(units))
	require.NoError(t, err)

	q.FailFast = true
	dagRunner := runnerpool.NewController(
		q,
		discoveryFromUnits(units),
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(8),
	)
	err = dagRunner.Run(t.Context(), logger.CreateLogger())
	require.Error(t, err)

	for _, want := range []string{"unit B failed", "unit D did not run due to early exit", "unit E did not run due to early exit"} {
		assert.Contains(t, err.Error(), want, "Expected error message '%s' in errors", want)
	}
}
