package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
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

			classifier := filter.NewClassifier()
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

			classifier := filter.NewClassifier()
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

	classifier := filter.NewClassifier()
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

	classifier := filter.NewClassifier()
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

	classifier := filter.NewClassifier()
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

func TestClassifier_Classify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		componentPath      string
		componentRef       string
		filterStrs         []string
		expectedStatus     filter.ClassificationStatus
		expectedReason     filter.CandidacyReason
		expectedIdx        int
		parseDataAvailable bool
		expectIdxGteZero   bool
	}{
		{
			name:           "no_filters_returns_discovered",
			filterStrs:     nil,
			componentPath:  "./apps/app1",
			expectedStatus: filter.StatusDiscovered,
			expectedReason: filter.CandidacyReasonNone,
			expectedIdx:    -1,
		},
		{
			name:           "only_matches_negation_returns_excluded",
			filterStrs:     []string{"!./apps/app1"},
			componentPath:  "./apps/app1",
			expectedStatus: filter.StatusExcluded,
			expectedReason: filter.CandidacyReasonNone,
			expectedIdx:    -1,
		},
		{
			name:             "matches_negated_graph_expression_target_returns_candidate",
			filterStrs:       []string{"!...db"},
			componentPath:    "./libs/db",
			expectedStatus:   filter.StatusCandidate,
			expectedReason:   filter.CandidacyReasonGraphTarget,
			expectIdxGteZero: true,
		},
		{
			name:               "parse_expressions_without_parse_data_returns_candidate",
			filterStrs:         []string{"reading=config/*.hcl"},
			componentPath:      "./apps/app1",
			parseDataAvailable: false,
			expectedStatus:     filter.StatusCandidate,
			expectedReason:     filter.CandidacyReasonRequiresParse,
			expectedIdx:        -1,
		},
		{
			name:           "matches_filesystem_expression_returns_discovered",
			filterStrs:     []string{"./apps/*"},
			componentPath:  "./apps/app1",
			expectedStatus: filter.StatusDiscovered,
			expectedReason: filter.CandidacyReasonNone,
			expectedIdx:    -1,
		},
		{
			name:           "matches_git_expression_returns_discovered",
			filterStrs:     []string{"[main...feature]"},
			componentPath:  "./apps/app1",
			componentRef:   "main",
			expectedStatus: filter.StatusDiscovered,
			expectedReason: filter.CandidacyReasonNone,
			expectedIdx:    -1,
		},
		{
			name:           "matches_graph_expression_target_returns_candidate",
			filterStrs:     []string{"./libs/db..."},
			componentPath:  "./libs/db",
			expectedStatus: filter.StatusCandidate,
			expectedReason: filter.CandidacyReasonGraphTarget,
			expectedIdx:    0,
		},
		{
			name:               "dependent_filters_without_parse_data_returns_candidate",
			filterStrs:         []string{"...vpc"},
			componentPath:      "./apps/app1",
			parseDataAvailable: false,
			expectedStatus:     filter.StatusCandidate,
			expectedReason:     filter.CandidacyReasonPotentialDependent,
			expectedIdx:        -1,
		},
		{
			name:           "negation_exists_component_not_matching_returns_discovered",
			filterStrs:     []string{"!./libs/db"},
			componentPath:  "./apps/app1",
			expectedStatus: filter.StatusDiscovered,
			expectedReason: filter.CandidacyReasonNone,
			expectedIdx:    -1,
		},
		{
			name:           "positive_filters_no_match_returns_excluded",
			filterStrs:     []string{"./libs/*"},
			componentPath:  "./apps/app1",
			expectedStatus: filter.StatusExcluded,
			expectedReason: filter.CandidacyReasonNone,
			expectedIdx:    -1,
		},
		{
			name:           "positive_match_with_negation_returns_discovered",
			filterStrs:     []string{"!./apps/app1", "./apps/*"},
			componentPath:  "./apps/app1",
			expectedStatus: filter.StatusDiscovered,
			expectedReason: filter.CandidacyReasonNone,
			expectedIdx:    -1,
		},
		{
			name:           "multiple_graph_expressions_returns_correct_index",
			filterStrs:     []string{"./libs/api...", "./libs/db..."},
			componentPath:  "./libs/db",
			expectedStatus: filter.StatusCandidate,
			expectedReason: filter.CandidacyReasonGraphTarget,
			expectedIdx:    1,
		},
		{
			name:           "graph_expression_index_with_preceding_non_graph_filter",
			filterStrs:     []string{"./libs/api", "./libs/db..."},
			componentPath:  "./libs/db",
			expectedStatus: filter.StatusCandidate,
			expectedReason: filter.CandidacyReasonGraphTarget,
			expectedIdx:    1,
		},
		{
			name:           "graph_expression_index_with_multiple_preceding_non_graph_filters",
			filterStrs:     []string{"./libs/api", "!./libs/cache", "./libs/db..."},
			componentPath:  "./libs/db",
			expectedStatus: filter.StatusCandidate,
			expectedReason: filter.CandidacyReasonGraphTarget,
			expectedIdx:    2,
		},
		{
			name:               "parse_expressions_with_parse_data_evaluates_normally",
			filterStrs:         []string{"reading=config/*.hcl"},
			componentPath:      "./apps/app1",
			parseDataAvailable: true,
			expectedStatus:     filter.StatusExcluded,
			expectedReason:     filter.CandidacyReasonNone,
			expectedIdx:        -1,
		},
		{
			name:               "dependent_filters_with_parse_data_no_potential_dependent",
			filterStrs:         []string{"...vpc"},
			componentPath:      "./apps/app1",
			parseDataAvailable: true,
			expectedStatus:     filter.StatusExcluded,
			expectedReason:     filter.CandidacyReasonNone,
			expectedIdx:        -1,
		},
		{
			name:           "git_expression_component_without_ref_no_match",
			filterStrs:     []string{"[main...feature]"},
			componentPath:  "./apps/app1",
			expectedStatus: filter.StatusExcluded,
			expectedReason: filter.CandidacyReasonNone,
			expectedIdx:    -1,
		},
		{
			name:           "name_attribute_filter_matches",
			filterStrs:     []string{"name=app1"},
			componentPath:  "./apps/app1",
			expectedStatus: filter.StatusDiscovered,
			expectedReason: filter.CandidacyReasonNone,
			expectedIdx:    -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var filters filter.Filters

			for _, filterStr := range tt.filterStrs {
				f, err := filter.Parse(filterStr)
				require.NoError(t, err)

				filters = append(filters, f)
			}

			classifier := filter.NewClassifier()
			err := classifier.Analyze(filters)
			require.NoError(t, err)

			var comp component.Component
			if tt.componentRef != "" {
				comp = newTestComponentWithRef(tt.componentPath, tt.componentRef)
			} else {
				comp = newTestComponent(tt.componentPath)
			}

			status, reason, idx := classifier.Classify(logger.CreateLogger(), comp, filter.ClassificationContext{
				ParseDataAvailable: tt.parseDataAvailable,
			})

			assert.Equal(t, tt.expectedStatus, status)
			assert.Equal(t, tt.expectedReason, reason)

			if tt.expectIdxGteZero {
				assert.GreaterOrEqual(t, idx, 0)
			} else {
				assert.Equal(t, tt.expectedIdx, idx)
			}
		})
	}
}

func TestClassifier_Classify_StatusString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected string
		status   filter.ClassificationStatus
	}{
		{status: filter.StatusDiscovered, expected: "discovered"},
		{status: filter.StatusCandidate, expected: "candidate"},
		{status: filter.StatusExcluded, expected: "excluded"},
		{status: filter.ClassificationStatus(99), expected: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestClassifier_Classify_CandidacyReasonString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected string
		reason   filter.CandidacyReason
	}{
		{reason: filter.CandidacyReasonNone, expected: "none"},
		{reason: filter.CandidacyReasonGraphTarget, expected: "graph-target"},
		{reason: filter.CandidacyReasonRequiresParse, expected: "requires-parse"},
		{reason: filter.CandidacyReasonPotentialDependent, expected: "potential-dependent"},
		{reason: filter.CandidacyReason(99), expected: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.reason.String())
		})
	}
}

func newTestComponent(path string) component.Component {
	return component.NewUnit(path).WithDiscoveryContext(&component.DiscoveryContext{
		WorkingDir: ".",
	})
}

func newTestComponentWithRef(path, ref string) component.Component {
	return component.NewUnit(path).WithDiscoveryContext(&component.DiscoveryContext{
		WorkingDir: ".",
		Ref:        ref,
	})
}
