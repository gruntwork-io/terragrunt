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
			expected: &filter.AttributeExpression{
				Key:   "name",
				Value: "foo",
			},
		},
		{
			name:  "attribute filter",
			input: "name=bar",
			expected: &filter.AttributeExpression{
				Key:   "name",
				Value: "bar",
			},
		},
		{
			name:  "type attribute filter",
			input: "type=unit",
			expected: &filter.AttributeExpression{
				Key:   "type",
				Value: "unit",
			},
		},
		{
			name:  "path filter relative",
			input: "./apps/foo",
			expected: &filter.PathExpression{
				Value: "./apps/foo",
			},
		},
		{
			name:  "path filter absolute",
			input: "/absolute/path",
			expected: &filter.PathExpression{
				Value: "/absolute/path",
			},
		},
		{
			name:  "path filter with wildcard",
			input: "./apps/*",
			expected: &filter.PathExpression{
				Value: "./apps/*",
			},
		},
		{
			name:  "path filter with recursive wildcard",
			input: "./apps/**/foo",
			expected: &filter.PathExpression{
				Value: "./apps/**/foo",
			},
		},
		{
			name:  "braced path filter",
			input: "{./apps/*}",
			expected: &filter.PathExpression{
				Value: "./apps/*",
			},
		},
		{
			name:  "braced path without prefix",
			input: "{apps}",
			expected: &filter.PathExpression{
				Value: "apps",
			},
		},
		{
			name:  "braced path with spaces",
			input: "{my path/file}",
			expected: &filter.PathExpression{
				Value: "my path/file",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
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
				Right: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
			},
		},
		{
			name:  "negated attribute filter",
			input: "!name=bar",
			expected: &filter.PrefixExpression{
				Operator: "!",
				Right: &filter.AttributeExpression{
					Key:   "name",
					Value: "bar",
				},
			},
		},
		{
			name:  "negated path filter",
			input: "!./apps/legacy",
			expected: &filter.PrefixExpression{
				Operator: "!",
				Right: &filter.PathExpression{
					Value: "./apps/legacy",
				},
			},
		},
		{
			name:  "negated braced path filter",
			input: "!{./apps/legacy}",
			expected: &filter.PrefixExpression{
				Operator: "!",
				Right: &filter.PathExpression{
					Value: "./apps/legacy",
				},
			},
		},
		{
			name:  "negated braced path filter with absolute path",
			input: "!{/absolute/path}",
			expected: &filter.PrefixExpression{
				Operator: "!",
				Right: &filter.PathExpression{
					Value: "/absolute/path",
				},
			},
		},
		{
			name:  "double negation collapses to positive",
			input: "!!foo",
			expected: &filter.AttributeExpression{
				Key:   "name",
				Value: "foo",
			},
		},
		{
			name:  "triple negation collapses to single negative",
			input: "!!!foo",
			expected: &filter.PrefixExpression{
				Operator: "!",
				Right: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
			},
		},
		{
			name:  "quadruple negation collapses to positive",
			input: "!!!!foo",
			expected: &filter.AttributeExpression{
				Key:   "name",
				Value: "foo",
			},
		},
		{
			name:  "double negation with attribute filter",
			input: "!!name=bar",
			expected: &filter.AttributeExpression{
				Key:   "name",
				Value: "bar",
			},
		},
		{
			name:  "double negation with path filter",
			input: "!!./apps/foo",
			expected: &filter.PathExpression{
				Value: "./apps/foo",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
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
				Left: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				Operator: "|",
				Right: &filter.AttributeExpression{
					Key:   "name",
					Value: "bar",
				},
			},
		},
		{
			name:  "union of attribute filters",
			input: "name=foo | name=bar",
			expected: &filter.InfixExpression{
				Left: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				Operator: "|",
				Right: &filter.AttributeExpression{
					Key:   "name",
					Value: "bar",
				},
			},
		},
		{
			name:  "union of path and name filter",
			input: "./apps/* | name=bar",
			expected: &filter.InfixExpression{
				Left: &filter.PathExpression{
					Value: "./apps/*",
				},
				Operator: "|",
				Right: &filter.AttributeExpression{
					Key:   "name",
					Value: "bar",
				},
			},
		},
		{
			name:  "union of three filters",
			input: "foo | bar | baz",
			expected: &filter.InfixExpression{
				Left: &filter.InfixExpression{
					Left: &filter.AttributeExpression{
						Key:   "name",
						Value: "foo",
					},
					Operator: "|",
					Right: &filter.AttributeExpression{
						Key:   "name",
						Value: "bar",
					},
				},
				Operator: "|",
				Right: &filter.AttributeExpression{
					Key:   "name",
					Value: "baz",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
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
					Right: &filter.AttributeExpression{
						Key:   "name",
						Value: "foo",
					},
				},
				Operator: "|",
				Right: &filter.AttributeExpression{
					Key:   "name",
					Value: "bar",
				},
			},
		},
		{
			name:  "union with negated second operand",
			input: "foo | !bar",
			expected: &filter.InfixExpression{
				Left: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				Operator: "|",
				Right: &filter.PrefixExpression{
					Operator: "!",
					Right: &filter.AttributeExpression{
						Key:   "name",
						Value: "bar",
					},
				},
			},
		},
		{
			name:  "complex mix of paths and attributes",
			input: "./apps/* | !./legacy | name=foo",
			expected: &filter.InfixExpression{
				Left: &filter.InfixExpression{
					Left: &filter.PathExpression{
						Value: "./apps/*",
					},
					Operator: "|",
					Right: &filter.PrefixExpression{
						Operator: "!",
						Right: &filter.PathExpression{
							Value: "./legacy",
						},
					},
				},
				Operator: "|",
				Right: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
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
			parser := filter.NewParser(lexer)
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
		{
			name:     "graph expression with dependents",
			input:    "...foo",
			expected: "...name=foo",
		},
		{
			name:     "graph expression with dependencies",
			input:    "foo...",
			expected: "name=foo...",
		},
		{
			name:     "graph expression with dependent depth",
			input:    "1...foo",
			expected: "1...name=foo",
		},
		{
			name:     "graph expression with dependency depth",
			input:    "foo...1",
			expected: "name=foo...1",
		},
		{
			name:     "graph expression with both depths",
			input:    "2...foo...3",
			expected: "2...name=foo...3",
		},
		{
			name:     "graph expression with caret",
			input:    "...^foo...",
			expected: "...^name=foo...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr.String())
		})
	}
}

func TestParser_GraphExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected filter.Expression
		name     string
		input    string
	}{
		{
			name:  "prefix ellipsis - dependents only",
			input: "...foo",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: false,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "postfix ellipsis - dependencies only",
			input: "foo...",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   false,
				IncludeDependencies: true,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "both prefix and postfix ellipsis",
			input: "...foo...",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: true,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "caret - exclude target only",
			input: "^foo",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   false,
				IncludeDependencies: false,
				ExcludeTarget:       true,
			},
		},
		{
			name:  "caret with prefix ellipsis",
			input: "...^foo",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: false,
				ExcludeTarget:       true,
			},
		},
		{
			name:  "caret with postfix ellipsis",
			input: "^foo...",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   false,
				IncludeDependencies: true,
				ExcludeTarget:       true,
			},
		},
		{
			name:  "caret with both ellipsis",
			input: "...^foo...",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: true,
				ExcludeTarget:       true,
			},
		},
		{
			name:  "graph expression with path filter",
			input: "...{./apps/foo}",
			expected: &filter.GraphExpression{
				Target: &filter.PathExpression{
					Value: "./apps/foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: false,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "graph expression with path filter and postfix ellipsis",
			input: "./apps/foo...",
			expected: &filter.GraphExpression{
				Target: &filter.PathExpression{
					Value: "./apps/foo",
				},
				IncludeDependents:   false,
				IncludeDependencies: true,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "graph expression with attribute filter",
			input: "...name=bar",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "bar",
				},
				IncludeDependents:   true,
				IncludeDependencies: false,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "graph expression with braced path and postfix ellipsis",
			input: "{./apps/foo}...",
			expected: &filter.GraphExpression{
				Target: &filter.PathExpression{
					Value: "./apps/foo",
				},
				IncludeDependents:   false,
				IncludeDependencies: true,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "graph expression with braced path and both ellipsis",
			input: "...{./apps/foo}...",
			expected: &filter.GraphExpression{
				Target: &filter.PathExpression{
					Value: "./apps/foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: true,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "graph expression with braced path, caret, and both ellipsis",
			input: "...^{./apps/foo}...",
			expected: &filter.GraphExpression{
				Target: &filter.PathExpression{
					Value: "./apps/foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: true,
				ExcludeTarget:       true,
			},
		},
		{
			name:  "depth-limited prefix - direct dependents only",
			input: "1...foo",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: false,
				ExcludeTarget:       false,
				DependentDepth:      1,
			},
		},
		{
			name:  "depth-limited postfix - direct dependencies only",
			input: "foo...1",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   false,
				IncludeDependencies: true,
				ExcludeTarget:       false,
				DependencyDepth:     1,
			},
		},
		{
			name:  "depth-limited both directions",
			input: "2...foo...3",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: true,
				ExcludeTarget:       false,
				DependentDepth:      2,
				DependencyDepth:     3,
			},
		},
		{
			name:  "depth-limited with caret",
			input: "1...^foo...2",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: true,
				ExcludeTarget:       true,
				DependentDepth:      1,
				DependencyDepth:     2,
			},
		},
		{
			name:  "depth-limited with multi-digit depth",
			input: "10...foo...25",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: true,
				ExcludeTarget:       false,
				DependentDepth:      10,
				DependencyDepth:     25,
			},
		},
		{
			name:  "very large depth clamps to max",
			input: "999999999...foo",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: false,
				ExcludeTarget:       false,
				DependentDepth:      filter.MaxTraversalDepth,
			},
		},
		{
			name:  "overflow depth falls back to unlimited",
			input: "99999999999999999999999...foo",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: false,
				ExcludeTarget:       false,
				DependentDepth:      0,
			},
		},
		// Numeric directory edge cases - testing disambiguation
		{
			name:  "numeric dir with depth - number before ellipsis is depth",
			input: "1...1",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "1",
				},
				IncludeDependents:   true,
				IncludeDependencies: false,
				ExcludeTarget:       false,
				DependentDepth:      1,
			},
		},
		{
			name:  "numeric dir escape hatch - braced path for target with dependency depth",
			input: "{1}...1",
			expected: &filter.GraphExpression{
				Target: &filter.PathExpression{
					Value: "1",
				},
				IncludeDependents:   false,
				IncludeDependencies: true,
				ExcludeTarget:       false,
				DependencyDepth:     1,
			},
		},
		{
			name:  "numeric dir escape hatch - braced path for target with dependent depth",
			input: "1...{1}",
			expected: &filter.GraphExpression{
				Target: &filter.PathExpression{
					Value: "1",
				},
				IncludeDependents:   true,
				IncludeDependencies: false,
				ExcludeTarget:       false,
				DependentDepth:      1,
			},
		},
		{
			name:  "numeric dir escape hatch - explicit name attribute with dependency depth",
			input: "name=1...1",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "1",
				},
				IncludeDependents:   false,
				IncludeDependencies: true,
				ExcludeTarget:       false,
				DependencyDepth:     1,
			},
		},
		{
			name:  "numeric dir escape hatch - explicit name attribute with dependent depth",
			input: "1...name=1",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "1",
				},
				IncludeDependents:   true,
				IncludeDependencies: false,
				ExcludeTarget:       false,
				DependentDepth:      1,
			},
		},
		{
			name:  "numeric dir full escape - both directions with braces",
			input: "1...{1}...1",
			expected: &filter.GraphExpression{
				Target: &filter.PathExpression{
					Value: "1",
				},
				IncludeDependents:   true,
				IncludeDependencies: true,
				ExcludeTarget:       false,
				DependentDepth:      1,
				DependencyDepth:     1,
			},
		},
		{
			name:  "alphanumeric dir not confused with depth",
			input: "1...1foo",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "1foo",
				},
				IncludeDependents:   true,
				IncludeDependencies: false,
				ExcludeTarget:       false,
				DependentDepth:      1,
			},
		},
		{
			name:  "alphanumeric dir not confused with depth - postfix",
			input: "foo1...1",
			expected: &filter.GraphExpression{
				Target: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo1",
				},
				IncludeDependents:   false,
				IncludeDependencies: true,
				ExcludeTarget:       false,
				DependencyDepth:     1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
			expr, err := parser.ParseExpression()

			require.NoError(t, err)

			graphExpr, ok := expr.(*filter.GraphExpression)
			require.True(t, ok, "Expected GraphExpression, got %T", expr)

			assert.Equal(t, tt.expected.(*filter.GraphExpression).IncludeDependents, graphExpr.IncludeDependents)
			assert.Equal(t, tt.expected.(*filter.GraphExpression).IncludeDependencies, graphExpr.IncludeDependencies)
			assert.Equal(t, tt.expected.(*filter.GraphExpression).ExcludeTarget, graphExpr.ExcludeTarget)
			assert.Equal(t, tt.expected.(*filter.GraphExpression).Target, graphExpr.Target)
			assert.Equal(t, tt.expected.(*filter.GraphExpression).DependentDepth, graphExpr.DependentDepth)
			assert.Equal(t, tt.expected.(*filter.GraphExpression).DependencyDepth, graphExpr.DependencyDepth)
		})
	}
}

func TestParser_GraphExpressionCombinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected filter.Expression
		name     string
		input    string
	}{
		{
			name:  "graph expression in union - left side",
			input: "...foo | bar",
			expected: &filter.InfixExpression{
				Left: &filter.GraphExpression{
					Target: &filter.AttributeExpression{
						Key:   "name",
						Value: "foo",
					},
					IncludeDependents:   true,
					IncludeDependencies: false,
					ExcludeTarget:       false,
				},
				Operator: "|",
				Right: &filter.AttributeExpression{
					Key:   "name",
					Value: "bar",
				},
			},
		},
		{
			name:  "graph expression in union - right side",
			input: "foo | bar...",
			expected: &filter.InfixExpression{
				Left: &filter.AttributeExpression{
					Key:   "name",
					Value: "foo",
				},
				Operator: "|",
				Right: &filter.GraphExpression{
					Target: &filter.AttributeExpression{
						Key:   "name",
						Value: "bar",
					},
					IncludeDependents:   false,
					IncludeDependencies: true,
					ExcludeTarget:       false,
				},
			},
		},
		{
			name:  "negated graph expression",
			input: "!...foo",
			expected: &filter.PrefixExpression{
				Operator: "!",
				Right: &filter.GraphExpression{
					Target: &filter.AttributeExpression{
						Key:   "name",
						Value: "foo",
					},
					IncludeDependents:   true,
					IncludeDependencies: false,
					ExcludeTarget:       false,
				},
			},
		},
		{
			name:  "graph expression with negation inside",
			input: "...!foo",
			expected: &filter.GraphExpression{
				Target: &filter.PrefixExpression{
					Operator: "!",
					Right: &filter.AttributeExpression{
						Key:   "name",
						Value: "foo",
					},
				},
				IncludeDependents:   true,
				IncludeDependencies: false,
				ExcludeTarget:       false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr)
		})
	}
}

func TestParser_GraphExpressionErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "ellipsis only",
			input:       "...",
			expectError: true,
		},
		{
			name:        "caret only",
			input:       "^",
			expectError: true,
		},
		{
			name:        "ellipsis followed by operator",
			input:       "... |",
			expectError: true,
		},
		{
			name:        "caret followed by operator",
			input:       "^ |",
			expectError: true,
		},
		{
			name:        "incomplete ellipsis",
			input:       "..foo",
			expectError: false, // This parses as name filter "..foo", not an error
		},
		{
			name:        "depth without target",
			input:       "1...",
			expectError: true,
		},
		{
			name:        "depth without target and trailing space",
			input:       "1... ",
			expectError: true,
		},
		{
			name:        "double depth no target",
			input:       "1......2",
			expectError: true, // 1... then ...2 with no target between
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
			expr, err := parser.ParseExpression()

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, expr)

				return
			}

			// For non-error cases, just verify it parses
			if err != nil {
				t.Logf("Unexpected error: %v", err)
			}
		})
	}
}

func TestParser_GitFilterExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected filter.Expression
		name     string
		input    string
	}{
		{
			name:     "single Git reference",
			input:    "[main]",
			expected: filter.NewGitExpression("main", "HEAD"),
		},
		{
			name:     "two Git references with ellipsis",
			input:    "[main...HEAD]",
			expected: filter.NewGitExpression("main", "HEAD"),
		},
		{
			name:     "Git reference with branch name",
			input:    "[feature-branch]",
			expected: filter.NewGitExpression("feature-branch", "HEAD"),
		},
		{
			name:     "Git reference with commit SHA",
			input:    "[abc123...def456]",
			expected: filter.NewGitExpression("abc123", "def456"),
		},
		{
			name:     "Git reference with tag",
			input:    "[v1.0.0...v2.0.0]",
			expected: filter.NewGitExpression("v1.0.0", "v2.0.0"),
		},
		{
			name:     "Git reference with relative ref",
			input:    "[HEAD~1...HEAD]",
			expected: filter.NewGitExpression("HEAD~1", "HEAD"),
		},
		{
			name:     "Git reference with underscore in branch name",
			input:    "[feature_branch]",
			expected: filter.NewGitExpression("feature_branch", "HEAD"),
		},
		{
			name:     "Git reference with slash in branch name",
			input:    "[feature/name]",
			expected: filter.NewGitExpression("feature/name", "HEAD"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr)
		})
	}
}

func TestParser_GitFilterWithOtherExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected filter.Expression
		name     string
		input    string
	}{
		{
			name:  "Git filter with negation",
			input: "![main...HEAD]",
			expected: &filter.PrefixExpression{
				Operator: "!",
				Right:    filter.NewGitExpression("main", "HEAD"),
			},
		},
		{
			name:  "Git filter with path filter intersection",
			input: "[main...HEAD] | ./apps/*",
			expected: &filter.InfixExpression{
				Left:     filter.NewGitExpression("main", "HEAD"),
				Operator: "|",
				Right: &filter.PathExpression{
					Value: "./apps/*",
				},
			},
		},
		{
			name:  "path filter with Git filter intersection",
			input: "./apps/* | [main...HEAD]",
			expected: &filter.InfixExpression{
				Left: &filter.PathExpression{
					Value: "./apps/*",
				},
				Operator: "|",
				Right:    filter.NewGitExpression("main", "HEAD"),
			},
		},
		{
			name:  "Git filter with name filter intersection",
			input: "[main...HEAD] | name=app",
			expected: &filter.InfixExpression{
				Left:     filter.NewGitExpression("main", "HEAD"),
				Operator: "|",
				Right: &filter.AttributeExpression{
					Key:   "name",
					Value: "app",
				},
			},
		},
		{
			name:  "Git filter with graph expression",
			input: "[main...HEAD] | app...",
			expected: &filter.InfixExpression{
				Left:     filter.NewGitExpression("main", "HEAD"),
				Operator: "|",
				Right: &filter.GraphExpression{
					Target: &filter.AttributeExpression{
						Key:   "name",
						Value: "app",
					},
					IncludeDependencies: true,
					IncludeDependents:   false,
					ExcludeTarget:       false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr)
		})
	}
}

func TestParser_GitFilterErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "empty Git filter",
			input:       "[]",
			expectError: true,
		},
		{
			name:        "unclosed Git filter",
			input:       "[main",
			expectError: true,
		},
		{
			name:        "Git filter with only ellipsis",
			input:       "[...]",
			expectError: true,
		},
		{
			name:        "Git filter with ellipsis but no second ref",
			input:       "[main...]",
			expectError: true,
		},
		{
			name:        "Git filter with only closing bracket",
			input:       "]",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
			expr, err := parser.ParseExpression()

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, expr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestParser_GitFilterAsGraphExpressionTarget tests parsing of combined git + graph expressions
// where a GitExpression is used as the target of a GraphExpression.
func TestParser_GitFilterAsGraphExpressionTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected filter.Expression
		name     string
		input    string
	}{
		{
			name:  "dependencies of git changes - postfix ellipsis",
			input: "[main...HEAD]...",
			expected: &filter.GraphExpression{
				Target:              filter.NewGitExpression("main", "HEAD"),
				IncludeDependencies: true,
				IncludeDependents:   false,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "dependents of git changes - prefix ellipsis",
			input: "...[main...HEAD]",
			expected: &filter.GraphExpression{
				Target:              filter.NewGitExpression("main", "HEAD"),
				IncludeDependencies: false,
				IncludeDependents:   true,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "both directions of git changes - issue #5307 pattern",
			input: "...[main...HEAD]...",
			expected: &filter.GraphExpression{
				Target:              filter.NewGitExpression("main", "HEAD"),
				IncludeDependencies: true,
				IncludeDependents:   true,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "exclude target with dependencies of git changes",
			input: "^[main...HEAD]...",
			expected: &filter.GraphExpression{
				Target:              filter.NewGitExpression("main", "HEAD"),
				IncludeDependencies: true,
				IncludeDependents:   false,
				ExcludeTarget:       true,
			},
		},
		{
			name:  "exclude target with dependents of git changes",
			input: "...^[main...HEAD]",
			expected: &filter.GraphExpression{
				Target:              filter.NewGitExpression("main", "HEAD"),
				IncludeDependencies: false,
				IncludeDependents:   true,
				ExcludeTarget:       true,
			},
		},
		{
			name:  "exclude target with both directions of git changes",
			input: "...^[main...HEAD]...",
			expected: &filter.GraphExpression{
				Target:              filter.NewGitExpression("main", "HEAD"),
				IncludeDependencies: true,
				IncludeDependents:   true,
				ExcludeTarget:       true,
			},
		},
		{
			name:  "single git ref with dependencies",
			input: "[main]...",
			expected: &filter.GraphExpression{
				Target:              filter.NewGitExpression("main", "HEAD"),
				IncludeDependencies: true,
				IncludeDependents:   false,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "single git ref with dependents",
			input: "...[main]",
			expected: &filter.GraphExpression{
				Target:              filter.NewGitExpression("main", "HEAD"),
				IncludeDependencies: false,
				IncludeDependents:   true,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "git ref with commit SHA and both directions",
			input: "...[abc123...def456]...",
			expected: &filter.GraphExpression{
				Target:              filter.NewGitExpression("abc123", "def456"),
				IncludeDependencies: true,
				IncludeDependents:   true,
				ExcludeTarget:       false,
			},
		},
		{
			name:  "git ref with relative ref (HEAD~1) and dependencies",
			input: "[HEAD~1...HEAD]...",
			expected: &filter.GraphExpression{
				Target:              filter.NewGitExpression("HEAD~1", "HEAD"),
				IncludeDependencies: true,
				IncludeDependents:   false,
				ExcludeTarget:       false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
			expr, err := parser.ParseExpression()

			require.NoError(t, err)

			graphExpr, ok := expr.(*filter.GraphExpression)
			require.True(t, ok, "Expected GraphExpression, got %T", expr)

			expectedGraph := tt.expected.(*filter.GraphExpression)
			assert.Equal(t, expectedGraph.IncludeDependents, graphExpr.IncludeDependents, "IncludeDependents mismatch")
			assert.Equal(t, expectedGraph.IncludeDependencies, graphExpr.IncludeDependencies, "IncludeDependencies mismatch")
			assert.Equal(t, expectedGraph.ExcludeTarget, graphExpr.ExcludeTarget, "ExcludeTarget mismatch")

			gitExpr, ok := graphExpr.Target.(*filter.GitExpression)
			require.True(t, ok, "Expected GitExpression as target, got %T", graphExpr.Target)

			expectedGit := expectedGraph.Target.(*filter.GitExpression)
			assert.Equal(t, expectedGit.FromRef, gitExpr.FromRef, "FromRef mismatch")
			assert.Equal(t, expectedGit.ToRef, gitExpr.ToRef, "ToRef mismatch")
		})
	}
}

// TestParser_GitFilterAsGraphExpressionTarget_StringRepresentation tests that
// combined git + graph expressions produce correct string representations.
func TestParser_GitFilterAsGraphExpressionTarget_StringRepresentation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "dependencies of git changes",
			input:    "[main...HEAD]...",
			expected: "[main...HEAD]...",
		},
		{
			name:     "dependents of git changes",
			input:    "...[main...HEAD]",
			expected: "...[main...HEAD]",
		},
		{
			name:     "both directions of git changes",
			input:    "...[main...HEAD]...",
			expected: "...[main...HEAD]...",
		},
		{
			name:     "exclude target with both directions",
			input:    "...^[main...HEAD]...",
			expected: "...^[main...HEAD]...",
		},
		{
			name:     "single ref defaults to HEAD",
			input:    "[main]...",
			expected: "[main...HEAD]...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
			parser := filter.NewParser(lexer)
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr.String())
		})
	}
}
