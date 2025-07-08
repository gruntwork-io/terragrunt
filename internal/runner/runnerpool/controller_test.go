package runnerpool_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

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

	q, _ := queue.NewQueue(configs)
	dagRunner := runnerpool.NewController(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(2),
	)
	results := dagRunner.Run(t.Context(), logger.CreateLogger())

	// Check that results are in the same order as queue entries
	for i, entry := range q.Entries {
		res := results[i]
		// Find the unit with this path
		var unit *common.Unit
		for _, u := range units {
			if u.Path == entry.Config.Path {
				unit = u
				break
			}
		}
		require.NotNil(t, unit, "Unit for path %s not found", entry.Config.Path)
		assert.Equal(t, entry.Config.Path, unit.Path, "Result order mismatch at index %d: expected %s, got %s", i, entry.Config.Path, unit.Path)
		assert.NoError(t, res.Err)
	}
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

	q, _ := queue.NewQueue(discoveryFromUnits(units))
	dagRunner := runnerpool.NewController(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(2),
	)
	results := dagRunner.Run(t.Context(), logger.CreateLogger())

	// Check that results are in the same order as queue entries
	for i, entry := range q.Entries {
		res := results[i]
		assert.Equal(t, entry.Config.Path, units[i].Path, "Result order mismatch at index %d: expected %s, got %s", i, entry.Config.Path, units[i].Path)
		// Find the unit with this path
		var unit *common.Unit
		for _, u := range units {
			if u.Path == entry.Config.Path {
				unit = u
				break
			}
		}
		require.NotNil(t, unit, "Unit for path %s not found", entry.Config.Path)
		assert.NoError(t, res.Err)
	}
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
			return assert.AnError
		}
		return nil
	}

	q, _ := queue.NewQueue(discoveryFromUnits(units))
	dagRunner := runnerpool.NewController(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(2),
	)
	results := dagRunner.Run(t.Context(), logger.CreateLogger())

	// Check that if C fails, all others fail too
	for i := range results {
		res := results[i]
		assert.Error(t, res.Err, "Expected error for unit %d", i)
	}
}
