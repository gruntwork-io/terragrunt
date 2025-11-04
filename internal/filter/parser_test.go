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
				Target: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
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
				Target: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
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
				Target: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
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
				Target: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
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
				Target: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
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
				Target: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
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
				Target: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
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
				Target: &filter.PathFilter{
					Value:      "./apps/foo",
					WorkingDir: ".",
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
				Target: &filter.PathFilter{
					Value:      "./apps/foo",
					WorkingDir: ".",
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
				Target: &filter.AttributeFilter{
					Key:        "name",
					Value:      "bar",
					WorkingDir: ".",
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
				Target: &filter.PathFilter{
					Value:      "./apps/foo",
					WorkingDir: ".",
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
				Target: &filter.PathFilter{
					Value:      "./apps/foo",
					WorkingDir: ".",
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
				Target: &filter.PathFilter{
					Value:      "./apps/foo",
					WorkingDir: ".",
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
			parser := filter.NewParser(lexer, ".")
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
					Target: &filter.AttributeFilter{
						Key:        "name",
						Value:      "foo",
						WorkingDir: ".",
					},
					IncludeDependents:   true,
					IncludeDependencies: false,
					ExcludeTarget:       false,
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
			name:  "graph expression in union - right side",
			input: "foo | bar...",
			expected: &filter.InfixExpression{
				Left: &filter.AttributeFilter{
					Key:        "name",
					Value:      "foo",
					WorkingDir: ".",
				},
				Operator: "|",
				Right: &filter.GraphExpression{
					Target: &filter.AttributeFilter{
						Key:        "name",
						Value:      "bar",
						WorkingDir: ".",
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
					Target: &filter.AttributeFilter{
						Key:        "name",
						Value:      "foo",
						WorkingDir: ".",
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
					Right: &filter.AttributeFilter{
						Key:        "name",
						Value:      "foo",
						WorkingDir: ".",
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
			parser := filter.NewParser(lexer, ".")
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
			parser := filter.NewParser(lexer, ".")
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
			name:  "single Git reference",
			input: "[main]",
			expected: &filter.GitFilter{
				FromRef: "main",
				ToRef:   "",
			},
		},
		{
			name:  "two Git references with ellipsis",
			input: "[main...HEAD]",
			expected: &filter.GitFilter{
				FromRef: "main",
				ToRef:   "HEAD",
			},
		},
		{
			name:  "Git reference with branch name",
			input: "[feature-branch]",
			expected: &filter.GitFilter{
				FromRef: "feature-branch",
				ToRef:   "",
			},
		},
		{
			name:  "Git reference with commit SHA",
			input: "[abc123...def456]",
			expected: &filter.GitFilter{
				FromRef: "abc123",
				ToRef:   "def456",
			},
		},
		{
			name:  "Git reference with tag",
			input: "[v1.0.0...v2.0.0]",
			expected: &filter.GitFilter{
				FromRef: "v1.0.0",
				ToRef:   "v2.0.0",
			},
		},
		{
			name:  "Git reference with relative ref",
			input: "[HEAD~1...HEAD]",
			expected: &filter.GitFilter{
				FromRef: "HEAD~1",
				ToRef:   "HEAD",
			},
		},
		{
			name:  "Git reference with underscore in branch name",
			input: "[feature_branch]",
			expected: &filter.GitFilter{
				FromRef: "feature_branch",
				ToRef:   "",
			},
		},
		{
			name:  "Git reference with slash in branch name",
			input: "[feature/name]",
			expected: &filter.GitFilter{
				FromRef: "feature/name",
				ToRef:   "",
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
				Right: &filter.GitFilter{
					FromRef: "main",
					ToRef:   "HEAD",
				},
			},
		},
		{
			name:  "Git filter with path filter intersection",
			input: "[main...HEAD] | ./apps/*",
			expected: &filter.InfixExpression{
				Left: &filter.GitFilter{
					FromRef: "main",
					ToRef:   "HEAD",
				},
				Operator: "|",
				Right: &filter.PathFilter{
					Value:      "./apps/*",
					WorkingDir: ".",
				},
			},
		},
		{
			name:  "path filter with Git filter intersection",
			input: "./apps/* | [main...HEAD]",
			expected: &filter.InfixExpression{
				Left: &filter.PathFilter{
					Value:      "./apps/*",
					WorkingDir: ".",
				},
				Operator: "|",
				Right: &filter.GitFilter{
					FromRef: "main",
					ToRef:   "HEAD",
				},
			},
		},
		{
			name:  "Git filter with name filter intersection",
			input: "[main...HEAD] | name=app",
			expected: &filter.InfixExpression{
				Left: &filter.GitFilter{
					FromRef: "main",
					ToRef:   "HEAD",
				},
				Operator: "|",
				Right: &filter.AttributeFilter{
					Key:        "name",
					Value:      "app",
					WorkingDir: ".",
				},
			},
		},
		{
			name:  "Git filter with graph expression",
			input: "[main...HEAD] | app...",
			expected: &filter.InfixExpression{
				Left: &filter.GitFilter{
					FromRef: "main",
					ToRef:   "HEAD",
				},
				Operator: "|",
				Right: &filter.GraphExpression{
					Target: &filter.AttributeFilter{
						Key:        "name",
						Value:      "app",
						WorkingDir: ".",
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
			parser := filter.NewParser(lexer, ".")
			expr, err := parser.ParseExpression()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, expr)
		})
	}
}

func TestParser_GitFilterErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expectError bool
		name        string
		input       string
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
			parser := filter.NewParser(lexer, ".")
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
