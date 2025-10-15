package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluate_PathFilter(t *testing.T) {
	t.Parallel()

	components := []*component.Component{
		{Path: "./apps/app1", Kind: component.Unit},
		{Path: "./apps/app2", Kind: component.Unit},
		{Path: "./apps/legacy", Kind: component.Unit},
		{Path: "./libs/db", Kind: component.Unit},
		{Path: "./libs/api", Kind: component.Unit},
		{Path: "./apps/subdir/nested", Kind: component.Unit},
	}

	tests := []struct {
		name     string
		filter   *filter.PathFilter
		expected []*component.Component
	}{
		{
			name:   "exact path match",
			filter: &filter.PathFilter{Value: "./apps/app1"},
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
			},
		},
		{
			name:   "glob with single wildcard",
			filter: &filter.PathFilter{Value: "./apps/*"},
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
				{Path: "./apps/app2", Kind: component.Unit},
				{Path: "./apps/legacy", Kind: component.Unit},
			},
		},
		{
			name:   "glob with single wildcard and partial match",
			filter: &filter.PathFilter{Value: "./apps/app*"},
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
				{Path: "./apps/app2", Kind: component.Unit},
			},
		},
		{
			name:   "glob with recursive wildcard",
			filter: &filter.PathFilter{Value: "./apps/**"},
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
				{Path: "./apps/app2", Kind: component.Unit},
				{Path: "./apps/legacy", Kind: component.Unit},
				{Path: "./apps/subdir/nested", Kind: component.Unit},
			},
		},
		{
			name:     "no matches",
			filter:   &filter.PathFilter{Value: "./nonexistent/*"},
			expected: []*component.Component{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.filter, components)
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter(t *testing.T) {
	t.Parallel()

	components := []*component.Component{
		{Path: "./apps/app", Kind: component.Unit},
		{Path: "./libs/app", Kind: component.Unit},
		{Path: "./libs/db", Kind: component.Unit},
		{Path: "./libs/api", Kind: component.Unit},
		{Path: "./libs/api", Kind: component.Stack},
	}

	tests := []struct {
		name     string
		filter   *filter.AttributeFilter
		expected []*component.Component
	}{
		{
			name:   "name filter single match",
			filter: &filter.AttributeFilter{Key: "name", Value: "db"},
			expected: []*component.Component{
				{Path: "./libs/db", Kind: component.Unit},
			},
		},
		{
			name:   "name filter multiple matches",
			filter: &filter.AttributeFilter{Key: "name", Value: "app"},
			expected: []*component.Component{
				{Path: "./apps/app", Kind: component.Unit},
				{Path: "./libs/app", Kind: component.Unit},
			},
		},
		{
			name:     "name filter no matches",
			filter:   &filter.AttributeFilter{Key: "name", Value: "nonexistent"},
			expected: []*component.Component{},
		},
		{
			name:   "type filter unit",
			filter: &filter.AttributeFilter{Key: "type", Value: "unit"},
			expected: []*component.Component{
				{Path: "./apps/app", Kind: component.Unit},
				{Path: "./libs/app", Kind: component.Unit},
				{Path: "./libs/db", Kind: component.Unit},
				{Path: "./libs/api", Kind: component.Unit},
			},
		},
		{
			name:   "type filter stack",
			filter: &filter.AttributeFilter{Key: "type", Value: "stack"},
			expected: []*component.Component{
				{Path: "./libs/api", Kind: component.Stack},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.filter, components)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter_InvalidKey(t *testing.T) {
	t.Parallel()

	components := []*component.Component{
		{Path: "./apps/app", Kind: component.Unit},
	}

	attrFilter := &filter.AttributeFilter{Key: "invalid", Value: "foo"}
	result, err := filter.Evaluate(attrFilter, components)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unknown attribute key")
}

func TestEvaluate_PrefixExpression(t *testing.T) {
	t.Parallel()

	components := []*component.Component{
		{Path: "./apps/app1", Kind: component.Unit},
		{Path: "./apps/app2", Kind: component.Unit},
		{Path: "./apps/legacy", Kind: component.Unit},
		{Path: "./libs/db", Kind: component.Unit},
	}

	tests := []struct {
		name     string
		expr     *filter.PrefixExpression
		expected []*component.Component
	}{
		{
			name: "exclude by name",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.AttributeFilter{Key: "name", Value: "legacy"},
			},
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
				{Path: "./apps/app2", Kind: component.Unit},
				{Path: "./libs/db", Kind: component.Unit},
			},
		},
		{
			name: "exclude by path",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.PathFilter{Value: "./apps/legacy"},
			},
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
				{Path: "./apps/app2", Kind: component.Unit},
				{Path: "./libs/db", Kind: component.Unit},
			},
		},
		{
			name: "exclude by glob",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.PathFilter{Value: "./apps/*"},
			},
			expected: []*component.Component{
				{Path: "./libs/db", Kind: component.Unit},
			},
		},
		{
			name: "exclude all (double negation effect)",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.AttributeFilter{Key: "type", Value: "unit"},
			},
			expected: []*component.Component{},
		},
		{
			name: "exclude nothing",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.AttributeFilter{Key: "name", Value: "nonexistent"},
			},
			expected: components,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.expr, components)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_InfixExpression(t *testing.T) {
	t.Parallel()

	components := []*component.Component{
		{Path: "./apps/app1", Kind: component.Unit},
		{Path: "./apps/app2", Kind: component.Unit},
		{Path: "./apps/legacy", Kind: component.Unit},
		{Path: "./libs/db", Kind: component.Unit},
		{Path: "./libs/api", Kind: component.Unit},
	}

	tests := []struct {
		name     string
		expr     *filter.InfixExpression
		expected []*component.Component
	}{
		{
			name: "intersection of path and name",
			expr: &filter.InfixExpression{
				Left:     &filter.PathFilter{Value: "./apps/*"},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "app1"},
			},
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
			},
		},
		{
			name: "intersection with no overlap",
			expr: &filter.InfixExpression{
				Left:     &filter.PathFilter{Value: "./apps/*"},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "db"},
			},
			expected: []*component.Component{},
		},
		{
			name: "intersection of exact path and name",
			expr: &filter.InfixExpression{
				Left:     &filter.PathFilter{Value: "./apps/app1"},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "app1"},
			},
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
			},
		},
		{
			name: "intersection of empty results",
			expr: &filter.InfixExpression{
				Left:     &filter.AttributeFilter{Key: "name", Value: "nonexistent1"},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "app1"},
			},
			expected: []*component.Component{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.expr, components)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_ComplexExpressions(t *testing.T) {
	t.Parallel()

	components := []*component.Component{
		{Path: "./apps/app1", Kind: component.Unit},
		{Path: "./apps/app2", Kind: component.Unit},
		{Path: "./apps/legacy", Kind: component.Unit},
		{Path: "./libs/db", Kind: component.Unit},
		{Path: "./libs/api", Kind: component.Unit},
		{Path: "./special/unit", Kind: component.Unit},
	}

	tests := []struct {
		name     string
		expr     filter.Expression
		expected []*component.Component
	}{
		{
			name: "intersection with negation (refinement)",
			expr: &filter.InfixExpression{
				Left:     &filter.PathFilter{Value: "./apps/*"},
				Operator: "|",
				Right: &filter.PrefixExpression{
					Operator: "!",
					Right:    &filter.AttributeFilter{Key: "name", Value: "legacy"},
				},
			},
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
				{Path: "./apps/app2", Kind: component.Unit},
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
			expected: []*component.Component{
				{Path: "./apps/app2", Kind: component.Unit},
				{Path: "./apps/legacy", Kind: component.Unit},
				{Path: "./libs/db", Kind: component.Unit},
				{Path: "./libs/api", Kind: component.Unit},
				{Path: "./special/unit", Kind: component.Unit},
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
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.expr, components)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil expression", func(t *testing.T) {
		t.Parallel()

		components := []*component.Component{{Path: "./app", Kind: component.Unit}}
		result, err := filter.Evaluate(nil, components)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "expression is nil")
	})

	t.Run("empty components list", func(t *testing.T) {
		t.Parallel()

		expr := &filter.AttributeFilter{Key: "name", Value: "foo"}
		result, err := filter.Evaluate(expr, []*component.Component{})

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("invalid glob pattern", func(t *testing.T) {
		t.Parallel()

		components := []*component.Component{{Path: "./app", Kind: component.Unit}}
		expr := &filter.PathFilter{Value: "[invalid-glob"}
		result, err := filter.Evaluate(expr, components)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to compile glob pattern")
	})
}
