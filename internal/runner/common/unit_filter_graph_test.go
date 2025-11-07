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
	unitA := &common.Unit{Component: component.NewUnit("/project/a")}
	unitB := &common.Unit{Component: component.NewUnit("/project/b")}
	unitC := &common.Unit{Component: component.NewUnit("/project/c")}

	unitC.Component.AddDependency(unitB.Component)
	unitB.Component.AddDependency(unitA.Component)

	units := common.Units{unitA, unitB, unitC}

	// Filter for C - should include C, B, and A (all dependents and C itself)
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/c",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	// C should be included (it's the target)
	assert.False(t, unitC.Component.Excluded(), "Target unit C should be included")
	// B should be excluded (it's a dependency of C, not a dependent)
	assert.True(t, unitB.Component.Excluded(), "Unit B should be excluded (it's a dependency, not dependent)")
	// A should be excluded (it's a dependency of B, not a dependent)
	assert.True(t, unitA.Component.Excluded(), "Unit A should be excluded (it's a dependency, not dependent)")
}

func TestGraphDependencyFilter_WithDependents(t *testing.T) {
	t.Parallel()

	// Create dependency structure:
	//   A
	//   |
	//   B <- target
	//   |
	//   C
	unitA := &common.Unit{Component: component.NewUnit("/project/a")}
	unitB := &common.Unit{Component: component.NewUnit("/project/b")}
	unitC := &common.Unit{Component: component.NewUnit("/project/c")}

	unitB.Component.AddDependency(unitA.Component)
	unitC.Component.AddDependency(unitB.Component)

	units := common.Units{unitA, unitB, unitC}

	// Filter for B - should include B and C (C depends on B)
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/b",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	// A should be excluded (it's a dependency, not dependent)
	assert.True(t, unitA.Component.Excluded(), "Unit A should be excluded")
	// B should be included (it's the target)
	assert.False(t, unitB.Component.Excluded(), "Target unit B should be included")
	// C should be included (it depends on B)
	assert.False(t, unitC.Component.Excluded(), "Unit C should be included (it's a dependent)")
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
	unitA := &common.Unit{Component: component.NewUnit("/project/a")}
	unitB := &common.Unit{Component: component.NewUnit("/project/b")}
	unitC := &common.Unit{Component: component.NewUnit("/project/c")}
	unitD := &common.Unit{Component: component.NewUnit("/project/d")}
	unitE := &common.Unit{Component: component.NewUnit("/project/e")}

	unitB.Component.AddDependency(unitA.Component)
	unitC.Component.AddDependency(unitA.Component)
	unitD.Component.AddDependency(unitB.Component)
	unitD.Component.AddDependency(unitC.Component)
	unitE.Component.AddDependency(unitD.Component)

	units := common.Units{unitA, unitB, unitC, unitD, unitE}

	// Filter for D - should include D and E (E depends on D)
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/d",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	// A, B, C should be excluded (they are dependencies, not dependents)
	assert.True(t, unitA.Component.Excluded(), "Unit A should be excluded")
	assert.True(t, unitB.Component.Excluded(), "Unit B should be excluded")
	assert.True(t, unitC.Component.Excluded(), "Unit C should be excluded")
	// D should be included (it's the target)
	assert.False(t, unitD.Component.Excluded(), "Target unit D should be included")
	// E should be included (it depends on D)
	assert.False(t, unitE.Component.Excluded(), "Unit E should be included")
}

func TestGraphDependencyFilter_TransitiveDependents(t *testing.T) {
	t.Parallel()

	// Create a chain to test transitive dependents:
	// A <- B <- C <- D
	// Filter for A, should include A, B, C, D
	unitA := &common.Unit{Component: component.NewUnit("/project/a")}
	unitB := &common.Unit{Component: component.NewUnit("/project/b")}
	unitC := &common.Unit{Component: component.NewUnit("/project/c")}
	unitD := &common.Unit{Component: component.NewUnit("/project/d")}

	unitB.Component.AddDependency(unitA.Component)
	unitC.Component.AddDependency(unitB.Component)
	unitD.Component.AddDependency(unitC.Component)

	units := common.Units{unitA, unitB, unitC, unitD}

	// Filter for A - should include all units (they all transitively depend on A)
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/a",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	assert.False(t, unitA.Component.Excluded(), "Unit A should be included (target)")
	assert.False(t, unitB.Component.Excluded(), "Unit B should be included (depends on A)")
	assert.False(t, unitC.Component.Excluded(), "Unit C should be included (transitively depends on A)")
	assert.False(t, unitD.Component.Excluded(), "Unit D should be included (transitively depends on A)")
}

func TestGraphDependencyFilter_NoDependents(t *testing.T) {
	t.Parallel()

	// Create a structure where the target has no dependents:
	// A <- B <- C (target)
	unitA := &common.Unit{Component: component.NewUnit("/project/a")}
	unitB := &common.Unit{Component: component.NewUnit("/project/b")}
	unitC := &common.Unit{Component: component.NewUnit("/project/c")}

	unitB.Component.AddDependency(unitA.Component)
	unitC.Component.AddDependency(unitB.Component)

	units := common.Units{unitA, unitB, unitC}

	// Filter for C - should only include C (no dependents)
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/c",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	assert.True(t, unitA.Component.Excluded(), "Unit A should be excluded")
	assert.True(t, unitB.Component.Excluded(), "Unit B should be excluded")
	assert.False(t, unitC.Component.Excluded(), "Unit C should be included (target)")
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
	unitA := &common.Unit{Component: component.NewUnit("/project/a")}
	unitB := &common.Unit{Component: component.NewUnit("/project/b")}
	unitC := &common.Unit{Component: component.NewUnit("/project/c")}
	unitD := &common.Unit{Component: component.NewUnit("/project/d")}
	unitE := &common.Unit{Component: component.NewUnit("/project/e")}

	unitB.Component.AddDependency(unitA.Component)
	unitC.Component.AddDependency(unitA.Component)
	unitD.Component.AddDependency(unitB.Component)
	unitD.Component.AddDependency(unitC.Component)
	unitE.Component.AddDependency(unitD.Component)

	units := common.Units{unitA, unitB, unitC, unitD, unitE}

	// Filter for E - should only include E (no dependents)
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/e",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	assert.True(t, unitA.Component.Excluded(), "Unit A should be excluded")
	assert.True(t, unitB.Component.Excluded(), "Unit B should be excluded")
	assert.True(t, unitC.Component.Excluded(), "Unit C should be excluded")
	assert.True(t, unitD.Component.Excluded(), "Unit D should be excluded")
	assert.False(t, unitE.Component.Excluded(), "Unit E should be included (target)")
}

func TestGraphDependencyFilter_IsolatedUnits(t *testing.T) {
	t.Parallel()

	// Create units with no dependencies
	unitA := &common.Unit{Component: component.NewUnit("/project/a")}
	unitB := &common.Unit{Component: component.NewUnit("/project/b")}
	unitC := &common.Unit{Component: component.NewUnit("/project/c")}

	units := common.Units{unitA, unitB, unitC}

	// Filter for B - should only include B
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/b",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	assert.True(t, unitA.Component.Excluded(), "Unit A should be excluded (no relationship)")
	assert.False(t, unitB.Component.Excluded(), "Unit B should be included (target)")
	assert.True(t, unitC.Component.Excluded(), "Unit C should be excluded (no relationship)")
}

func TestGraphDependencyFilter_EmptyUnits(t *testing.T) {
	t.Parallel()

	units := common.Units{}

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
	unitA := &common.Unit{Component: component.NewUnit("/project/a")}
	unitB := &common.Unit{Component: component.NewUnit("/project/b")}

	unitB.Component.AddDependency(unitA.Component)

	units := common.Units{unitA, unitB}

	// Filter for non-existent target - all should be excluded
	filter := &common.UnitFilterGraph{
		TargetDir: "/project/nonexistent",
	}

	err := filter.Filter(context.Background(), units, &options.TerragruntOptions{})
	require.NoError(t, err)

	assert.True(t, unitA.Component.Excluded(), "Unit A should be excluded")
	assert.True(t, unitB.Component.Excluded(), "Unit B should be excluded")
}
