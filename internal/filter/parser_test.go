package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_SimpleExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected Expression
	}{
		{
			name:  "simple name filter",
			input: "foo",
			expected: &AttributeFilter{
				Key:   "name",
				Value: "foo",
			},
		},
		{
			name:  "attribute filter",
			input: "name=bar",
			expected: &AttributeFilter{
				Key:   "name",
				Value: "bar",
			},
		},
		{
			name:  "type attribute filter",
			input: "type=unit",
			expected: &AttributeFilter{
				Key:   "type",
				Value: "unit",
			},
		},
		{
			name:  "path filter relative",
			input: "./apps/foo",
			expected: &PathFilter{
				Value: "./apps/foo",
			},
		},
		{
			name:  "path filter absolute",
			input: "/absolute/path",
			expected: &PathFilter{
				Value: "/absolute/path",
			},
		},
		{
			name:  "path filter with wildcard",
			input: "./apps/*",
			expected: &PathFilter{
				Value: "./apps/*",
			},
		},
		{
			name:  "path filter with recursive wildcard",
			input: "./apps/**/foo",
			expected: &PathFilter{
				Value: "./apps/**/foo",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := NewLexer(tt.input)
			parser := NewParser(lexer)
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr)
		})
	}
}

func TestParser_PrefixExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected Expression
	}{
		{
			name:  "negated name filter",
			input: "!foo",
			expected: &PrefixExpression{
				Operator: "!",
				Right: &AttributeFilter{
					Key:   "name",
					Value: "foo",
				},
			},
		},
		{
			name:  "negated attribute filter",
			input: "!name=bar",
			expected: &PrefixExpression{
				Operator: "!",
				Right: &AttributeFilter{
					Key:   "name",
					Value: "bar",
				},
			},
		},
		{
			name:  "negated path filter",
			input: "!./apps/legacy",
			expected: &PrefixExpression{
				Operator: "!",
				Right: &PathFilter{
					Value: "./apps/legacy",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := NewLexer(tt.input)
			parser := NewParser(lexer)
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr)
		})
	}
}

func TestParser_InfixExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected Expression
	}{
		{
			name:  "union of two name filters",
			input: "foo | bar",
			expected: &InfixExpression{
				Left: &AttributeFilter{
					Key:   "name",
					Value: "foo",
				},
				Operator: "|",
				Right: &AttributeFilter{
					Key:   "name",
					Value: "bar",
				},
			},
		},
		{
			name:  "union of attribute filters",
			input: "name=foo | name=bar",
			expected: &InfixExpression{
				Left: &AttributeFilter{
					Key:   "name",
					Value: "foo",
				},
				Operator: "|",
				Right: &AttributeFilter{
					Key:   "name",
					Value: "bar",
				},
			},
		},
		{
			name:  "union of path and name filter",
			input: "./apps/* | name=bar",
			expected: &InfixExpression{
				Left: &PathFilter{
					Value: "./apps/*",
				},
				Operator: "|",
				Right: &AttributeFilter{
					Key:   "name",
					Value: "bar",
				},
			},
		},
		{
			name:  "union of three filters",
			input: "foo | bar | baz",
			expected: &InfixExpression{
				Left: &InfixExpression{
					Left: &AttributeFilter{
						Key:   "name",
						Value: "foo",
					},
					Operator: "|",
					Right: &AttributeFilter{
						Key:   "name",
						Value: "bar",
					},
				},
				Operator: "|",
				Right: &AttributeFilter{
					Key:   "name",
					Value: "baz",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := NewLexer(tt.input)
			parser := NewParser(lexer)
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr)
		})
	}
}

func TestParser_ComplexExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected Expression
	}{
		{
			name:  "negated filter in union",
			input: "!foo | bar",
			expected: &InfixExpression{
				Left: &PrefixExpression{
					Operator: "!",
					Right: &AttributeFilter{
						Key:   "name",
						Value: "foo",
					},
				},
				Operator: "|",
				Right: &AttributeFilter{
					Key:   "name",
					Value: "bar",
				},
			},
		},
		{
			name:  "union with negated second operand",
			input: "foo | !bar",
			expected: &InfixExpression{
				Left: &AttributeFilter{
					Key:   "name",
					Value: "foo",
				},
				Operator: "|",
				Right: &PrefixExpression{
					Operator: "!",
					Right: &AttributeFilter{
						Key:   "name",
						Value: "bar",
					},
				},
			},
		},
		{
			name:  "complex mix of paths and attributes",
			input: "./apps/* | !./legacy | name=foo",
			expected: &InfixExpression{
				Left: &InfixExpression{
					Left: &PathFilter{
						Value: "./apps/*",
					},
					Operator: "|",
					Right: &PrefixExpression{
						Operator: "!",
						Right: &PathFilter{
							Value: "./legacy",
						},
					},
				},
				Operator: "|",
				Right: &AttributeFilter{
					Key:   "name",
					Value: "foo",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := NewLexer(tt.input)
			parser := NewParser(lexer)
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr)
		})
	}
}

func TestParser_ErrorCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "empty input",
			input:       "",
			expectError: true,
		},
		{
			name:        "only operator",
			input:       "!",
			expectError: true,
		},
		{
			name:        "missing value after equal",
			input:       "name=",
			expectError: true,
		},
		{
			name:        "missing right side of union",
			input:       "foo |",
			expectError: true,
		},
		{
			name:        "invalid token",
			input:       "foo|",
			expectError: true,
		},
		{
			name:        "trailing pipe",
			input:       "foo | bar |",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := NewLexer(tt.input)
			parser := NewParser(lexer)
			expr, err := parser.ParseExpression()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, expr)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, expr)
			}
		})
	}
}

func TestParser_StringRepresentation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name filter",
			input:    "foo",
			expected: "name=foo",
		},
		{
			name:     "path filter",
			input:    "./apps/*",
			expected: "./apps/*",
		},
		{
			name:     "negated filter",
			input:    "!foo",
			expected: "!name=foo",
		},
		{
			name:     "union filter",
			input:    "foo | bar",
			expected: "name=foo | name=bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := NewLexer(tt.input)
			parser := NewParser(lexer)
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr.String())
		})
	}
}
