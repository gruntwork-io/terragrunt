package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
)

func TestRestrictToStacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expr     filter.Expression
		name     string
		expected bool
	}{
		{
			name:     "path filter",
			expr:     filter.NewPathFilter("./apps/*", "."),
			expected: false,
		},
		{
			name:     "attribute filter restricted to stacks",
			expr:     filter.NewAttributeFilter("type", "stack", "."),
			expected: true,
		},
		{
			name:     "attribute filter not restricted to stacks",
			expr:     filter.NewAttributeFilter("name", "foo", "."),
			expected: false,
		},
		{
			name:     "prefix expression restricted to stacks",
			expr:     filter.NewPrefixExpression("!", filter.NewAttributeFilter("type", "unit", ".")),
			expected: true,
		},
		{
			name:     "prefix expression not restricted to stacks",
			expr:     filter.NewPrefixExpression("!", filter.NewAttributeFilter("name", "foo", ".")),
			expected: false,
		},
		{
			name:     "infix expression restricted to stacks",
			expr:     filter.NewInfixExpression(filter.NewAttributeFilter("type", "stack", "."), "|", filter.NewAttributeFilter("external", "true", ".")),
			expected: true,
		},
		{
			name:     "infix expression also restricted to stacks",
			expr:     filter.NewInfixExpression(filter.NewAttributeFilter("external", "true", "."), "|", filter.NewAttributeFilter("type", "stack", ".")),
			expected: true,
		},
		{
			name:     "infix expression not restricted to stacks",
			expr:     filter.NewInfixExpression(filter.NewAttributeFilter("name", "foo", "."), "|", filter.NewAttributeFilter("external", "true", ".")),
			expected: false,
		},
		{
			name:     "graph expression",
			expr:     filter.NewGraphExpression(filter.NewAttributeFilter("name", "foo", "."), true, false, false),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.expr.IsRestrictedToStacks())
		})
	}
}
