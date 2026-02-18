package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
)

func TestIsNegated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expr     filter.Expression
		name     string
		expected bool
	}{
		{
			name:     "path expression",
			expr:     filter.NewPathFilter("./foo"),
			expected: false,
		},
		{
			name:     "negated path",
			expr:     filter.NewPrefixExpression("!", filter.NewPathFilter("./foo")),
			expected: true,
		},
		{
			name:     "double negation",
			expr:     filter.NewPrefixExpression("!", filter.NewPrefixExpression("!", filter.NewPathFilter("./foo"))),
			expected: true,
		},
		{
			name: "infix with negated left",
			expr: filter.NewInfixExpression(
				filter.NewPrefixExpression("!", filter.NewPathFilter("./foo")),
				"|",
				filter.NewPathFilter("./bar"),
			),
			expected: true,
		},
		{
			name: "infix with non-negated left",
			expr: filter.NewInfixExpression(
				filter.NewPathFilter("./foo"),
				"|",
				filter.NewPrefixExpression("!", filter.NewPathFilter("./bar")),
			),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := filter.IsNegated(tt.expr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGraphDirection_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected  string
		direction filter.GraphDirection
	}{
		{"none", filter.GraphDirectionNone},
		{"dependencies", filter.GraphDirectionDependencies},
		{"dependents", filter.GraphDirectionDependents},
		{"both", filter.GraphDirectionBoth},
		{"unknown", filter.GraphDirection(999)},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.direction.String())
		})
	}
}
