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
