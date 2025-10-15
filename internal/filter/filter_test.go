package filter_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testComponents = []*component.Component{
	{Path: "./apps/app1", Kind: component.Unit},
	{Path: "./apps/app2", Kind: component.Unit},
	{Path: "./apps/legacy", Kind: component.Unit},
	{Path: "./libs/db", Kind: component.Unit},
	{Path: "./libs/api", Kind: component.Unit},
	{Path: "./services/web", Kind: component.Unit},
	{Path: "./services/worker", Kind: component.Unit},
}

func TestFilter_ParseAndEvaluate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		filterString string
		expected     []*component.Component
		expectError  bool
	}{
		{
			name:         "simple name filter",
			filterString: "app1",
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
			},
		},
		{
			name:         "attribute filter",
			filterString: "name=db",
			expected: []*component.Component{
				{Path: "./libs/db", Kind: component.Unit},
			},
		},
		{
			name:         "path filter with wildcard",
			filterString: "./apps/*",
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
				{Path: "./apps/app2", Kind: component.Unit},
				{Path: "./apps/legacy", Kind: component.Unit},
			},
		},
		{
			name:         "negated filter",
			filterString: "!legacy",
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
				{Path: "./apps/app2", Kind: component.Unit},
				{Path: "./libs/db", Kind: component.Unit},
				{Path: "./libs/api", Kind: component.Unit},
				{Path: "./services/web", Kind: component.Unit},
				{Path: "./services/worker", Kind: component.Unit},
			},
		},
		{
			name:         "intersection of path and name",
			filterString: "./apps/* | app1",
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
			},
		},
		{
			name:         "intersection with negation",
			filterString: "./apps/* | !legacy",
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
				{Path: "./apps/app2", Kind: component.Unit},
			},
		},
		{
			name:         "chained intersections",
			filterString: "./apps/* | !legacy | app1",
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
			},
		},
		{
			name:         "recursive wildcard",
			filterString: "./services/**",
			expected: []*component.Component{
				{Path: "./services/web", Kind: component.Unit},
				{Path: "./services/worker", Kind: component.Unit},
			},
		},
		{
			name:         "parse error - empty",
			filterString: "",
			expected:     nil,
			expectError:  true,
		},
		{
			name:         "parse error - invalid syntax",
			filterString: "foo |",
			expected:     nil,
			expectError:  true,
		},
		{
			name:         "parse error - incomplete expression",
			filterString: "name=",
			expected:     nil,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter, err := filter.Parse(tt.filterString)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, filter)

				return
			}

			require.NoError(t, err)

			require.NotNil(t, filter)

			result, err := filter.Evaluate(testComponents)
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.expected, result)

			// Verify String() returns original query
			assert.Equal(t, tt.filterString, filter.String())
		})
	}
}

func TestFilter_Apply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		filterString string
		components   []*component.Component
		expected     []*component.Component
		expectError  bool
	}{
		{
			name:         "apply with simple filter",
			filterString: "app1",
			components:   testComponents,
			expected: []*component.Component{
				{Path: "./apps/app1", Kind: component.Unit},
			},
		},
		{
			name:         "apply with path filter",
			filterString: "./libs/*",
			components:   testComponents,
			expected: []*component.Component{
				{Path: "./libs/db", Kind: component.Unit},
				{Path: "./libs/api", Kind: component.Unit},
			},
		},
		{
			name:         "apply with empty components",
			filterString: "anything",
			components:   []*component.Component{},
			expected:     []*component.Component{},
		},
		{
			name:         "apply with parse error",
			filterString: "!",
			components:   testComponents,
			expected:     nil,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Apply(tt.filterString, tt.components)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, result)

				return
			}

			require.NoError(t, err)

			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestFilter_Expression(t *testing.T) {
	t.Parallel()

	filterString := "name=foo"
	f, err := filter.Parse(filterString)
	require.NoError(t, err)

	expr := f.Expression()
	assert.NotNil(t, expr)

	// Verify it's the correct type
	attrFilter, ok := expr.(*filter.AttributeFilter)
	assert.True(t, ok)
	assert.Equal(t, "name", attrFilter.Key)
	assert.Equal(t, "foo", attrFilter.Value)
}

func TestFilter_RealWorldScenarios(t *testing.T) {
	t.Parallel()

	repoComponents := []*component.Component{
		{Path: "./infrastructure/networking/vpc", Kind: component.Unit},
		{Path: "./infrastructure/networking/subnets", Kind: component.Unit},
		{Path: "./infrastructure/networking/security-groups", Kind: component.Unit},
		{Path: "./infrastructure/compute/app-server", Kind: component.Unit},
		{Path: "./infrastructure/compute/db-server", Kind: component.Unit},
		{Path: "./apps/frontend", Kind: component.Unit},
		{Path: "./apps/backend", Kind: component.Unit},
		{Path: "./apps/api", Kind: component.Unit},
		{Path: "./test/test-app", Kind: component.Unit},
	}

	tests := []struct {
		name         string
		filterString string
		description  string
		expected     []string
	}{
		{
			name:         "all networking infrastructure",
			filterString: "./infrastructure/networking/*",
			description:  "Select all networking-related units",
			expected:     []string{"vpc", "subnets", "security-groups"},
		},
		{
			name:         "apps excluding test-app",
			filterString: "./apps/* | !test-app",
			description:  "Select all apps except test-app",
			expected:     []string{"frontend", "backend", "api"},
		},
		{
			name:         "compute infrastructure excluding db-server",
			filterString: "./infrastructure/compute/* | !db-server",
			description:  "Select compute infrastructure except db-server",
			expected:     []string{"app-server"},
		},
		{
			name:         "everything in infrastructure",
			filterString: "./infrastructure/**",
			description:  "Select all infrastructure units recursively",
			expected:     []string{"vpc", "subnets", "security-groups", "app-server", "db-server"},
		},
		{
			name:         "exclude specific unit",
			filterString: "!test-app",
			description:  "Exclude test-app from all units",
			expected:     []string{"vpc", "subnets", "security-groups", "app-server", "db-server", "frontend", "backend", "api"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Apply(tt.filterString, repoComponents)
			require.NoError(t, err)

			var resultNames []string
			for _, c := range result {
				resultNames = append(resultNames, filepath.Base(c.Path))
			}

			assert.ElementsMatch(t, tt.expected, resultNames, tt.description)
		})
	}
}

func TestFilter_EdgeCasesAndErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("filter with no matches", func(t *testing.T) {
		t.Parallel()

		result, err := filter.Apply("nonexistent", testComponents)
		require.NoError(t, err)

		assert.Empty(t, result)
	})

	t.Run("multiple parse and evaluate calls", func(t *testing.T) {
		t.Parallel()

		filter, err := filter.Parse("app1")
		require.NoError(t, err)

		result1, err := filter.Evaluate(testComponents)
		require.NoError(t, err)

		result2, err := filter.Evaluate(testComponents)
		require.NoError(t, err)

		assert.Equal(t, result1, result2)
	})

	t.Run("whitespace handling", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			filterString string
		}{
			{"./apps/* |   !legacy"},
			{"  ./apps/*  |  !legacy  "},
			{"./apps/* | !legacy"},
		}

		expected := []*component.Component{
			{Path: "./apps/app1", Kind: component.Unit},
			{Path: "./apps/app2", Kind: component.Unit},
		}

		for _, tt := range tests {
			result, err := filter.Apply(tt.filterString, testComponents)
			require.NoError(t, err)

			assert.ElementsMatch(t, expected, result)
		}
	})
}
