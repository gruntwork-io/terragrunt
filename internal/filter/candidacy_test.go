package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeCandidacy_PathExpression(t *testing.T) {
	t.Parallel()

	expr := filter.NewPathFilter("./foo")
	info := filter.AnalyzeCandidacy(expr)

	assert.True(t, info.RequiresFilesystemOnly, "path expression should only require filesystem")
	assert.False(t, info.RequiresParsing, "path expression should not require parsing")
	assert.False(t, info.RequiresGraphDiscovery, "path expression should not require graph discovery")
	assert.False(t, info.IsNegated, "path expression should not be negated")
}

func TestAnalyzeCandidacy_AttributeExpression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		key                   string
		value                 string
		expectFilesystemOnly  bool
		expectRequiresParsing bool
	}{
		{
			name:                  "name attribute",
			key:                   filter.AttributeName,
			value:                 "my-app",
			expectFilesystemOnly:  true,
			expectRequiresParsing: false,
		},
		{
			name:                  "type attribute",
			key:                   filter.AttributeType,
			value:                 "unit",
			expectFilesystemOnly:  true,
			expectRequiresParsing: false,
		},
		{
			name:                  "external attribute",
			key:                   filter.AttributeExternal,
			value:                 "true",
			expectFilesystemOnly:  true,
			expectRequiresParsing: false,
		},
		{
			name:                  "reading attribute",
			key:                   filter.AttributeReading,
			value:                 "config/*",
			expectFilesystemOnly:  false,
			expectRequiresParsing: true,
		},
		{
			name:                  "source attribute",
			key:                   filter.AttributeSource,
			value:                 "git::https://example.com",
			expectFilesystemOnly:  false,
			expectRequiresParsing: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			expr := filter.NewAttributeExpression(tt.key, tt.value)
			info := filter.AnalyzeCandidacy(expr)

			assert.Equal(t, tt.expectFilesystemOnly, info.RequiresFilesystemOnly, "RequiresFilesystemOnly mismatch")
			assert.Equal(t, tt.expectRequiresParsing, info.RequiresParsing, "RequiresParsing mismatch")
			assert.False(t, info.RequiresGraphDiscovery, "attribute should not require graph discovery")
		})
	}
}

func TestAnalyzeCandidacy_GraphExpression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		includeDependencies bool
		includeDependents   bool
		excludeTarget       bool
		expectDirection     filter.GraphDirection
	}{
		{
			name:                "dependencies only",
			includeDependencies: true,
			includeDependents:   false,
			excludeTarget:       false,
			expectDirection:     filter.GraphDirectionDependencies,
		},
		{
			name:                "dependents only",
			includeDependencies: false,
			includeDependents:   true,
			excludeTarget:       false,
			expectDirection:     filter.GraphDirectionDependents,
		},
		{
			name:                "both directions",
			includeDependencies: true,
			includeDependents:   true,
			excludeTarget:       false,
			expectDirection:     filter.GraphDirectionBoth,
		},
		{
			name:                "exclude target",
			includeDependencies: true,
			includeDependents:   false,
			excludeTarget:       true,
			expectDirection:     filter.GraphDirectionDependencies,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			target := filter.NewPathFilter("./foo")
			expr := filter.NewGraphExpression(target, tt.includeDependents, tt.includeDependencies, tt.excludeTarget)
			info := filter.AnalyzeCandidacy(expr)

			assert.False(t, info.RequiresFilesystemOnly, "graph expression should not be filesystem only")
			assert.True(t, info.RequiresGraphDiscovery, "graph expression should require graph discovery")
			assert.Equal(t, tt.excludeTarget, info.ExcludeTarget, "ExcludeTarget mismatch")
			assert.Equal(t, tt.expectDirection, info.GraphDirection, "GraphDirection mismatch")
		})
	}
}

func TestAnalyzeCandidacy_NegatedExpression(t *testing.T) {
	t.Parallel()

	expr := filter.NewPrefixExpression("!", filter.NewPathFilter("./foo"))
	info := filter.AnalyzeCandidacy(expr)

	assert.True(t, info.IsNegated, "should be negated")
	assert.True(t, info.RequiresFilesystemOnly, "negated path should still only require filesystem")
}

func TestAnalyzeCandidacy_InfixExpression(t *testing.T) {
	t.Parallel()

	// Test: path | reading=config/*
	// Should require parsing because one side requires parsing
	left := filter.NewPathFilter("./foo")
	right := filter.NewAttributeExpression(filter.AttributeReading, "config/*")
	expr := filter.NewInfixExpression(left, "|", right)

	info := filter.AnalyzeCandidacy(expr)

	assert.False(t, info.RequiresFilesystemOnly, "infix with parsing requirement should not be filesystem only")
	assert.True(t, info.RequiresParsing, "should require parsing due to reading attribute")
}

func TestGetGraphTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expr          filter.Expression
		name          string
		expectTargets int
	}{
		{
			name:          "path expression - no targets",
			expr:          filter.NewPathFilter("./foo"),
			expectTargets: 0,
		},
		{
			name:          "single graph expression",
			expr:          filter.NewGraphExpression(filter.NewPathFilter("./foo"), false, true, false),
			expectTargets: 1,
		},
		{
			name: "multiple graph expressions via infix",
			expr: filter.NewInfixExpression(
				filter.NewGraphExpression(filter.NewPathFilter("./foo"), false, true, false),
				"|",
				filter.NewGraphExpression(filter.NewPathFilter("./bar"), true, false, false),
			),
			expectTargets: 2,
		},
		{
			name:          "negated graph expression",
			expr:          filter.NewPrefixExpression("!", filter.NewGraphExpression(filter.NewPathFilter("./foo"), false, true, false)),
			expectTargets: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			targets := filter.GetGraphTargets(tt.expr)
			assert.Len(t, targets, tt.expectTargets)
		})
	}
}

func TestIsNegated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expr     filter.Expression
		name     string
		expected bool
	}{
		{
			name:     "path expression",
			expr:     filter.NewPathFilter("./foo"),
			expected: false,
		},
		{
			name:     "negated path",
			expr:     filter.NewPrefixExpression("!", filter.NewPathFilter("./foo")),
			expected: true,
		},
		{
			name:     "double negation",
			expr:     filter.NewPrefixExpression("!", filter.NewPrefixExpression("!", filter.NewPathFilter("./foo"))),
			expected: true,
		},
		{
			name: "infix with negated left",
			expr: filter.NewInfixExpression(
				filter.NewPrefixExpression("!", filter.NewPathFilter("./foo")),
				"|",
				filter.NewPathFilter("./bar"),
			),
			expected: true,
		},
		{
			name: "infix with non-negated left",
			expr: filter.NewInfixExpression(
				filter.NewPathFilter("./foo"),
				"|",
				filter.NewPrefixExpression("!", filter.NewPathFilter("./bar")),
			),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := filter.IsNegated(tt.expr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetGraphExpressions(t *testing.T) {
	t.Parallel()

	target := filter.NewPathFilter("./foo")
	graphExpr := filter.NewGraphExpression(target, false, true, false)

	tests := []struct {
		expr        filter.Expression
		name        string
		expectCount int
	}{
		{
			name:        "path expression - none",
			expr:        filter.NewPathFilter("./foo"),
			expectCount: 0,
		},
		{
			name:        "single graph expression",
			expr:        graphExpr,
			expectCount: 1,
		},
		{
			name:        "nested in prefix",
			expr:        filter.NewPrefixExpression("!", graphExpr),
			expectCount: 1,
		},
		{
			name: "multiple in infix",
			expr: filter.NewInfixExpression(
				filter.NewGraphExpression(filter.NewPathFilter("./foo"), false, true, false),
				"|",
				filter.NewGraphExpression(filter.NewPathFilter("./bar"), true, false, false),
			),
			expectCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			graphExprs := filter.GetGraphExpressions(tt.expr)
			assert.Len(t, graphExprs, tt.expectCount)
		})
	}
}

func TestGraphDirection_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected  string
		direction filter.GraphDirection
	}{
		{"none", filter.GraphDirectionNone},
		{"dependencies", filter.GraphDirectionDependencies},
		{"dependents", filter.GraphDirectionDependents},
		{"both", filter.GraphDirectionBoth},
		{"unknown", filter.GraphDirection(999)},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.direction.String())
		})
	}
}

func TestAnalyzeCandidacy_ParseFilter(t *testing.T) {
	t.Parallel()

	// Test with real parsed filters
	tests := []struct {
		name                  string
		filterStr             string
		expectFilesystemOnly  bool
		expectRequiresParsing bool
		expectGraphDiscovery  bool
		expectNegated         bool
	}{
		{
			name:                  "simple path",
			filterStr:             "./foo",
			expectFilesystemOnly:  true,
			expectRequiresParsing: false,
			expectGraphDiscovery:  false,
			expectNegated:         false,
		},
		{
			name:                  "negated path",
			filterStr:             "!./foo",
			expectFilesystemOnly:  true,
			expectRequiresParsing: false,
			expectGraphDiscovery:  false,
			expectNegated:         true,
		},
		{
			name:                  "glob path",
			filterStr:             "./apps/**/*",
			expectFilesystemOnly:  true,
			expectRequiresParsing: false,
			expectGraphDiscovery:  false,
			expectNegated:         false,
		},
		{
			name:                  "dependency graph",
			filterStr:             "./foo...",
			expectFilesystemOnly:  false,
			expectRequiresParsing: false,
			expectGraphDiscovery:  true,
			expectNegated:         false,
		},
		{
			name:                  "dependent graph",
			filterStr:             "..../foo",
			expectFilesystemOnly:  false,
			expectRequiresParsing: false,
			expectGraphDiscovery:  true,
			expectNegated:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := filter.Parse(tt.filterStr)
			require.NoError(t, err, "failed to parse filter")

			info := filter.AnalyzeCandidacy(f.Expression())

			assert.Equal(t, tt.expectFilesystemOnly, info.RequiresFilesystemOnly, "RequiresFilesystemOnly mismatch")
			assert.Equal(t, tt.expectRequiresParsing, info.RequiresParsing, "RequiresParsing mismatch")
			assert.Equal(t, tt.expectGraphDiscovery, info.RequiresGraphDiscovery, "RequiresGraphDiscovery mismatch")
			assert.Equal(t, tt.expectNegated, info.IsNegated, "IsNegated mismatch")
		})
	}
}
