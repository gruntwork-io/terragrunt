package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestrictToStacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		exprFn   func(t *testing.T) filter.Expression
		name     string
		expected bool
	}{
		{
			name: "path filter",
			exprFn: func(t *testing.T) filter.Expression {
				t.Helper()
				return mustPath(t, "./apps/*")
			},
			expected: false,
		},
		{
			name: "attribute filter restricted to stacks",
			exprFn: func(t *testing.T) filter.Expression {
				t.Helper()
				return mustAttr(t, "type", "stack")
			},
			expected: true,
		},
		{
			name: "attribute filter not restricted to stacks",
			exprFn: func(t *testing.T) filter.Expression {
				t.Helper()
				return mustAttr(t, "name", "foo")
			},
			expected: false,
		},
		{
			name: "prefix expression restricted to stacks",
			exprFn: func(t *testing.T) filter.Expression {
				t.Helper()
				return filter.NewPrefixExpression("!", mustAttr(t, "type", "unit"))
			},
			expected: true,
		},
		{
			name: "prefix expression not restricted to stacks",
			exprFn: func(t *testing.T) filter.Expression {
				t.Helper()
				return filter.NewPrefixExpression("!", mustAttr(t, "name", "foo"))
			},
			expected: false,
		},
		{
			name: "infix expression restricted to stacks",
			exprFn: func(t *testing.T) filter.Expression {
				t.Helper()
				return filter.NewInfixExpression(mustAttr(t, "type", "stack"), "|", mustAttr(t, "external", "true"))
			},
			expected: true,
		},
		{
			name: "infix expression also restricted to stacks",
			exprFn: func(t *testing.T) filter.Expression {
				t.Helper()
				return filter.NewInfixExpression(mustAttr(t, "external", "true"), "|", mustAttr(t, "type", "stack"))
			},
			expected: true,
		},
		{
			name: "infix expression not restricted to stacks",
			exprFn: func(t *testing.T) filter.Expression {
				t.Helper()
				return filter.NewInfixExpression(mustAttr(t, "name", "foo"), "|", mustAttr(t, "external", "true"))
			},
			expected: false,
		},
		{
			name: "graph expression",
			exprFn: func(t *testing.T) filter.Expression {
				t.Helper()
				return filter.NewGraphExpression(mustAttr(t, "name", "foo"), true, false, false)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			expr := tt.exprFn(t)
			assert.Equal(t, tt.expected, expr.IsRestrictedToStacks())
		})
	}
}

func mustPath(t *testing.T, value string) *filter.PathExpression {
	t.Helper()

	expr, err := filter.NewPathFilter(value)
	require.NoError(t, err)

	return expr
}

func mustAttr(t *testing.T, key, value string) *filter.AttributeExpression {
	t.Helper()

	expr, err := filter.NewAttributeExpression(key, value)
	require.NoError(t, err)

	return expr
}
