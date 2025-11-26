package runnerpool_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetUnitFilters_Deduplication tests that SetUnitFilters properly deduplicates filters
func TestSetUnitFilters_Deduplication(t *testing.T) {
	t.Parallel()

	runner := &runnerpool.Runner{
		Stack: &component.Stack{},
	}

	// Create a filter instance
	filter1 := &runnerpool.UnitFilterGraph{TargetDir: "/project/a"}

	// Add the same filter twice
	runner.SetUnitFilters(filter1)
	runner.SetUnitFilters(filter1)

	// Should only have one filter
	assert.Len(t, runner.GetUnitFilters(), 1, "Duplicate filters should be removed")
}

// TestSetUnitFilters_DifferentFilters tests that different filters are all added
func TestSetUnitFilters_DifferentFilters(t *testing.T) {
	t.Parallel()

	runner := &runnerpool.Runner{
		Stack: &component.Stack{},
	}

	// Create different filter instances
	filter1 := &runnerpool.UnitFilterGraph{TargetDir: "/project/a"}
	filter2 := &runnerpool.UnitFilterGraph{TargetDir: "/project/b"}

	runner.SetUnitFilters(filter1)
	runner.SetUnitFilters(filter2)

	// Should have two different filters
	assert.Len(t, runner.GetUnitFilters(), 2, "Different filters should all be added")
}

// TestSetUnitFilters_MultipleCallsWithMixedFilters tests deduplication with multiple calls
func TestSetUnitFilters_MultipleCallsWithMixedFilters(t *testing.T) {
	t.Parallel()

	runner := &runnerpool.Runner{
		Stack: &component.Stack{},
	}

	filter1 := &runnerpool.UnitFilterGraph{TargetDir: "/project/a"}
	filter2 := &runnerpool.UnitFilterGraph{TargetDir: "/project/b"}
	filter3 := &runnerpool.UnitFilterGraph{TargetDir: "/project/c"}

	// First call with filter1 and filter2
	runner.SetUnitFilters(filter1, filter2)
	assert.Len(t, runner.GetUnitFilters(), 2, "Should have 2 filters after first call")

	// Second call with filter2 (duplicate) and filter3 (new)
	runner.SetUnitFilters(filter2, filter3)
	assert.Len(t, runner.GetUnitFilters(), 3, "Should have 3 filters after second call (filter2 deduplicated)")

	// Verify the order is preserved (filter1, filter2, filter3)
	assert.Equal(t, filter1, runner.GetUnitFilters()[0], "First filter should be filter1")
	assert.Equal(t, filter2, runner.GetUnitFilters()[1], "Second filter should be filter2")
	assert.Equal(t, filter3, runner.GetUnitFilters()[2], "Third filter should be filter3")
}

// TestSetUnitFilters_SameValuesDifferentInstances tests that filters with same values are deduplicated
func TestSetUnitFilters_SameValuesDifferentInstances(t *testing.T) {
	t.Parallel()

	runner := &runnerpool.Runner{
		Stack: &component.Stack{},
	}

	// Create two separate instances with the same values
	filter1 := &runnerpool.UnitFilterGraph{TargetDir: "/project/a"}
	filter2 := &runnerpool.UnitFilterGraph{TargetDir: "/project/a"}

	runner.SetUnitFilters(filter1)
	runner.SetUnitFilters(filter2)

	// Should deduplicate based on value equality
	assert.Len(t, runner.GetUnitFilters(), 1, "Filters with same values should be deduplicated")
}

// TestSetUnitFilters_EmptyCall tests that calling with no filters doesn't break anything
func TestSetUnitFilters_EmptyCall(t *testing.T) {
	t.Parallel()

	runner := &runnerpool.Runner{
		Stack: &component.Stack{},
	}

	filter1 := &runnerpool.UnitFilterGraph{TargetDir: "/project/a"}

	runner.SetUnitFilters(filter1)
	assert.Len(t, runner.GetUnitFilters(), 1, "Should have 1 filter")

	// Call with no filters
	runner.SetUnitFilters()
	assert.Len(t, runner.GetUnitFilters(), 1, "Should still have 1 filter after empty call")
}

// mockFilter is a custom filter for testing
type mockFilter struct {
	ID string
}

func (m *mockFilter) Filter(ctx context.Context, units []*component.Unit, opts *options.TerragruntOptions) error {
	return nil
}

// TestSetUnitFilters_CustomFilterType tests deduplication with custom filter types
func TestSetUnitFilters_CustomFilterType(t *testing.T) {
	t.Parallel()

	runner := &runnerpool.Runner{
		Stack: &component.Stack{},
	}

	filter1 := &mockFilter{ID: "test1"}
	filter2 := &mockFilter{ID: "test1"} // Same values
	filter3 := &mockFilter{ID: "test2"} // Different values

	runner.SetUnitFilters(filter1)
	runner.SetUnitFilters(filter2) // Should be deduplicated
	runner.SetUnitFilters(filter3)

	// Should have 2 filters (filter2 deduplicated)
	assert.Len(t, runner.GetUnitFilters(), 2, "Should have 2 filters with custom type")
}

// TestSetUnitFilters_MixedFilterTypes tests deduplication with mixed filter types
func TestSetUnitFilters_MixedFilterTypes(t *testing.T) {
	t.Parallel()

	runner := &runnerpool.Runner{
		Stack: &component.Stack{},
	}

	graphFilter := &runnerpool.UnitFilterGraph{TargetDir: "/project/a"}
	customFilter := &mockFilter{ID: "test"}
	compositeFilter := &runnerpool.CompositeFilter{
		Filters: []runnerpool.UnitFilter{graphFilter},
	}

	runner.SetUnitFilters(graphFilter, customFilter, compositeFilter)

	// Should have 3 different filter types
	assert.Len(t, runner.GetUnitFilters(), 3, "Should have all 3 different filter types")

	// Add duplicates
	runner.SetUnitFilters(graphFilter, customFilter)

	// Should still have 3 (duplicates deduplicated)
	assert.Len(t, runner.GetUnitFilters(), 3, "Duplicates should be removed")
}

// TestSetUnitFilters_OrderPreserved tests that filter order is preserved
func TestSetUnitFilters_OrderPreserved(t *testing.T) {
	t.Parallel()

	runner := &runnerpool.Runner{
		Stack: &component.Stack{},
	}

	filter1 := &runnerpool.UnitFilterGraph{TargetDir: "/project/a"}
	filter2 := &runnerpool.UnitFilterGraph{TargetDir: "/project/b"}
	filter3 := &runnerpool.UnitFilterGraph{TargetDir: "/project/c"}

	runner.SetUnitFilters(filter1, filter2, filter3)

	// Verify order
	require.Len(t, runner.GetUnitFilters(), 3)
	assert.Equal(t, filter1, runner.GetUnitFilters()[0])
	assert.Equal(t, filter2, runner.GetUnitFilters()[1])
	assert.Equal(t, filter3, runner.GetUnitFilters()[2])

	// Add some duplicates and a new one
	filter4 := &runnerpool.UnitFilterGraph{TargetDir: "/project/d"}
	runner.SetUnitFilters(filter2, filter4, filter1) // filter2 and filter1 are duplicates

	// Order should be: filter1, filter2, filter3, filter4 (duplicates skipped)
	require.Len(t, runner.GetUnitFilters(), 4)
	assert.Equal(t, filter1, runner.GetUnitFilters()[0])
	assert.Equal(t, filter2, runner.GetUnitFilters()[1])
	assert.Equal(t, filter3, runner.GetUnitFilters()[2])
	assert.Equal(t, filter4, runner.GetUnitFilters()[3])
}
