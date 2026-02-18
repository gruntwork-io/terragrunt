package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
)

func TestIsNegated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		exprFn   func(t *testing.T) filter.Expression
		name     string
		expected bool
	}{
		{
			name:     "path expression",
			exprFn:   func(t *testing.T) filter.Expression { return mustPath(t, "./foo") },
			expected: false,
		},
		{
			name: "negated path",
			exprFn: func(t *testing.T) filter.Expression {
				return filter.NewPrefixExpression("!", mustPath(t, "./foo"))
			},
			expected: true,
		},
		{
			name: "double negation",
			exprFn: func(t *testing.T) filter.Expression {
				return filter.NewPrefixExpression("!", filter.NewPrefixExpression("!", mustPath(t, "./foo")))
			},
			expected: true,
		},
		{
			name: "infix with negated left",
			exprFn: func(t *testing.T) filter.Expression {
				return filter.NewInfixExpression(
					filter.NewPrefixExpression("!", mustPath(t, "./foo")),
					"|",
					mustPath(t, "./bar"),
				)
			},
			expected: true,
		},
		{
			name: "infix with non-negated left",
			exprFn: func(t *testing.T) filter.Expression {
				return filter.NewInfixExpression(
					mustPath(t, "./foo"),
					"|",
					filter.NewPrefixExpression("!", mustPath(t, "./bar")),
				)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := filter.IsNegated(tt.exprFn(t))
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
