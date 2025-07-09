package runnerpool_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/errors"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"

	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
)

// mockUnit creates a common.Unit with the given path and dependencies.
func mockUnit(path string, deps ...*common.Unit) *common.Unit {
	return &common.Unit{
		Path:         path,
		Dependencies: deps,
	}
}

// Add a helper to convert units to discovered configs
func discoveryFromUnits(units []*common.Unit) []*discovery.DiscoveredConfig {
	discovered := make([]*discovery.DiscoveredConfig, 0, len(units))
	unitMap := make(map[*common.Unit]*discovery.DiscoveredConfig)
	// First pass: create configs
	for _, u := range units {
		cfg := &discovery.DiscoveredConfig{Path: u.Path}
		unitMap[u] = cfg
		discovered = append(discovered, cfg)
	}
	// Second pass: wire dependencies
	for i, u := range units {
		var deps []*discovery.DiscoveredConfig
		for _, dep := range u.Dependencies {
			if depCfg, ok := unitMap[dep]; ok {
				deps = append(deps, depCfg)
			}
		}
		discovered[i].Dependencies = deps
	}
	return discovered
}

func TestRunnerPool_LinearDependency(t *testing.T) {
	t.Parallel()

	// A -> B -> C
	// Build DiscoveredConfig objects directly
	cfgA := &discovery.DiscoveredConfig{Path: "A"}
	cfgB := &discovery.DiscoveredConfig{Path: "B"}
	cfgC := &discovery.DiscoveredConfig{Path: "C"}
	cfgB.Dependencies = []*discovery.DiscoveredConfig{cfgA}
	cfgC.Dependencies = []*discovery.DiscoveredConfig{cfgB}
	configs := []*discovery.DiscoveredConfig{cfgA, cfgB, cfgC}

	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	unitC := mockUnit("C", unitB)
	units := []*common.Unit{unitA, unitB, unitC}

	runner := func(ctx context.Context, u *common.Unit) error {
		return nil
	}

	q, err := queue.NewQueue(configs)
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
	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	unitC := mockUnit("C", unitA)
	units := []*common.Unit{unitA, unitB, unitC}

	runner := func(ctx context.Context, u *common.Unit) error {
		return nil
	}

	q, err := queue.NewQueue(discoveryFromUnits(units))
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
	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	unitC := mockUnit("C", unitB)
	units := []*common.Unit{unitA, unitB, unitC}

	runner := func(ctx context.Context, u *common.Unit) error {
		if u.Path == "A" {
			return errors.New("unit A failed")
		}
		return nil
	}

	q, err := queue.NewQueue(discoveryFromUnits(units))
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
func buildComplexUnits() []*common.Unit {
	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	unitC := mockUnit("C", unitA)
	unitD := mockUnit("D", unitB)
	unitE := mockUnit("E", unitB)
	return []*common.Unit{unitA, unitB, unitC, unitD, unitE}
}

func TestRunnerPool_ComplexDependency_BFails(t *testing.T) {
	t.Parallel()
	units := buildComplexUnits()

	runner := func(ctx context.Context, u *common.Unit) error {
		if u.Path == "B" {
			return errors.New("unit B failed")
		}
		return nil
	}

	q, err := queue.NewQueue(discoveryFromUnits(units))
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

	runner := func(ctx context.Context, u *common.Unit) error {
		if u.Path == "A" {
			return errors.New("unit A failed")
		}
		return nil
	}

	q, err := queue.NewQueue(discoveryFromUnits(units))
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

	runner := func(ctx context.Context, u *common.Unit) error {
		if u.Path == "B" {
			return errors.New("unit B failed")
		}
		return nil
	}

	q, err := queue.NewQueue(discoveryFromUnits(units))
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
