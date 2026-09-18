package queue_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// destroyPlan marks a unit as a "down" entry, the way a unit deleted in a git-range filter is
// scheduled under --filter-allow-destroy.
func destroyPlan(u *component.Unit) *component.Unit {
	return u.WithDiscoveryContext(&component.DiscoveryContext{Cmd: "plan", Args: []string{"-destroy"}})
}

// drainable reports whether a queue with no running entries can still make progress: either it
// is finished, or something is dispatchable. A queue that is neither is the hang - Controller.Run
// parks on readyCh with no goroutine left to signal it.
func drainable(t *testing.T, q *queue.Queue) bool {
	t.Helper()

	return q.Finished() || len(q.GetReadyWithDependencies(logger.CreateLogger())) > 0
}

// TestFailedDestroyDoesNotEarlyExitUpDependencies is the regression test for the
// `run --all --filter-allow-destroy` hang. A deleted unit's `plan -destroy` (down) shares a
// surviving dependency with surviving units (up):
//
//	shared (up) <- consumer (up)
//	shared (up) <- gone     (down)
//
// Readiness already ignores cross-direction edges (#6322): `gone` never waited on `shared`. Failure
// propagation must not cross them either. Before the fix, FailEntry(gone) marked `shared`
// EarlyExit; `consumer` needs `shared` to be Succeeded, nothing ever walked from `shared` to
// `consumer`, and the queue could neither finish nor dispatch anything - forever.
func TestFailedDestroyDoesNotEarlyExitUpDependencies(t *testing.T) {
	t.Parallel()

	shared := component.NewUnit("/repo/shared")
	consumer := component.NewUnit("/repo/consumer")
	consumer.AddDependency(shared)

	gone := destroyPlan(component.NewUnit("/wt/from/gone"))
	gone.AddDependency(shared)

	q, err := queue.NewQueue(component.Components{shared, consumer, gone})
	require.NoError(t, err)

	goneEntry := q.EntryByPath(gone.Path())
	require.True(t, q.ClaimForRunning(goneEntry))
	q.FailEntry(goneEntry) // fails while `shared` is still Ready (not yet claimed)

	sharedEntry := q.EntryByPath(shared.Path())
	assert.Equal(t, queue.StatusReady, sharedEntry.Status,
		"a failed destroy must not cancel a surviving unit it never gated on")
	assert.True(t, drainable(t, q), "the queue must still be able to make progress")

	// The surviving chain runs to completion as if `gone` had never been there.
	require.True(t, q.ClaimForRunning(sharedEntry))
	q.SetEntryStatus(sharedEntry, queue.StatusSucceeded)

	consumerEntry := q.EntryByPath(consumer.Path())
	require.ElementsMatch(t, []*queue.Entry{consumerEntry}, q.GetReadyWithDependencies(logger.CreateLogger()))
	require.True(t, q.ClaimForRunning(consumerEntry))
	q.SetEntryStatus(consumerEntry, queue.StatusSucceeded)

	assert.True(t, q.Finished())
	assert.Equal(t, queue.StatusFailed, goneEntry.Status, "the destroy's own failure is still recorded")
}

// TestFailedPlanDoesNotEarlyExitDownDependents is the mirror image. A surviving unit's plan (up)
// fails while a deleted unit that depends on it (down) has its own down dependency still gated:
//
//	shared (up) <- gone (down) <- goneDep (down)   (goneDep is destroyed after gone)
//
// `gone` never waited on `shared`, so its failure must not cancel `gone`. Before the fix,
// FailEntry(shared) marked `gone` EarlyExit; `goneDep` needs `gone` to be Succeeded before it can be
// destroyed, and stayed Ready forever.
func TestFailedPlanDoesNotEarlyExitDownDependents(t *testing.T) {
	t.Parallel()

	shared := component.NewUnit("/repo/shared")
	goneDep := destroyPlan(component.NewUnit("/wt/from/gone-dep"))
	gone := destroyPlan(component.NewUnit("/wt/from/gone"))
	gone.AddDependency(shared)
	gone.AddDependency(goneDep)

	q, err := queue.NewQueue(component.Components{shared, gone, goneDep})
	require.NoError(t, err)

	sharedEntry := q.EntryByPath(shared.Path())
	require.True(t, q.ClaimForRunning(sharedEntry))
	q.FailEntry(sharedEntry) // fails while `gone` is still Ready

	goneEntry := q.EntryByPath(gone.Path())
	assert.Equal(t, queue.StatusReady, goneEntry.Status,
		"a failed plan must not cancel a destroy that never gated on it")
	assert.True(t, drainable(t, q))

	require.True(t, q.ClaimForRunning(goneEntry))
	q.SetEntryStatus(goneEntry, queue.StatusSucceeded)

	goneDepEntry := q.EntryByPath(goneDep.Path())
	require.ElementsMatch(t, []*queue.Entry{goneDepEntry}, q.GetReadyWithDependencies(logger.CreateLogger()))
	require.True(t, q.ClaimForRunning(goneDepEntry))
	q.SetEntryStatus(goneDepEntry, queue.StatusSucceeded)

	assert.True(t, q.Finished())
}

// TestSameDirectionPropagationUnchanged pins that the fix is scoped to mixed queues: within one
// direction, a failure still early-exits everything downstream of it exactly as before.
func TestSameDirectionPropagationUnchanged(t *testing.T) {
	t.Parallel()

	t.Run("up: failure early-exits dependents", func(t *testing.T) {
		t.Parallel()

		a := component.NewUnit("/repo/a")
		b := component.NewUnit("/repo/b")
		c := component.NewUnit("/repo/c")
		b.AddDependency(a)
		c.AddDependency(b)

		q, err := queue.NewQueue(component.Components{a, b, c})
		require.NoError(t, err)

		aEntry := q.EntryByPath(a.Path())
		require.True(t, q.ClaimForRunning(aEntry))
		q.FailEntry(aEntry)

		assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath(b.Path()).Status)
		assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath(c.Path()).Status)
		assert.True(t, q.Finished())
	})

	t.Run("down: failure early-exits dependencies", func(t *testing.T) {
		t.Parallel()

		a := destroyPlan(component.NewUnit("/wt/a"))
		b := destroyPlan(component.NewUnit("/wt/b"))
		c := destroyPlan(component.NewUnit("/wt/c"))
		b.AddDependency(a)
		c.AddDependency(b)

		q, err := queue.NewQueue(component.Components{a, b, c})
		require.NoError(t, err)

		// Destroy order is c, b, a; c fails first.
		cEntry := q.EntryByPath(c.Path())
		require.True(t, q.ClaimForRunning(cEntry))
		q.FailEntry(cEntry)

		assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath(b.Path()).Status)
		assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath(a.Path()).Status)
		assert.True(t, q.Finished())
	})
}

// TestIncidentShapeDrainsAfterDestroyFailure reproduces the dependency shape from the production
// incident this fixes (Optimuse infrastructure PR deleting 11 dev units; two CI runs hung for 3h
// and 20h respectively). The deleted `workflows/ifc-lift` (down) failed at HCL evaluation and
// early-exited its surviving dependencies `agent-core` and - via the deleted `engines/ifc-lift` -
// `otel-collector`, which had not started yet. Every surviving `engines/*` unit gated on
// `otel-collector`, and every surviving `workflows/*` unit gated on `agent-core`, then sat Ready
// forever. With the fix, the destroy failure is contained to the deleted units.
func TestIncidentShapeDrainsAfterDestroyFailure(t *testing.T) {
	t.Parallel()

	// Surviving (up).
	ciAuth := component.NewUnit("/repo/ci-auth")
	jena := component.NewUnit("/repo/jena")
	batch := component.NewUnit("/repo/batch")
	agentCore := component.NewUnit("/repo/agent-core")
	otelCollector := component.NewUnit("/repo/otel-collector")
	engineEnergyplus := component.NewUnit("/repo/engines/energyplus")
	workflowSimE2E := component.NewUnit("/repo/workflows/simulation-e2e")
	agentCore.AddDependency(ciAuth)
	agentCore.AddDependency(jena)
	otelCollector.AddDependency(batch)
	engineEnergyplus.AddDependency(otelCollector)
	engineEnergyplus.AddDependency(batch)
	workflowSimE2E.AddDependency(engineEnergyplus)
	workflowSimE2E.AddDependency(agentCore)

	// Deleted (down), discovered from the origin/main worktree.
	engineIfcLift := destroyPlan(component.NewUnit("/wt/engines/ifc-lift"))
	engineIfcLift.AddDependency(ciAuth)
	engineIfcLift.AddDependency(batch)
	engineIfcLift.AddDependency(otelCollector)
	engineIfcLift.AddDependency(jena)
	workflowIfcLift := destroyPlan(component.NewUnit("/wt/workflows/ifc-lift"))
	workflowIfcLift.AddDependency(batch)
	workflowIfcLift.AddDependency(engineIfcLift)
	workflowIfcLift.AddDependency(agentCore)

	q, err := queue.NewQueue(component.Components{
		ciAuth, jena, batch, agentCore, otelCollector, engineEnergyplus, workflowSimE2E,
		engineIfcLift, workflowIfcLift,
	})
	require.NoError(t, err)

	l := logger.CreateLogger()

	// Pass 1 as the Controller would do it: ci-auth, jena, batch (no deps) and workflows/ifc-lift
	// (down, nothing depends on it) are claimed. agent-core / otel-collector are gated.
	for _, e := range q.GetReadyWithDependencies(l) {
		require.True(t, q.ClaimForRunning(e))
	}

	require.ElementsMatch(t,
		[]string{ciAuth.Path(), jena.Path(), batch.Path(), workflowIfcLift.Path()},
		paths(runningEntries(q)))

	// The deleted workflow fails at HCL evaluation while its surviving dependencies are still gated.
	q.FailEntry(q.EntryByPath(workflowIfcLift.Path()))

	assert.Equal(t, queue.StatusEarlyExit, q.EntryByPath(engineIfcLift.Path()).Status,
		"the other deleted unit, destroyed after its dependent, is correctly cancelled")
	assert.Equal(t, queue.StatusReady, q.EntryByPath(agentCore.Path()).Status,
		"a surviving unit must not be cancelled by a deleted unit's failure")
	assert.Equal(t, queue.StatusReady, q.EntryByPath(otelCollector.Path()).Status)

	// Drive the surviving graph to completion: everything up must finish.
	for _, e := range runningEntries(q) {
		q.SetEntryStatus(e, queue.StatusSucceeded)
	}

	for range 10 {
		ready := q.GetReadyWithDependencies(l)
		if len(ready) == 0 {
			break
		}

		for _, e := range ready {
			require.True(t, q.ClaimForRunning(e))
			q.SetEntryStatus(e, queue.StatusSucceeded)
		}
	}

	require.True(t, q.Finished(), "with the fix the queue drains; before it, 4 surviving units stayed Ready forever")

	for _, u := range []*component.Unit{ciAuth, jena, batch, agentCore, otelCollector, engineEnergyplus, workflowSimE2E} {
		assert.Equal(t, queue.StatusSucceeded, q.EntryByPath(u.Path()).Status, u.Path())
	}
}

func paths(entries []*queue.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Component.Path())
	}

	return out
}

func runningEntries(q *queue.Queue) []*queue.Entry {
	var out []*queue.Entry

	for _, e := range q.Entries {
		if e.Status == queue.StatusRunning {
			out = append(out, e)
		}
	}

	return out
}
