package runnerpool_test

import (
	"context"
	"sync"
	"testing"

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
	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	unitC := mockUnit("C", unitB)
	units := []*common.Unit{unitA, unitB, unitC}

	var mu sync.Mutex
	executionOrder := []string{}
	runner := func(ctx context.Context, u *common.Unit) (int, error) {
		mu.Lock()
		executionOrder = append(executionOrder, u.Path)
		mu.Unlock()
		return 0, nil
	}

	q, _ := queue.NewQueue(discoveryFromUnits(units))
	pool := runnerpool.NewRunnerPool(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(2),
		runnerpool.WithFailFast(false),
	)
	results := pool.Run(t.Context(), logger.CreateLogger())

	// All should succeed
	for _, res := range results {
		assert.Equal(t, 0, res.ExitCode)
	}
	// A must run before B, B before C
	assert.Less(t, indexOf(executionOrder, "A"), indexOf(executionOrder, "B"))
	assert.Less(t, indexOf(executionOrder, "B"), indexOf(executionOrder, "C"))
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

	var mu sync.Mutex
	executionOrder := []string{}
	runner := func(ctx context.Context, u *common.Unit) (int, error) {
		mu.Lock()
		executionOrder = append(executionOrder, u.Path)
		mu.Unlock()
		return 0, nil
	}

	q, _ := queue.NewQueue(discoveryFromUnits(units))
	pool := runnerpool.NewRunnerPool(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(2),
		runnerpool.WithFailFast(false),
	)
	results := pool.Run(t.Context(), logger.CreateLogger())

	for _, res := range results {
		assert.Equal(t, 0, res.ExitCode)
	}
	// A must run before B and C
	assert.Less(t, indexOf(executionOrder, "A"), indexOf(executionOrder, "B"))
	assert.Less(t, indexOf(executionOrder, "A"), indexOf(executionOrder, "C"))
}

func TestRunnerPool_FailFast(t *testing.T) {
	t.Parallel()
	//   A
	//  / \
	// B   C
	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	unitC := mockUnit("C", unitA)
	units := []*common.Unit{unitA, unitB, unitC}

	runner := func(ctx context.Context, u *common.Unit) (int, error) {
		if u.Path == "A" {
			return 1, assert.AnError
		}
		return 0, nil
	}

	q, _ := queue.NewQueue(discoveryFromUnits(units))
	pool := runnerpool.NewRunnerPool(
		q,
		units,
		runnerpool.WithRunner(runner),
		runnerpool.WithMaxConcurrency(2),
		runnerpool.WithFailFast(true),
	)
	results := pool.Run(t.Context(), logger.CreateLogger())

	// A should fail, B and C should be skipped (fail-fast)
	for i, res := range results {
		if units[i].Path == "A" {
			assert.Equal(t, 1, res.ExitCode)
			assert.Error(t, res.Err)
		} else {
			assert.NotEqual(t, 0, res.ExitCode)
		}
	}
}

// indexOf returns the index of s in arr, or -1 if not found.
func indexOf(arr []string, s string) int {
	for i, v := range arr {
		if v == s {
			return i
		}
	}
	return -1
}
