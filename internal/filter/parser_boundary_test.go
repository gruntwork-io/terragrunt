package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_GraphBoundaryOperand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected *filter.GraphExpression
		name     string
		input    string
	}{
		{
			name:  "dependent boundary with braced path target",
			input: "(./envs/prod)...{./envs/prod/vpc}",
			expected: &filter.GraphExpression{
				Target:            mustPath(t, "./envs/prod/vpc"),
				IncludeDependents: true,
				DependentBoundary: "./envs/prod",
			},
		},
		{
			name:  "dependency boundary with braced path target",
			input: "{./envs/prod/edge}...(./envs/prod)",
			expected: &filter.GraphExpression{
				Target:              mustPath(t, "./envs/prod/edge"),
				IncludeDependencies: true,
				DependencyBoundary:  "./envs/prod",
			},
		},
		{
			name:  "boundary in both directions",
			input: "(./a)...{./apps/foo}...(./b)",
			expected: &filter.GraphExpression{
				Target:              mustPath(t, "./apps/foo"),
				IncludeDependents:   true,
				IncludeDependencies: true,
				DependentBoundary:   "./a",
				DependencyBoundary:  "./b",
			},
		},
		{
			name:  "dependent boundary with name target",
			input: "(./bound)...foo",
			expected: &filter.GraphExpression{
				Target:            mustAttr(t, "name", "foo"),
				IncludeDependents: true,
				DependentBoundary: "./bound",
			},
		},
		{
			name:  "working directory boundary",
			input: "(.)...{./apps/foo}",
			expected: &filter.GraphExpression{
				Target:            mustPath(t, "./apps/foo"),
				IncludeDependents: true,
				DependentBoundary: ".",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			expr, err := filter.NewParser(filter.NewLexer(tt.input)).ParseExpression()
			require.NoError(t, err)

			graphExpr, ok := expr.(*filter.GraphExpression)
			require.True(t, ok, "Expected GraphExpression, got %T", expr)

			assert.Equal(t, tt.expected.Target, graphExpr.Target)
			assert.Equal(t, tt.expected.IncludeDependents, graphExpr.IncludeDependents)
			assert.Equal(t, tt.expected.IncludeDependencies, graphExpr.IncludeDependencies)
			assert.Equal(t, tt.expected.DependentBoundary, graphExpr.DependentBoundary)
			assert.Equal(t, tt.expected.DependencyBoundary, graphExpr.DependencyBoundary)
			assert.Equal(t, tt.expected.DependentDepth, graphExpr.DependentDepth)
			assert.Equal(t, tt.expected.DependencyDepth, graphExpr.DependencyDepth)
		})
	}
}

// TestParser_GraphBoundaryRoundTrip verifies that String() renders the boundary
// operand so that re-parsing the rendered form yields the same boundaries.
func TestParser_GraphBoundaryRoundTrip(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"(./envs/prod)...{./envs/prod/vpc}",
		"{./envs/prod/edge}...(./envs/prod)",
		"(./a)...{./apps/foo}...(./b)",
		"(.)...{./apps/foo}",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			first, err := filter.NewParser(filter.NewLexer(input)).ParseExpression()
			require.NoError(t, err)

			rendered := first.String()

			second, err := filter.NewParser(filter.NewLexer(rendered)).ParseExpression()
			require.NoError(t, err)

			firstGraph, ok := first.(*filter.GraphExpression)
			require.True(t, ok)
			secondGraph, ok := second.(*filter.GraphExpression)
			require.True(t, ok)

			assert.Equal(t, firstGraph.DependentBoundary, secondGraph.DependentBoundary)
			assert.Equal(t, firstGraph.DependencyBoundary, secondGraph.DependencyBoundary)
		})
	}
}

// TestClassifier_PropagatesGraphBoundary verifies the boundary operands reach
// GraphExpressionInfo for both positive and negated graph expressions.
func TestClassifier_PropagatesGraphBoundary(t *testing.T) {
	t.Parallel()

	positive, err := filter.Parse("(./a)...{./apps/foo}...(./b)")
	require.NoError(t, err)

	negated, err := filter.Parse("!(./c)...{./apps/bar}")
	require.NoError(t, err)

	graphExprs := filter.NewClassifier(filter.Filters{positive, negated}).GraphExpressions()
	require.Len(t, graphExprs, 2)

	assert.False(t, graphExprs[0].IsNegated)
	assert.Equal(t, "./a", graphExprs[0].DependentBoundary)
	assert.Equal(t, "./b", graphExprs[0].DependencyBoundary)

	assert.True(t, graphExprs[1].IsNegated)
	assert.Equal(t, "./c", graphExprs[1].DependentBoundary)
	assert.Empty(t, graphExprs[1].DependencyBoundary)
}

func TestFilters_HasGraphBoundary(t *testing.T) {
	t.Parallel()

	withBoundary, err := filter.Parse("(./a)...{./apps/foo}")
	require.NoError(t, err)
	assert.True(t, filter.Filters{withBoundary}.HasGraphBoundary())

	without, err := filter.Parse("...{./apps/foo}")
	require.NoError(t, err)
	assert.False(t, filter.Filters{without}.HasGraphBoundary())
}

// TestParser_BracedPathWithParens verifies that a path whose name contains
// literal parentheses is disambiguated by wrapping it in braces, so the
// parens are kept as part of the path rather than parsed as a boundary.
func TestParser_BracedPathWithParens(t *testing.T) {
	t.Parallel()

	t.Run("braced path keeps literal parens", func(t *testing.T) {
		t.Parallel()

		expr, err := filter.NewParser(filter.NewLexer("{./weird(name)}")).ParseExpression()
		require.NoError(t, err)

		path, ok := expr.(*filter.PathExpression)
		require.True(t, ok, "Expected PathExpression, got %T", expr)
		assert.Equal(t, "./weird(name)", path.Value)
	})

	t.Run("braced parens path as graph target", func(t *testing.T) {
		t.Parallel()

		expr, err := filter.NewParser(filter.NewLexer("{./weird(name)}...")).ParseExpression()
		require.NoError(t, err)

		graphExpr, ok := expr.(*filter.GraphExpression)
		require.True(t, ok, "Expected GraphExpression, got %T", expr)
		assert.Equal(t, mustPath(t, "./weird(name)"), graphExpr.Target)
		assert.True(t, graphExpr.IncludeDependencies)
		assert.Empty(t, graphExpr.DependencyBoundary)
	})

	t.Run("parens boundary with braced parens target", func(t *testing.T) {
		t.Parallel()

		// The leading parens are a boundary delimiter; the braced parens are
		// kept as a literal path. They must not be conflated.
		expr, err := filter.NewParser(filter.NewLexer("(./bound)...{./weird(name)}")).ParseExpression()
		require.NoError(t, err)

		graphExpr, ok := expr.(*filter.GraphExpression)
		require.True(t, ok, "Expected GraphExpression, got %T", expr)
		assert.Equal(t, mustPath(t, "./weird(name)"), graphExpr.Target)
		assert.True(t, graphExpr.IncludeDependents)
		assert.Equal(t, "./bound", graphExpr.DependentBoundary)
	})

	t.Run("unbraced parens path is an error", func(t *testing.T) {
		t.Parallel()

		// Without braces the parens are read as delimiters, so a path with
		// literal parens must be braced to disambiguate it.
		_, err := filter.NewParser(filter.NewLexer("./weird(name)")).ParseExpression()
		require.Error(t, err)
	})
}

func TestParser_GraphBoundaryErrors(t *testing.T) {
	t.Parallel()

	// A boundary "(dir)" must sit in the operand slot adjacent to "...", be
	// non-empty, and be closed.
	inputs := []string{
		"(./bound)",           // no ellipsis follows the boundary
		"()...{./apps/foo}",   // empty boundary
		"(./a...{./apps/foo}", // unclosed boundary
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			_, err := filter.NewParser(filter.NewLexer(input)).ParseExpression()
			require.Error(t, err)
		})
	}
}
