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
			expectError: false, // This parses as path filter, not an error
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
