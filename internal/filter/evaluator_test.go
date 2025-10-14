package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluate_PathFilter(t *testing.T) {
	t.Parallel()

	units := []Unit{
		{Name: "app1", Path: "./apps/app1"},
		{Name: "app2", Path: "./apps/app2"},
		{Name: "legacy", Path: "./apps/legacy"},
		{Name: "db", Path: "./libs/db"},
		{Name: "api", Path: "./libs/api"},
		{Name: "nested", Path: "./apps/subdir/nested"},
	}

	tests := []struct {
		name     string
		filter   *PathFilter
		expected []Unit
	}{
		{
			name:   "exact path match",
			filter: &PathFilter{Value: "./apps/app1"},
			expected: []Unit{
				{Name: "app1", Path: "./apps/app1"},
			},
		},
		{
			name:   "glob with single wildcard",
			filter: &PathFilter{Value: "./apps/*"},
			expected: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "legacy", Path: "./apps/legacy"},
			},
		},
		{
			name:   "glob with recursive wildcard",
			filter: &PathFilter{Value: "./apps/**"},
			expected: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "legacy", Path: "./apps/legacy"},
				{Name: "nested", Path: "./apps/subdir/nested"},
			},
		},
		{
			name:   "glob matching specific subdirectory",
			filter: &PathFilter{Value: "./libs/*"},
			expected: []Unit{
				{Name: "db", Path: "./libs/db"},
				{Name: "api", Path: "./libs/api"},
			},
		},
		{
			name:     "no matches",
			filter:   &PathFilter{Value: "./nonexistent/*"},
			expected: []Unit{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := evaluatePathFilter(tt.filter, units)
			require.NoError(t, err)

			// Sort for consistent comparison
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter(t *testing.T) {
	t.Parallel()

	units := []Unit{
		{Name: "app", Path: "./apps/app"},
		{Name: "app", Path: "./libs/app"}, // Same name, different path
		{Name: "db", Path: "./libs/db"},
		{Name: "api", Path: "./libs/api"},
	}

	tests := []struct {
		name     string
		filter   *AttributeFilter
		expected []Unit
	}{
		{
			name:   "name filter single match",
			filter: &AttributeFilter{Key: "name", Value: "db"},
			expected: []Unit{
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name:   "name filter multiple matches",
			filter: &AttributeFilter{Key: "name", Value: "app"},
			expected: []Unit{
				{Name: "app", Path: "./apps/app"},
				{Name: "app", Path: "./libs/app"},
			},
		},
		{
			name:     "name filter no matches",
			filter:   &AttributeFilter{Key: "name", Value: "nonexistent"},
			expected: []Unit{},
		},
		{
			name:     "type filter",
			filter:   &AttributeFilter{Key: "type", Value: "unit"},
			expected: units, // All units match type=unit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := evaluateAttributeFilter(tt.filter, units)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter_InvalidKey(t *testing.T) {
	t.Parallel()

	units := []Unit{
		{Name: "app", Path: "./apps/app"},
	}

	filter := &AttributeFilter{Key: "invalid", Value: "foo"}
	result, err := evaluateAttributeFilter(filter, units)

	assert.Error(t, err)
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

	units := []Unit{
		{Name: "app1", Path: "./apps/app1"},
		{Name: "app2", Path: "./apps/app2"},
		{Name: "legacy", Path: "./apps/legacy"},
		{Name: "db", Path: "./libs/db"},
	}

	tests := []struct {
		name     string
		expr     *PrefixExpression
		expected []Unit
	}{
		{
			name: "exclude by name",
			expr: &PrefixExpression{
				Operator: "!",
				Right:    &AttributeFilter{Key: "name", Value: "legacy"},
			},
			expected: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name: "exclude by path",
			expr: &PrefixExpression{
				Operator: "!",
				Right:    &PathFilter{Value: "./apps/legacy"},
			},
			expected: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name: "exclude by glob",
			expr: &PrefixExpression{
				Operator: "!",
				Right:    &PathFilter{Value: "./apps/*"},
			},
			expected: []Unit{
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name: "exclude all (double negation effect)",
			expr: &PrefixExpression{
				Operator: "!",
				Right:    &AttributeFilter{Key: "type", Value: "unit"},
			},
			expected: []Unit{}, // All excluded
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := evaluatePrefixExpression(tt.expr, units)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_InfixExpression(t *testing.T) {
	t.Parallel()

	units := []Unit{
		{Name: "app1", Path: "./apps/app1"},
		{Name: "app2", Path: "./apps/app2"},
		{Name: "legacy", Path: "./apps/legacy"},
		{Name: "db", Path: "./libs/db"},
		{Name: "api", Path: "./libs/api"},
	}

	tests := []struct {
		name     string
		expr     *InfixExpression
		expected []Unit
	}{
		{
			name: "union of two names",
			expr: &InfixExpression{
				Left:     &AttributeFilter{Key: "name", Value: "app1"},
				Operator: "|",
				Right:    &AttributeFilter{Key: "name", Value: "db"},
			},
			expected: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name: "union of path and name",
			expr: &InfixExpression{
				Left:     &PathFilter{Value: "./apps/*"},
				Operator: "|",
				Right:    &AttributeFilter{Key: "name", Value: "db"},
			},
			expected: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "legacy", Path: "./apps/legacy"},
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name: "union with overlap (deduplication)",
			expr: &InfixExpression{
				Left:     &PathFilter{Value: "./apps/app1"},
				Operator: "|",
				Right:    &AttributeFilter{Key: "name", Value: "app1"},
			},
			expected: []Unit{
				{Name: "app1", Path: "./apps/app1"}, // Should appear only once
			},
		},
		{
			name: "union of empty results",
			expr: &InfixExpression{
				Left:     &AttributeFilter{Key: "name", Value: "nonexistent1"},
				Operator: "|",
				Right:    &AttributeFilter{Key: "name", Value: "nonexistent2"},
			},
			expected: []Unit{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := evaluateInfixExpression(tt.expr, units)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_ComplexExpressions(t *testing.T) {
	t.Parallel()

	units := []Unit{
		{Name: "app1", Path: "./apps/app1"},
		{Name: "app2", Path: "./apps/app2"},
		{Name: "legacy", Path: "./apps/legacy"},
		{Name: "db", Path: "./libs/db"},
		{Name: "api", Path: "./libs/api"},
		{Name: "special", Path: "./special/unit"},
	}

	tests := []struct {
		name     string
		expr     Expression
		expected []Unit
	}{
		{
			name: "union includes all from both sides",
			expr: &InfixExpression{
				Left:     &PathFilter{Value: "./apps/*"},
				Operator: "|",
				Right:    &PathFilter{Value: "./libs/*"},
			},
			expected: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "legacy", Path: "./apps/legacy"},
				{Name: "db", Path: "./libs/db"},
				{Name: "api", Path: "./libs/api"},
			},
		},
		{
			name: "negated union",
			expr: &PrefixExpression{
				Operator: "!",
				Right: &InfixExpression{
					Left:     &AttributeFilter{Key: "name", Value: "app1"},
					Operator: "|",
					Right:    &AttributeFilter{Key: "name", Value: "db"},
				},
			},
			expected: []Unit{
				{Name: "app2", Path: "./apps/app2"},
				{Name: "legacy", Path: "./apps/legacy"},
				{Name: "api", Path: "./libs/api"},
				{Name: "special", Path: "./special/unit"},
			},
		},
		{
			name: "multiple unions",
			expr: &InfixExpression{
				Left: &InfixExpression{
					Left:     &AttributeFilter{Key: "name", Value: "app1"},
					Operator: "|",
					Right:    &AttributeFilter{Key: "name", Value: "db"},
				},
				Operator: "|",
				Right:    &AttributeFilter{Key: "name", Value: "special"},
			},
			expected: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "db", Path: "./libs/db"},
				{Name: "special", Path: "./special/unit"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := Evaluate(tt.expr, units)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil expression", func(t *testing.T) {
		t.Parallel()

		units := []Unit{{Name: "app", Path: "./app"}}
		result, err := Evaluate(nil, units)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "expression is nil")
	})

	t.Run("empty units list", func(t *testing.T) {
		t.Parallel()

		expr := &AttributeFilter{Key: "name", Value: "foo"}
		result, err := Evaluate(expr, []Unit{})

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("invalid glob pattern", func(t *testing.T) {
		t.Parallel()

		units := []Unit{{Name: "app", Path: "./app"}}
		expr := &PathFilter{Value: "[invalid-glob"}
		result, err := Evaluate(expr, units)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to compile glob pattern")
	})
}

func TestUnionUnits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		left     []Unit
		right    []Unit
		expected []Unit
	}{
		{
			name:     "no overlap",
			left:     []Unit{{Name: "a", Path: "./a"}},
			right:    []Unit{{Name: "b", Path: "./b"}},
			expected: []Unit{{Name: "a", Path: "./a"}, {Name: "b", Path: "./b"}},
		},
		{
			name:     "complete overlap",
			left:     []Unit{{Name: "a", Path: "./a"}},
			right:    []Unit{{Name: "a", Path: "./a"}},
			expected: []Unit{{Name: "a", Path: "./a"}},
		},
		{
			name:     "partial overlap",
			left:     []Unit{{Name: "a", Path: "./a"}, {Name: "b", Path: "./b"}},
			right:    []Unit{{Name: "b", Path: "./b"}, {Name: "c", Path: "./c"}},
			expected: []Unit{{Name: "a", Path: "./a"}, {Name: "b", Path: "./b"}, {Name: "c", Path: "./c"}},
		},
		{
			name:     "empty left",
			left:     []Unit{},
			right:    []Unit{{Name: "a", Path: "./a"}},
			expected: []Unit{{Name: "a", Path: "./a"}},
		},
		{
			name:     "empty right",
			left:     []Unit{{Name: "a", Path: "./a"}},
			right:    []Unit{},
			expected: []Unit{{Name: "a", Path: "./a"}},
		},
		{
			name:     "both empty",
			left:     []Unit{},
			right:    []Unit{},
			expected: []Unit{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := unionUnits(tt.left, tt.right)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}
