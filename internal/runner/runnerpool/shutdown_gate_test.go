package runnerpool_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
)

// TestUnitsWithDependents verifies a unit is flagged iff another in-run unit depends on
// it. Flagged units must not have their engine shut down early, since a dependent
// re-reads their outputs through engine.Run, which would re-spawn the engine.
func TestUnitsWithDependents(t *testing.T) {
	t.Parallel()

	d := component.NewUnit("/tmp/test/d")
	y := component.NewUnit("/tmp/test/y")
	leaf := component.NewUnit("/tmp/test/leaf")

	y.AddDependency(d) // y depends on d, so d has a dependent

	q := &queue.Queue{Entries: queue.Entries{
		{Component: d},
		{Component: y},
		{Component: leaf},
	}}

	got := runnerpool.UnitsWithDependents(q)

	assert.True(t, got[d.Path()], "d has a dependent (y) → must be kept alive, not shut down early")
	assert.False(t, got[y.Path()], "y has no dependents → safe to shut down early")
	assert.False(t, got[leaf.Path()], "an independent unit → safe to shut down early")
}

// TestUnitsWithDependents_NilQueue is defensive: a nil queue yields an empty set
// (every unit then treated as having no dependents).
func TestUnitsWithDependents_NilQueue(t *testing.T) {
	t.Parallel()

	assert.Empty(t, runnerpool.UnitsWithDependents(nil))
}
