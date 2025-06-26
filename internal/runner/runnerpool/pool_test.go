package runnerpool_test

import (
	"context"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
)

// mockUnit creates a runbase.Unit with the given path and dependencies.
func mockUnit(path string, deps ...*runbase.Unit) *runbase.Unit {
	return &runbase.Unit{
		Path:         path,
		Dependencies: deps,
	}
}

func TestRunnerPool_LinearDependency(t *testing.T) {
	t.Parallel()

	// A -> B -> C
	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	unitC := mockUnit("C", unitB)
	units := []*runbase.Unit{unitA, unitB, unitC}

	var mu sync.Mutex
	executionOrder := []string{}
	runner := func(ctx context.Context, t *runnerpool.Task) runnerpool.Result {
		mu.Lock()
		executionOrder = append(executionOrder, t.ID())
		mu.Unlock()
		return runnerpool.Result{TaskID: t.ID(), ExitCode: 0}
	}

	pool := runnerpool.New(units, runner, 2, false)
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
	units := []*runbase.Unit{unitA, unitB, unitC}

	var mu sync.Mutex
	executionOrder := []string{}
	runner := func(ctx context.Context, t *runnerpool.Task) runnerpool.Result {
		mu.Lock()
		executionOrder = append(executionOrder, t.ID())
		mu.Unlock()
		return runnerpool.Result{TaskID: t.ID(), ExitCode: 0}
	}

	pool := runnerpool.New(units, runner, 2, false)
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
	units := []*runbase.Unit{unitA, unitB, unitC}

	runner := func(ctx context.Context, t *runnerpool.Task) runnerpool.Result {
		if t.ID() == "A" {
			return runnerpool.Result{TaskID: t.ID(), ExitCode: 1, Err: assert.AnError}
		}
		return runnerpool.Result{TaskID: t.ID(), ExitCode: 0}
	}

	pool := runnerpool.New(units, runner, 2, true)
	results := pool.Run(t.Context(), logger.CreateLogger())

	// A should fail, B and C should be skipped (fail-fast)
	for _, res := range results {
		if res.TaskID == "A" {
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
