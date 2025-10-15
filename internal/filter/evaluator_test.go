package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluate_PathFilter(t *testing.T) {
	t.Parallel()

	units := []filter.Unit{
		{Name: "app1", Path: "./apps/app1"},
		{Name: "app2", Path: "./apps/app2"},
		{Name: "legacy", Path: "./apps/legacy"},
		{Name: "db", Path: "./libs/db"},
		{Name: "api", Path: "./libs/api"},
		{Name: "nested", Path: "./apps/subdir/nested"},
	}

	tests := []struct {
		name     string
		filter   *filter.PathFilter
		expected []filter.Unit
	}{
		{
			name:   "exact path match",
			filter: &filter.PathFilter{Value: "./apps/app1"},
			expected: []filter.Unit{
				{Name: "app1", Path: "./apps/app1"},
			},
		},
		{
			name:   "glob with single wildcard",
			filter: &filter.PathFilter{Value: "./apps/*"},
			expected: []filter.Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "legacy", Path: "./apps/legacy"},
			},
		},
		{
			name:   "glob with recursive wildcard",
			filter: &filter.PathFilter{Value: "./apps/**"},
			expected: []filter.Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "legacy", Path: "./apps/legacy"},
				{Name: "nested", Path: "./apps/subdir/nested"},
			},
		},
		{
			name:   "glob matching specific subdirectory",
			filter: &filter.PathFilter{Value: "./libs/*"},
			expected: []filter.Unit{
				{Name: "db", Path: "./libs/db"},
				{Name: "api", Path: "./libs/api"},
			},
		},
		{
			name:     "no matches",
			filter:   &filter.PathFilter{Value: "./nonexistent/*"},
			expected: []filter.Unit{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.filter, units)
			require.NoError(t, err)

			// Sort for consistent comparison
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter(t *testing.T) {
	t.Parallel()

	units := []filter.Unit{
		{Name: "app", Path: "./apps/app"},
		{Name: "app", Path: "./libs/app"}, // Same name, different path
		{Name: "db", Path: "./libs/db"},
		{Name: "api", Path: "./libs/api"},
	}

	tests := []struct {
		name     string
		filter   *filter.AttributeFilter
		expected []filter.Unit
	}{
		{
			name:   "name filter single match",
			filter: &filter.AttributeFilter{Key: "name", Value: "db"},
			expected: []filter.Unit{
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name:   "name filter multiple matches",
			filter: &filter.AttributeFilter{Key: "name", Value: "app"},
			expected: []filter.Unit{
				{Name: "app", Path: "./apps/app"},
				{Name: "app", Path: "./libs/app"},
			},
		},
		{
			name:     "name filter no matches",
			filter:   &filter.AttributeFilter{Key: "name", Value: "nonexistent"},
			expected: []filter.Unit{},
		},
		{
			name:     "type filter",
			filter:   &filter.AttributeFilter{Key: "type", Value: "unit"},
			expected: units, // All units match type=unit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.filter, units)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter_InvalidKey(t *testing.T) {
	t.Parallel()

	units := []filter.Unit{
		{Name: "app", Path: "./apps/app"},
	}

	attrFilter := &filter.AttributeFilter{Key: "invalid", Value: "foo"}
	result, err := filter.Evaluate(attrFilter, units)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unknown attribute key")
}

func TestEvaluate_PrefixExpression(t *testing.T) {
	t.Parallel()

	units := []filter.Unit{
		{Name: "app1", Path: "./apps/app1"},
		{Name: "app2", Path: "./apps/app2"},
		{Name: "legacy", Path: "./apps/legacy"},
		{Name: "db", Path: "./libs/db"},
	}

	tests := []struct {
		name     string
		expr     *filter.PrefixExpression
		expected []filter.Unit
	}{
		{
			name: "exclude by name",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.AttributeFilter{Key: "name", Value: "legacy"},
			},
			expected: []filter.Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name: "exclude by path",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.PathFilter{Value: "./apps/legacy"},
			},
			expected: []filter.Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name: "exclude by glob",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.PathFilter{Value: "./apps/*"},
			},
			expected: []filter.Unit{
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name: "exclude all (double negation effect)",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.AttributeFilter{Key: "type", Value: "unit"},
			},
			expected: []filter.Unit{}, // All excluded
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.expr, units)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_InfixExpression(t *testing.T) {
	t.Parallel()

	units := []filter.Unit{
		{Name: "app1", Path: "./apps/app1"},
		{Name: "app2", Path: "./apps/app2"},
		{Name: "legacy", Path: "./apps/legacy"},
		{Name: "db", Path: "./libs/db"},
		{Name: "api", Path: "./libs/api"},
	}

	tests := []struct {
		name     string
		expr     *filter.InfixExpression
		expected []filter.Unit
	}{
		{
			name: "intersection of path and name",
			expr: &filter.InfixExpression{
				Left:     &filter.PathFilter{Value: "./apps/*"},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "app1"},
			},
			expected: []filter.Unit{
				{Name: "app1", Path: "./apps/app1"},
			},
		},
		{
			name: "intersection with no overlap",
			expr: &filter.InfixExpression{
				Left:     &filter.PathFilter{Value: "./apps/*"},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "db"}, // db is in ./libs/, not ./apps/
			},
			expected: []filter.Unit{},
		},
		{
			name: "intersection of exact path and name",
			expr: &filter.InfixExpression{
				Left:     &filter.PathFilter{Value: "./apps/app1"},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "app1"},
			},
			expected: []filter.Unit{
				{Name: "app1", Path: "./apps/app1"},
			},
		},
		{
			name: "intersection of empty results",
			expr: &filter.InfixExpression{
				Left:     &filter.AttributeFilter{Key: "name", Value: "nonexistent1"},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "app1"}, // Can't refine empty set
			},
			expected: []filter.Unit{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.expr, units)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_ComplexExpressions(t *testing.T) {
	t.Parallel()

	units := []filter.Unit{
		{Name: "app1", Path: "./apps/app1"},
		{Name: "app2", Path: "./apps/app2"},
		{Name: "legacy", Path: "./apps/legacy"},
		{Name: "db", Path: "./libs/db"},
		{Name: "api", Path: "./libs/api"},
		{Name: "special", Path: "./special/unit"},
	}

	tests := []struct {
		name     string
		expr     filter.Expression
		expected []filter.Unit
	}{
		{
			name: "intersection with negation (refinement)",
			expr: &filter.InfixExpression{
				Left:     &filter.PathFilter{Value: "./apps/*"},
				Operator: "|",
				Right:    &filter.PrefixExpression{Operator: "!", Right: &filter.AttributeFilter{Key: "name", Value: "legacy"}},
			},
			expected: []filter.Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				// legacy excluded
			},
		},
		{
			name: "negated intersection",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right: &filter.InfixExpression{
					Left:     &filter.PathFilter{Value: "./apps/*"},
					Operator: "|",
					Right:    &filter.AttributeFilter{Key: "name", Value: "app1"},
				},
			},
			expected: []filter.Unit{
				{Name: "app2", Path: "./apps/app2"},
				{Name: "legacy", Path: "./apps/legacy"},
				{Name: "db", Path: "./libs/db"},
				{Name: "api", Path: "./libs/api"},
				{Name: "special", Path: "./special/unit"},
				// Everything except app1
			},
		},
		{
			name: "chained intersections (multiple refinements)",
			expr: &filter.InfixExpression{
				Left: &filter.InfixExpression{
					Left:     &filter.PathFilter{Value: "./apps/*"},
					Operator: "|",
					Right:    &filter.PrefixExpression{Operator: "!", Right: &filter.AttributeFilter{Key: "name", Value: "legacy"}},
				},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "app1"},
			},
			expected: []filter.Unit{
				{Name: "app1", Path: "./apps/app1"},
				// Only app1 from ./apps/* after excluding legacy
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.expr, units)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil expression", func(t *testing.T) {
		t.Parallel()

		units := []filter.Unit{{Name: "app", Path: "./app"}}
		result, err := filter.Evaluate(nil, units)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "expression is nil")
	})

	t.Run("empty units list", func(t *testing.T) {
		t.Parallel()

		expr := &filter.AttributeFilter{Key: "name", Value: "foo"}
		result, err := filter.Evaluate(expr, []filter.Unit{})

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("invalid glob pattern", func(t *testing.T) {
		t.Parallel()

		units := []filter.Unit{{Name: "app", Path: "./app"}}
		expr := &filter.PathFilter{Value: "[invalid-glob"}
		result, err := filter.Evaluate(expr, units)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to compile glob pattern")
	})
}
