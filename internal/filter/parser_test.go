package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_SimpleExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected filter.Expression
		name     string
		input    string
	}{
		{
			name:  "simple name filter",
			input: "foo",
			expected: &filter.AttributeFilter{
				Key:        "name",
				Value:      "foo",
				WorkingDir: ".",
			},
		},
		{
			name:  "attribute filter",
			input: "name=bar",
			expected: &filter.AttributeFilter{
				Key:        "name",
				Value:      "bar",
				WorkingDir: ".",
			},
		},
		{
			name:  "type attribute filter",
			input: "type=unit",
			expected: &filter.AttributeFilter{
				Key:        "type",
				Value:      "unit",
				WorkingDir: ".",
			},
		},
		{
			name:  "path filter relative",
			input: "./apps/foo",
			expected: &filter.PathFilter{
				Value:      "./apps/foo",
				WorkingDir: ".",
			},
		},
		{
			name:  "path filter absolute",
			input: "/absolute/path",
			expected: &filter.PathFilter{
				Value:      "/absolute/path",
				WorkingDir: ".",
			},
		},
		{
			name:  "path filter with wildcard",
			input: "./apps/*",
			expected: &filter.PathFilter{
				Value:      "./apps/*",
				WorkingDir: ".",
			},
		},
		{
			name:  "path filter with recursive wildcard",
			input: "./apps/**/foo",
			expected: &filter.PathFilter{
				Value:      "./apps/**/foo",
				WorkingDir: ".",
			},
		},
		{
			name:  "braced path filter",
			input: "{./apps/*}",
			expected: &filter.PathFilter{
				Value:      "./apps/*",
				WorkingDir: ".",
			},
		},
		{
			name:  "braced path without prefix",
			input: "{apps}",
			expected: &filter.PathFilter{
				Value:      "apps",
				WorkingDir: ".",
			},
		},
		{
			name:  "braced path with spaces",
			input: "{my path/file}",
			expected: &filter.PathFilter{
				Value:      "my path/file",
				WorkingDir: ".",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer, ".")
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr)
		})
	}
}

func TestParser_PrefixExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected filter.Expression
		name     string
		input    string
	}{
		{
			name:  "negated name filter",
			input: "!foo",
			expected: &filter.PrefixExpression{
				Operator: "!",
				Right: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
				},
			},
		},
		{
			name:  "negated attribute filter",
			input: "!name=bar",
			expected: &filter.PrefixExpression{
				Operator: "!",
				Right: &filter.AttributeFilter{
					Key:        "name",
					Value:      "bar",
					WorkingDir: ".",
				},
			},
		},
		{
			name:  "negated path filter",
			input: "!./apps/legacy",
			expected: &filter.PrefixExpression{
				Operator: "!",
				Right: &filter.PathFilter{
					Value:      "./apps/legacy",
					WorkingDir: ".",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer, ".")
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr)
		})
	}
}

func TestParser_InfixExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected filter.Expression
		name     string
		input    string
	}{
		{
			name:  "union of two name filters",
			input: "foo | bar",
			expected: &filter.InfixExpression{
				Left: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
				},
				Operator: "|",
				Right: &filter.AttributeFilter{
					Key:        "name",
					Value:      "bar",
					WorkingDir: ".",
				},
			},
		},
		{
			name:  "union of attribute filters",
			input: "name=foo | name=bar",
			expected: &filter.InfixExpression{
				Left: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
				},
				Operator: "|",
				Right: &filter.AttributeFilter{
					Key:        "name",
					Value:      "bar",
					WorkingDir: ".",
				},
			},
		},
		{
			name:  "union of path and name filter",
			input: "./apps/* | name=bar",
			expected: &filter.InfixExpression{
				Left: &filter.PathFilter{
					Value:      "./apps/*",
					WorkingDir: ".",
				},
				Operator: "|",
				Right: &filter.AttributeFilter{
					Key:        "name",
					Value:      "bar",
					WorkingDir: ".",
				},
			},
		},
		{
			name:  "union of three filters",
			input: "foo | bar | baz",
			expected: &filter.InfixExpression{
				Left: &filter.InfixExpression{
					Left: &filter.AttributeFilter{
						Key:        "name",
						Value:      "foo",
						WorkingDir: ".",
					},
					Operator: "|",
					Right: &filter.AttributeFilter{
						Key:        "name",
						Value:      "bar",
						WorkingDir: ".",
					},
				},
				Operator: "|",
				Right: &filter.AttributeFilter{
					Key:        "name",
					Value:      "baz",
					WorkingDir: ".",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer, ".")
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr)
		})
	}
}

func TestParser_ComplexExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected filter.Expression
		name     string
		input    string
	}{
		{
			name:  "negated filter in union",
			input: "!foo | bar",
			expected: &filter.InfixExpression{
				Left: &filter.PrefixExpression{
					Operator: "!",
					Right: &filter.AttributeFilter{
						Key:        "name",
						Value:      "foo",
						WorkingDir: ".",
					},
				},
				Operator: "|",
				Right: &filter.AttributeFilter{
					Key:        "name",
					Value:      "bar",
					WorkingDir: ".",
				},
			},
		},
		{
			name:  "union with negated second operand",
			input: "foo | !bar",
			expected: &filter.InfixExpression{
				Left: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
				},
				Operator: "|",
				Right: &filter.PrefixExpression{
					Operator: "!",
					Right: &filter.AttributeFilter{
						Key:        "name",
						Value:      "bar",
						WorkingDir: ".",
					},
				},
			},
		},
		{
			name:  "complex mix of paths and attributes",
			input: "./apps/* | !./legacy | name=foo",
			expected: &filter.InfixExpression{
				Left: &filter.InfixExpression{
					Left: &filter.PathFilter{
						Value:      "./apps/*",
						WorkingDir: ".",
					},
					Operator: "|",
					Right: &filter.PrefixExpression{
						Operator: "!",
						Right: &filter.PathFilter{
							Value:      "./legacy",
							WorkingDir: ".",
						},
					},
				},
				Operator: "|",
				Right: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer, ".")
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

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer, ".")
			expr, err := parser.ParseExpression()

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, expr)
			} else {
				require.NoError(t, err)
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

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer, ".")
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr.String())
		})
	}
}
