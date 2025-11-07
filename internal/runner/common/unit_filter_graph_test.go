package common_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphDependencyFilter_SimpleChain(t *testing.T) {
	t.Parallel()

	// Create a simple dependency chain: A -> B -> C
	unitA := component.NewUnit("/project/a")
	unitB := component.NewUnit("/project/b")
	unitC := component.NewUnit("/project/c")

	unitC.AddDependency(unitB)
	unitB.AddDependency(unitA)

	units := component.Units{unitA, unitB, unitC}

	// Filter for C - should include C, B, and A (all dependents and C itself)
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/c",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	// C should be included (it's the target)
	assert.False(t, unitC.Excluded(), "Target unit C should be included")
	// B should be excluded (it's a dependency of C, not a dependent)
	assert.True(t, unitB.Excluded(), "Unit B should be excluded (it's a dependency, not dependent)")
	// A should be excluded (it's a dependency of B, not a dependent)
	assert.True(t, unitA.Excluded(), "Unit A should be excluded (it's a dependency, not dependent)")
}

func TestGraphDependencyFilter_WithDependents(t *testing.T) {
	t.Parallel()

	// Create dependency structure:
	//   A
	//   |
	//   B <- target
	//   |
	//   C
	unitA := component.NewUnit("/project/a")
	unitB := component.NewUnit("/project/b")
	unitC := component.NewUnit("/project/c")

	unitB.AddDependency(unitA)
	unitC.AddDependency(unitB)

	units := component.Units{unitA, unitB, unitC}

	// Filter for B - should include B and C (C depends on B)
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/b",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	// A should be excluded (it's a dependency, not dependent)
	assert.True(t, unitA.Excluded(), "Unit A should be excluded")
	// B should be included (it's the target)
	assert.False(t, unitB.Excluded(), "Target unit B should be included")
	// C should be included (it depends on B)
	assert.False(t, unitC.Excluded(), "Unit C should be included (it's a dependent)")
}

func TestGraphDependencyFilter_ComplexGraph(t *testing.T) {
	t.Parallel()

	// Create a more complex dependency graph:
	//     A
	//    / \
	//   B   C
	//    \ /
	//     D <- target
	//     |
	//     E
	unitA := component.NewUnit("/project/a")
	unitB := component.NewUnit("/project/b")
	unitC := component.NewUnit("/project/c")
	unitD := component.NewUnit("/project/d")
	unitE := component.NewUnit("/project/e")

	unitB.AddDependency(unitA)
	unitC.AddDependency(unitA)
	units := component.Units{unitA, unitB, unitC, unitD, unitE}

	// Filter for D - should include D and E (E depends on D)
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/d",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	// A, B, C should be excluded (they are dependencies, not dependents)
	assert.True(t, unitA.Excluded(), "Unit A should be excluded")
	assert.True(t, unitB.Excluded(), "Unit B should be excluded")
	assert.True(t, unitC.Excluded(), "Unit C should be excluded")
	// D should be included (it's the target)
	assert.False(t, unitD.Excluded(), "Target unit D should be included")
	// E should be included (it depends on D)
	assert.False(t, unitE.Excluded(), "Unit E should be included")
}

func TestGraphDependencyFilter_TransitiveDependents(t *testing.T) {
	t.Parallel()

	// Create a chain to test transitive dependents:
	// A <- B <- C <- D
	// Filter for A, should include A, B, C, D
	unitA := component.NewUnit("/project/a")
	unitB := component.NewUnit("/project/b")
	unitC := component.NewUnit("/project/c")
	unitD := component.NewUnit("/project/d")

	unitB.AddDependency(unitA)
	unitC.AddDependency(unitB)
	unitD.AddDependency(unitC)

	units := component.Units{unitA, unitB, unitC, unitD}

	// Filter for A - should include all units (they all transitively depend on A)
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/a",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	assert.False(t, unitA.Excluded(), "Unit A should be included (target)")
	assert.False(t, unitB.Excluded(), "Unit B should be included (depends on A)")
	assert.False(t, unitC.Excluded(), "Unit C should be included (transitively depends on A)")
	assert.False(t, unitD.Excluded(), "Unit D should be included (transitively depends on A)")
}

func TestGraphDependencyFilter_NoDependents(t *testing.T) {
	t.Parallel()

	// Create a structure where the target has no dependents:
	// A <- B <- C (target)
	unitA := component.NewUnit("/project/a")
	unitB := component.NewUnit("/project/b")
	unitC := component.NewUnit("/project/c")

	unitB.AddDependency(unitA)
	unitC.AddDependency(unitB)

	units := component.Units{unitA, unitB, unitC}

	// Filter for C - should only include C (no dependents)
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/c",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	assert.True(t, unitA.Excluded(), "Unit A should be excluded")
	assert.True(t, unitB.Excluded(), "Unit B should be excluded")
	assert.False(t, unitC.Excluded(), "Unit C should be included (target)")
}

func TestGraphDependencyFilter_MultiplePathsToTarget(t *testing.T) {
	t.Parallel()

	// Create a diamond dependency:
	//     A
	//    / \
	//   B   C
	//    \ /
	//     D
	//     |
	//     E <- target
	unitA := component.NewUnit("/project/a")
	unitB := component.NewUnit("/project/b")
	unitC := component.NewUnit("/project/c")
	unitD := component.NewUnit("/project/d")
	unitE := component.NewUnit("/project/e")

	unitB.AddDependency(unitA)
	unitC.AddDependency(unitA)
	unitD.AddDependency(unitB)
	unitD.AddDependency(unitC)
	unitE.AddDependency(unitD)

	units := component.Units{unitA, unitB, unitC, unitD, unitE}

	// Filter for E - should only include E (no dependents)
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/e",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	assert.True(t, unitA.Excluded(), "Unit A should be excluded")
	assert.True(t, unitB.Excluded(), "Unit B should be excluded")
	assert.True(t, unitC.Excluded(), "Unit C should be excluded")
	assert.True(t, unitD.Excluded(), "Unit D should be excluded")
	assert.False(t, unitE.Excluded(), "Unit E should be included (target)")
}

func TestGraphDependencyFilter_IsolatedUnits(t *testing.T) {
	t.Parallel()

	// Create units with no dependencies
	unitA := component.NewUnit("/project/a")
	unitB := component.NewUnit("/project/b")
	unitC := component.NewUnit("/project/c")

	units := component.Units{unitA, unitB, unitC}

	// Filter for B - should only include B
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/b",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	assert.True(t, unitA.Excluded(), "Unit A should be excluded (no relationship)")
	assert.False(t, unitB.Excluded(), "Unit B should be included (target)")
	assert.True(t, unitC.Excluded(), "Unit C should be excluded (no relationship)")
}

func TestGraphDependencyFilter_EmptyUnits(t *testing.T) {
	t.Parallel()

	units := component.Units{}

	filter := &common.UnitFilterGraph{
		TargetDir: "/project/a",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)
	assert.Empty(t, units, "Empty units should remain empty")
}

func TestGraphDependencyFilter_NonExistentTarget(t *testing.T) {
	t.Parallel()

	// Create units but target a non-existent directory
	unitA := component.NewUnit("/project/a")
	unitB := component.NewUnit("/project/b")

	unitB.AddDependency(unitA)

	units := component.Units{unitA, unitB}

	// Filter for non-existent target - all should be excluded
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/nonexistent",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	assert.True(t, unitA.Excluded(), "Unit A should be excluded")
	assert.True(t, unitB.Excluded(), "Unit B should be excluded")
}
