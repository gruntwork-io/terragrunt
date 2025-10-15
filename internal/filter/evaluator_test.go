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

func TestEvaluate_AttributeFilter_Reading(t *testing.T) {
	t.Parallel()

	components := []*component.Component{
		{Path: "./apps/app1", Kind: component.Unit, Reading: []string{"shared.hcl", "shared.tfvars"}},
		{Path: "./apps/app2", Kind: component.Unit, Reading: []string{"shared.hcl", "common/variables.hcl"}},
		{Path: "./apps/app3", Kind: component.Unit, Reading: []string{"config.yaml", "settings.json"}},
		{Path: "./libs/db", Kind: component.Unit, Reading: []string{"database.hcl"}},
		{Path: "./libs/api", Kind: component.Unit, Reading: []string{}},
		{Path: "./apps/app4", Kind: component.Unit, Reading: []string{"shared.hcl", "shared.tfvars", "extra.hcl"}},
	}

	tests := []struct {
		name     string
		filter   *filter.AttributeFilter
		expected []*component.Component
	}{
		{
			name:   "exact file path match - single match",
			filter: &filter.AttributeFilter{Key: "reading", Value: "database.hcl"},
			expected: []*component.Component{
				{Path: "./libs/db", Kind: component.Unit, Reading: []string{"database.hcl"}},
			},
		},
		{
			name:   "exact file path match - multiple matches",
			filter: &filter.AttributeFilter{Key: "reading", Value: "shared.hcl"},
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit, Reading: []string{"shared.hcl", "shared.tfvars"}},
				{Path: "./apps/app2", Kind: component.Unit, Reading: []string{"shared.hcl", "common/variables.hcl"}},
				{Path: "./apps/app4", Kind: component.Unit, Reading: []string{"shared.hcl", "shared.tfvars", "extra.hcl"}},
			},
		},
		{
			name:     "exact file path match - no matches",
			filter:   &filter.AttributeFilter{Key: "reading", Value: "nonexistent.hcl"},
			expected: []*component.Component{},
		},
		{
			name:   "glob pattern with single wildcard - *.hcl",
			filter: &filter.AttributeFilter{Key: "reading", Value: "*.hcl"},
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit, Reading: []string{"shared.hcl", "shared.tfvars"}},
				{Path: "./apps/app2", Kind: component.Unit, Reading: []string{"shared.hcl", "common/variables.hcl"}},
				{Path: "./libs/db", Kind: component.Unit, Reading: []string{"database.hcl"}},
				{Path: "./apps/app4", Kind: component.Unit, Reading: []string{"shared.hcl", "shared.tfvars", "extra.hcl"}},
			},
		},
		{
			name:   "glob pattern with prefix - shared*",
			filter: &filter.AttributeFilter{Key: "reading", Value: "shared*"},
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit, Reading: []string{"shared.hcl", "shared.tfvars"}},
				{Path: "./apps/app2", Kind: component.Unit, Reading: []string{"shared.hcl", "common/variables.hcl"}},
				{Path: "./apps/app4", Kind: component.Unit, Reading: []string{"shared.hcl", "shared.tfvars", "extra.hcl"}},
			},
		},
		{
			name:   "glob pattern with double wildcard - **/variables.hcl",
			filter: &filter.AttributeFilter{Key: "reading", Value: "**/variables.hcl"},
			expected: []*component.Component{
				{Path: "./apps/app2", Kind: component.Unit, Reading: []string{"shared.hcl", "common/variables.hcl"}},
			},
		},
		{
			name:     "empty Reading slice - no matches",
			filter:   &filter.AttributeFilter{Key: "reading", Value: "*.hcl"},
			expected: []*component.Component{},
		},
		{
			name:   "glob pattern with question mark - config.???l",
			filter: &filter.AttributeFilter{Key: "reading", Value: "config.???l"},
			expected: []*component.Component{
				{Path: "./apps/app3", Kind: component.Unit, Reading: []string{"config.yaml", "settings.json"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var testComponents []*component.Component
			if tt.name == "empty Reading slice - no matches" {
				testComponents = []*component.Component{
					{Path: "./libs/api", Kind: component.Unit, Reading: []string{}},
				}
			} else {
				testComponents = components
			}

			result, err := filter.Evaluate(tt.filter, testComponents)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter_Reading_ComponentAddedOnlyOnce(t *testing.T) {
	t.Parallel()

	components := []*component.Component{
		{Path: "./apps/app1", Kind: component.Unit, Reading: []string{"shared.hcl", "shared.tfvars", "shared.yaml"}},
	}

	// This glob should match multiple files in the Reading slice, but component should only be added once
	attrFilter := &filter.AttributeFilter{Key: "reading", Value: "shared*"}
	result, err := filter.Evaluate(attrFilter, components)
	require.NoError(t, err)

	// Should only have one component even though three files matched
	assert.Len(t, result, 1)
	assert.Equal(t, "./apps/app1", result[0].Path)
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
