package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discoveredconfig"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluate_PathFilter(t *testing.T) {
	t.Parallel()

	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/legacy", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/api", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/subdir/nested", Type: discoveredconfig.ConfigTypeUnit},
	}

	tests := []struct {
		name     string
		filter   *filter.PathFilter
		expected []*discoveredconfig.DiscoveredConfig
	}{
		{
			name:   "exact path match",
			filter: &filter.PathFilter{Value: "./apps/app1"},
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:   "glob with single wildcard",
			filter: &filter.PathFilter{Value: "./apps/*"},
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/legacy", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:   "glob with recursive wildcard",
			filter: &filter.PathFilter{Value: "./apps/**"},
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/legacy", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/subdir/nested", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:   "glob matching specific subdirectory",
			filter: &filter.PathFilter{Value: "./libs/*"},
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./libs/api", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:     "no matches",
			filter:   &filter.PathFilter{Value: "./nonexistent/*"},
			expected: []*discoveredconfig.DiscoveredConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.filter, configs)
			require.NoError(t, err)

			// Sort for consistent comparison
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter(t *testing.T) {
	t.Parallel()

	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./apps/app", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/app", Type: discoveredconfig.ConfigTypeUnit}, // Same name, different path
		{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/api", Type: discoveredconfig.ConfigTypeUnit},
	}

	tests := []struct {
		name     string
		filter   *filter.AttributeFilter
		expected []*discoveredconfig.DiscoveredConfig
	}{
		{
			name:   "name filter single match",
			filter: &filter.AttributeFilter{Key: "name", Value: "db"},
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:   "name filter multiple matches",
			filter: &filter.AttributeFilter{Key: "name", Value: "app"},
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./libs/app", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:     "name filter no matches",
			filter:   &filter.AttributeFilter{Key: "name", Value: "nonexistent"},
			expected: []*discoveredconfig.DiscoveredConfig{},
		},
		{
			name:     "type filter",
			filter:   &filter.AttributeFilter{Key: "type", Value: "unit"},
			expected: configs, // All configs match type=unit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.filter, configs)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter_InvalidKey(t *testing.T) {
	t.Parallel()

	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./apps/app", Type: discoveredconfig.ConfigTypeUnit},
	}

	attrFilter := &filter.AttributeFilter{Key: "invalid", Value: "foo"}
	result, err := filter.Evaluate(attrFilter, configs)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unknown attribute key")
}

func TestEvaluate_PrefixExpression(t *testing.T) {
	t.Parallel()

	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/legacy", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
	}

	tests := []struct {
		name     string
		expr     *filter.PrefixExpression
		expected []*discoveredconfig.DiscoveredConfig
	}{
		{
			name: "exclude by name",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.AttributeFilter{Key: "name", Value: "legacy"},
			},
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name: "exclude by path",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.PathFilter{Value: "./apps/legacy"},
			},
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name: "exclude by glob",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.PathFilter{Value: "./apps/*"},
			},
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name: "exclude all (double negation effect)",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.AttributeFilter{Key: "type", Value: "unit"},
			},
			expected: []*discoveredconfig.DiscoveredConfig{}, // All excluded
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.expr, configs)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_InfixExpression(t *testing.T) {
	t.Parallel()

	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/legacy", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/api", Type: discoveredconfig.ConfigTypeUnit},
	}

	tests := []struct {
		name     string
		expr     *filter.InfixExpression
		expected []*discoveredconfig.DiscoveredConfig
	}{
		{
			name: "intersection of path and name",
			expr: &filter.InfixExpression{
				Left:     &filter.PathFilter{Value: "./apps/*"},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "app1"},
			},
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name: "intersection with no overlap",
			expr: &filter.InfixExpression{
				Left:     &filter.PathFilter{Value: "./apps/*"},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "db"}, // db is in ./libs/, not ./apps/
			},
			expected: []*discoveredconfig.DiscoveredConfig{},
		},
		{
			name: "intersection of exact path and name",
			expr: &filter.InfixExpression{
				Left:     &filter.PathFilter{Value: "./apps/app1"},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "app1"},
			},
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name: "intersection of empty results",
			expr: &filter.InfixExpression{
				Left:     &filter.AttributeFilter{Key: "name", Value: "nonexistent1"},
				Operator: "|",
				Right:    &filter.AttributeFilter{Key: "name", Value: "app1"}, // Can't refine empty set
			},
			expected: []*discoveredconfig.DiscoveredConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.expr, configs)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_ComplexExpressions(t *testing.T) {
	t.Parallel()

	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/legacy", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/api", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./special/unit", Type: discoveredconfig.ConfigTypeUnit},
	}

	tests := []struct {
		name     string
		expr     filter.Expression
		expected []*discoveredconfig.DiscoveredConfig
	}{
		{
			name: "intersection with negation (refinement)",
			expr: &filter.InfixExpression{
				Left:     &filter.PathFilter{Value: "./apps/*"},
				Operator: "|",
				Right:    &filter.PrefixExpression{Operator: "!", Right: &filter.AttributeFilter{Key: "name", Value: "legacy"}},
			},
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
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
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/legacy", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./libs/api", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./special/unit", Type: discoveredconfig.ConfigTypeUnit},
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
			expected: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
				// Only app1 from ./apps/* after excluding legacy
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Evaluate(tt.expr, configs)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil expression", func(t *testing.T) {
		t.Parallel()

		configs := []*discoveredconfig.DiscoveredConfig{{Path: "./app", Type: discoveredconfig.ConfigTypeUnit}}
		result, err := filter.Evaluate(nil, configs)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "expression is nil")
	})

	t.Run("empty configs list", func(t *testing.T) {
		t.Parallel()

		expr := &filter.AttributeFilter{Key: "name", Value: "foo"}
		result, err := filter.Evaluate(expr, []*discoveredconfig.DiscoveredConfig{})

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("invalid glob pattern", func(t *testing.T) {
		t.Parallel()

		configs := []*discoveredconfig.DiscoveredConfig{{Path: "./app", Type: discoveredconfig.ConfigTypeUnit}}
		expr := &filter.PathFilter{Value: "[invalid-glob"}
		result, err := filter.Evaluate(expr, configs)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to compile glob pattern")
	})
}
