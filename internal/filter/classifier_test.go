package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifier_NegatedGraphExpression_HasGraphFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		filterStr         string
		expectGraphFilter bool
		expectDependents  bool
	}{
		{
			name:              "negated dependent filter triggers HasGraphFilters and HasDependentFilters",
			filterStr:         "!...db",
			expectGraphFilter: true,
			expectDependents:  true,
		},
		{
			name:              "negated dependency filter triggers HasGraphFilters",
			filterStr:         "!db...",
			expectGraphFilter: true,
			expectDependents:  false,
		},
		{
			name:              "non-negated dependent filter",
			filterStr:         "...db",
			expectGraphFilter: true,
			expectDependents:  true,
		},
		{
			name:              "non-negated dependency filter",
			filterStr:         "db...",
			expectGraphFilter: true,
			expectDependents:  false,
		},
		{
			name:              "simple path filter",
			filterStr:         "./foo",
			expectGraphFilter: false,
			expectDependents:  false,
		},
		{
			name:              "negated path filter",
			filterStr:         "!./foo",
			expectGraphFilter: false,
			expectDependents:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := filter.Parse(tt.filterStr)
			require.NoError(t, err, "failed to parse filter")

			l := logger.CreateLogger()
			classifier := filter.NewClassifier(l)
			err = classifier.Analyze(filter.Filters{f})
			require.NoError(t, err, "failed to analyze filter")

			assert.Equal(t, tt.expectGraphFilter, classifier.HasGraphFilters(),
				"HasGraphFilters() mismatch for filter %q", tt.filterStr)
			assert.Equal(t, tt.expectDependents, classifier.HasDependentFilters(),
				"HasDependentFilters() mismatch for filter %q", tt.filterStr)
		})
	}
}

func TestClassifier_NegatedGraphExpression_IsNegatedFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		filterStr       string
		expectIsNegated []bool
	}{
		{
			name:            "single negated dependent filter",
			filterStr:       "!...db",
			expectIsNegated: []bool{true},
		},
		{
			name:            "single negated dependency filter",
			filterStr:       "!db...",
			expectIsNegated: []bool{true},
		},
		{
			name:            "non-negated dependent filter",
			filterStr:       "...db",
			expectIsNegated: []bool{false},
		},
		{
			name:            "non-negated dependency filter",
			filterStr:       "db...",
			expectIsNegated: []bool{false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := filter.Parse(tt.filterStr)
			require.NoError(t, err, "failed to parse filter")

			l := logger.CreateLogger()
			classifier := filter.NewClassifier(l)
			err = classifier.Analyze(filter.Filters{f})
			require.NoError(t, err, "failed to analyze filter")

			graphExprs := classifier.GraphExpressions()
			require.Len(t, graphExprs, len(tt.expectIsNegated),
				"unexpected number of graph expressions")

			for i, expected := range tt.expectIsNegated {
				assert.Equal(t, expected, graphExprs[i].IsNegated,
					"IsNegated mismatch for graph expression %d", i)
			}
		})
	}
}

func TestClassifier_MixedNegatedAndNonNegatedGraphFilters(t *testing.T) {
	t.Parallel()

	fooFilter, err := filter.Parse("...foo")
	require.NoError(t, err)

	barFilter, err := filter.Parse("!...bar")
	require.NoError(t, err)

	l := logger.CreateLogger()
	classifier := filter.NewClassifier(l)
	err = classifier.Analyze(filter.Filters{fooFilter, barFilter})
	require.NoError(t, err)

	assert.True(t, classifier.HasGraphFilters(), "should have graph filters")
	assert.True(t, classifier.HasDependentFilters(), "should have dependent filters")

	graphExprs := classifier.GraphExpressions()
	require.Len(t, graphExprs, 2, "should have 2 graph expressions")

	// First one is positive (...foo)
	assert.False(t, graphExprs[0].IsNegated, "first graph expression should not be negated")
	assert.Equal(t, 0, graphExprs[0].Index, "first graph expression should have index 0")
	assert.True(t, graphExprs[0].IncludeDependents, "first should include dependents")

	// Second one is negated (!...bar)
	assert.True(t, graphExprs[1].IsNegated, "second graph expression should be negated")
	assert.Equal(t, 1, graphExprs[1].Index, "second graph expression should have index 1")
	assert.True(t, graphExprs[1].IncludeDependents, "second should include dependents")
}

func TestClassifier_NestedNegatedGraphExpression(t *testing.T) {
	t.Parallel()

	target := filter.NewPathFilter("./db")
	graphExpr := filter.NewGraphExpression(target, false, true, false)
	negatedExpr := filter.NewPrefixExpression("!", graphExpr)

	f := filter.NewFilter(negatedExpr, "!./db...")

	l := logger.CreateLogger()
	classifier := filter.NewClassifier(l)
	err := classifier.Analyze(filter.Filters{f})
	require.NoError(t, err)

	assert.True(t, classifier.HasGraphFilters(), "should have graph filters")
	assert.False(t, classifier.HasDependentFilters(), "should not have dependent filters (db... is dependencies)")

	graphExprs := classifier.GraphExpressions()
	require.Len(t, graphExprs, 1)
	assert.True(t, graphExprs[0].IsNegated)
	assert.False(t, graphExprs[0].IncludeDependents)
	assert.True(t, graphExprs[0].IncludeDependencies)
}

func TestClassifier_NegatedBidirectionalGraphExpression(t *testing.T) {
	t.Parallel()

	target := filter.NewPathFilter("./db")
	graphExpr := filter.NewGraphExpression(target, true, true, false)
	negatedExpr := filter.NewPrefixExpression("!", graphExpr)

	f := filter.NewFilter(negatedExpr, "!...db...")

	l := logger.CreateLogger()
	classifier := filter.NewClassifier(l)
	err := classifier.Analyze(filter.Filters{f})
	require.NoError(t, err)

	assert.True(t, classifier.HasGraphFilters(), "should have graph filters")
	assert.True(t, classifier.HasDependentFilters(), "should have dependent filters")

	graphExprs := classifier.GraphExpressions()
	require.Len(t, graphExprs, 1)
	assert.True(t, graphExprs[0].IsNegated)
	assert.True(t, graphExprs[0].IncludeDependencies)
	assert.True(t, graphExprs[0].IncludeDependents)
}
