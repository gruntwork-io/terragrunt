package runnerpool_test

import (
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
	rp "github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/stretchr/testify/assert"
)

func TestBuildQueue_SimpleDAG(t *testing.T) {
	t.Parallel()

	// A -> B -> C
	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	unitC := mockUnit("C", unitB)
	units := []*runbase.Unit{unitA, unitB, unitC}

	q := rp.BuildQueue(units, false)
	assert.NotNil(t, q)
	assert.Equal(t, 3, len(q.Index))
	assert.Equal(t, 3, len(q.Ordered))

	// Check initial states
	assert.Equal(t, rp.StatusReady, q.Index["A"].State)
	assert.Equal(t, rp.StatusBlocked, q.Index["B"].State)
	assert.Equal(t, rp.StatusBlocked, q.Index["C"].State)
}

func TestDagQueue_GetReady(t *testing.T) {
	t.Parallel()

	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	units := []*runbase.Unit{unitA, unitB}
	q := rp.BuildQueue(units, false)

	ready := q.GetReady()
	assert.Equal(t, 1, len(ready))
	assert.Equal(t, "A", ready[0].Task.ID())
	assert.Equal(t, rp.StatusRunning, ready[0].State)
}

func TestDagQueue_MarkDone_SuccessAndUnblock(t *testing.T) {
	t.Parallel()

	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	units := []*runbase.Unit{unitA, unitB}
	q := rp.BuildQueue(units, false)

	ready := q.GetReady()
	entryA := ready[0]
	entryA.Result = rp.Result{TaskID: "A", ExitCode: 0}
	q.MarkDone(entryA, false)

	assert.Equal(t, rp.StatusSucceeded, entryA.State)
	assert.Equal(t, rp.StatusReady, q.Index["B"].State)
}

func TestDagQueue_MarkDone_FailFast(t *testing.T) {
	t.Parallel()

	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	units := []*runbase.Unit{unitA, unitB}
	q := rp.BuildQueue(units, true)

	ready := q.GetReady()
	entryA := ready[0]
	entryA.Result = rp.Result{TaskID: "A", ExitCode: 1, Err: errors.New("fail")}
	q.MarkDone(entryA, true)

	assert.Equal(t, rp.StatusFailed, entryA.State)
	assert.Equal(t, rp.StatusFailFast, q.Index["B"].State)
}

func TestDagQueue_Empty(t *testing.T) {
	t.Parallel()

	unitA := mockUnit("A")
	units := []*runbase.Unit{unitA}
	q := rp.BuildQueue(units, false)
	assert.False(t, q.Empty())
	ready := q.GetReady()
	entryA := ready[0]
	entryA.Result = rp.Result{TaskID: "A", ExitCode: 0}
	q.MarkDone(entryA, false)
	assert.True(t, q.Empty())
}

func TestDagQueue_Results_SkippedDueToFailFast(t *testing.T) {
	t.Parallel()

	unitA := mockUnit("A")
	unitB := mockUnit("B", unitA)
	units := []*runbase.Unit{unitA, unitB}
	q := rp.BuildQueue(units, true)

	ready := q.GetReady()
	entryA := ready[0]
	entryA.Result = rp.Result{TaskID: "A", ExitCode: 1, Err: errors.New("fail")}
	q.MarkDone(entryA, true)

	results := q.Results()
	assert.Equal(t, 2, len(results))
	for _, res := range results {
		if res.TaskID == "A" {
			assert.Equal(t, 1, res.ExitCode)
			assert.Error(t, res.Err)
		} else {
			assert.Equal(t, 1, res.ExitCode)
			assert.ErrorIs(t, res.Err, rp.ErrSkippedFailFast)
		}
	}
}

func TestStatus_String(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Pending", rp.StatusPending.String())
	assert.Equal(t, "Blocked", rp.StatusBlocked.String())
	assert.Equal(t, "Ready", rp.StatusReady.String())
	assert.Equal(t, "Running", rp.StatusRunning.String())
	assert.Equal(t, "Succeeded", rp.StatusSucceeded.String())
	assert.Equal(t, "Failed", rp.StatusFailed.String())
	assert.Equal(t, "AncestorFailed", rp.StatusAncestorFailed.String())
	assert.Equal(t, "FailFast", rp.StatusFailFast.String())
	assert.Equal(t, "Unknown", rp.Status(999).String())
}
