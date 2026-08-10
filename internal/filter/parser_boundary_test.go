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
				Target:     mustPath(t, "./envs/prod/vpc"),
				Dependents: filter.GraphBound{Include: true, Boundary: "./envs/prod"},
			},
		},
		{
			name:  "dependency boundary with braced path target",
			input: "{./envs/prod/edge}...(./envs/prod)",
			expected: &filter.GraphExpression{
				Target:       mustPath(t, "./envs/prod/edge"),
				Dependencies: filter.GraphBound{Include: true, Boundary: "./envs/prod"},
			},
		},
		{
			name:  "boundary in both directions",
			input: "(./a)...{./apps/foo}...(./b)",
			expected: &filter.GraphExpression{
				Target:       mustPath(t, "./apps/foo"),
				Dependents:   filter.GraphBound{Include: true, Boundary: "./a"},
				Dependencies: filter.GraphBound{Include: true, Boundary: "./b"},
			},
		},
		{
			name:  "dependent boundary with name target",
			input: "(./bound)...foo",
			expected: &filter.GraphExpression{
				Target:     mustAttr(t, "name", "foo"),
				Dependents: filter.GraphBound{Include: true, Boundary: "./bound"},
			},
		},
		{
			name:  "working directory boundary",
			input: "(.)...{./apps/foo}",
			expected: &filter.GraphExpression{
				Target:     mustPath(t, "./apps/foo"),
				Dependents: filter.GraphBound{Include: true, Boundary: "."},
			},
		},
		{
			// The dependent prefix is parsed before the caret, so the two
			// operands have to compose.
			name:  "boundary with excluded target",
			input: "(./a)...^{./apps/foo}",
			expected: &filter.GraphExpression{
				Target:        mustPath(t, "./apps/foo"),
				Dependents:    filter.GraphBound{Include: true, Boundary: "./a"},
				ExcludeTarget: true,
			},
		},
		{
			// A Git filter is a separate target arm from paths and names.
			name:  "boundary with git target",
			input: "(./a)...[main...HEAD]",
			expected: &filter.GraphExpression{
				Target:     filter.NewGitExpression("main", "HEAD"),
				Dependents: filter.GraphBound{Include: true, Boundary: "./a"},
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
			assert.Equal(t, tt.expected.Dependents, graphExpr.Dependents)
			assert.Equal(t, tt.expected.Dependencies, graphExpr.Dependencies)
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
		"(./a)...^{./apps/foo}",
		"(./a)...[main...HEAD]",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			first, err := filter.NewParser(filter.NewLexer(input)).ParseExpression()
			require.NoError(t, err)

			rendered := first.String()

			second, err := filter.NewParser(filter.NewLexer(rendered)).ParseExpression()
			require.NoError(t, err)

			assert.Equal(t, rendered, second.String(), "rendering must be idempotent")

			firstGraph, ok := first.(*filter.GraphExpression)
			require.True(t, ok)
			secondGraph, ok := second.(*filter.GraphExpression)
			require.True(t, ok)

			assert.Equal(t, firstGraph.Dependents, secondGraph.Dependents)
			assert.Equal(t, firstGraph.Dependencies, secondGraph.Dependencies)
			assert.Equal(t, firstGraph.ExcludeTarget, secondGraph.ExcludeTarget)
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
	assert.Equal(t, "./a", graphExprs[0].Dependents.Boundary)
	assert.Equal(t, "./b", graphExprs[0].Dependencies.Boundary)

	assert.True(t, graphExprs[1].IsNegated)
	assert.Equal(t, "./c", graphExprs[1].Dependents.Boundary)
	assert.Empty(t, graphExprs[1].Dependencies.Boundary)
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

func TestFilters_HasDependents(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		query    string
		expected bool
	}{
		{query: "...{./apps/foo}", expected: true},
		{query: "...{./apps/foo}...", expected: true},
		{query: "(./a)...{./apps/foo}", expected: true},
		{query: "{./apps/foo}...", expected: false},
		{query: "{./apps/foo}...(./a)", expected: false},
		{query: "{./apps/foo}", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()

			f, err := filter.Parse(tc.query)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, filter.Filters{f}.HasDependents())
		})
	}
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
		assert.True(t, graphExpr.Dependencies.Include)
		assert.Empty(t, graphExpr.Dependencies.Boundary)
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
		assert.True(t, graphExpr.Dependents.Include)
		assert.Equal(t, "./bound", graphExpr.Dependents.Boundary)
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
	// non-empty, and be closed. Each violation reports its own diagnostic, so
	// the title is asserted to keep the three paths from collapsing into one.
	tests := []struct {
		name  string
		input string
		title string
	}{
		{
			name:  "no ellipsis follows the boundary",
			input: "(./bound)",
			title: "Invalid boundary operand",
		},
		{
			name:  "empty boundary",
			input: "()...{./apps/foo}",
			title: "Empty boundary",
		},
		{
			name:  "unclosed boundary",
			input: "(./a...{./apps/foo}",
			title: "Unclosed boundary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := filter.NewParser(filter.NewLexer(tt.input)).ParseExpression()
			require.Error(t, err)

			var parseErr filter.ParseError
			require.ErrorAs(t, err, &parseErr)
			assert.Equal(t, tt.title, parseErr.Title)
		})
	}
}
