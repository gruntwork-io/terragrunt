package filter_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrentEvaluation(t *testing.T) {
	t.Parallel()

	// Create a large component list to test parallelization
	components := make([]*component.Component, 200)
	for i := range 200 {
		components[i] = &component.Component{
			Path:     fmt.Sprintf("./apps/app%d", i),
			Kind:     component.Unit,
			External: i%2 == 0,
		}
	}

	tests := []struct {
		name     string
		filter   string
		expected int
	}{
		{
			name:     "path filter with parallel evaluation",
			filter:   "./apps/app*",
			expected: 200,
		},
		{
			name:     "attribute filter with parallel evaluation",
			filter:   "type=unit",
			expected: 200,
		},
		{
			name:     "intersection with streaming pipeline",
			filter:   "./apps/app* | external=true",
			expected: 100, // half are external
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Apply(tt.filter, components)
			require.NoError(t, err)
			assert.Len(t, result, tt.expected)
		})
	}
}

func TestConcurrentFilters(t *testing.T) {
	t.Parallel()

	// Create components for testing
	components := []*component.Component{
		{Path: "./apps/app1", Kind: component.Unit, External: false},
		{Path: "./apps/app2", Kind: component.Unit, External: true},
		{Path: "./libs/lib1", Kind: component.Unit, External: false},
		{Path: "./libs/lib2", Kind: component.Stack, External: true},
	}

	// Test parallel parsing and evaluation
	filters, err := filter.ParseFilterQueries([]string{
		"./apps/*",
		"type=unit",
		"external=true",
	})
	require.NoError(t, err)

	result, err := filters.Evaluate(components)
	require.NoError(t, err)

	// Should include: app1 (apps + unit), app2 (apps + unit + external), lib1 (unit), lib2 (external)
	assert.Len(t, result, 4)
}

func TestDisableParallelizationFlag(t *testing.T) {
	t.Parallel()

	// Create a large component list that would normally trigger parallelization
	numComponents := 200
	components := make([]*component.Component, numComponents)
	for i := range numComponents {
		components[i] = &component.Component{
			Path:     fmt.Sprintf("./app-%d", i),
			Kind:     component.Unit,
			External: i%2 == 0,
		}
	}

	// Test with parallelization enabled (default)
	filter.DisableParallelization = false
	filterStr := "./app-1* | type=unit"
	f, err := filter.Parse(filterStr)
	require.NoError(t, err)

	result1, err := f.Evaluate(components)
	require.NoError(t, err)

	// Test with parallelization disabled
	filter.DisableParallelization = true
	result2, err := f.Evaluate(components)
	require.NoError(t, err)

	// Results should be identical regardless of parallelization
	assert.ElementsMatch(t, result1, result2)

	// Reset flag
	filter.DisableParallelization = false
}
