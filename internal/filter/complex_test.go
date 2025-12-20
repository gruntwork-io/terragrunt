package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_ComplexDepthExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:     "depth with intersection",
			input:    "1...foo | bar",
			expected: "1...name=foo | name=bar",
		},
		{
			name:     "both sides have depth",
			input:    "foo...1 | bar...2",
			expected: "name=foo...1 | name=bar...2",
		},
		{
			name:     "full depth both sides of intersection",
			input:    "1...foo...1 | 2...bar...2",
			expected: "1...name=foo...1 | 2...name=bar...2",
		},
		{
			name:     "negation with depth prefix",
			input:    "!1...foo",
			expected: "!1...name=foo",
		},
		{
			name:     "negation with depth postfix",
			input:    "!foo...1",
			expected: "!name=foo...1",
		},
		{
			name:     "intersection with negation and depth",
			input:    "1...foo | !bar...2",
			expected: "1...name=foo | !name=bar...2",
		},
		{
			name:     "unlimited mixed with depth",
			input:    "...foo | bar...1",
			expected: "...name=foo | name=bar...1",
		},
		{
			name:     "chained intersections with depth",
			input:    "1...a | b...2 | 3...c",
			expected: "1...name=a | name=b...2 | 3...name=c",
		},
		{
			name:     "depth with path filter",
			input:    "1..../apps/*",
			expected: "1..../apps/*",
		},
		{
			name:     "depth with braced path",
			input:    "1...{my app}...2",
			expected: "1...my app...2",
		},
		{
			name:     "depth with attribute filter",
			input:    "1...type=unit...2",
			expected: "1...type=unit...2",
		},
		{
			name:     "depth with caret and intersection",
			input:    "1...^foo | bar...",
			expected: "1...^name=foo | name=bar...",
		},
		{
			name:     "parentheses treated as part of identifier",
			input:    "1...(foo | bar)",
			expected: "1...name=(foo | name=bar)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := NewLexer(tt.input)
			parser := NewParser(lexer)
			expr, err := parser.ParseExpression()

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr.String())
		})
	}
}
