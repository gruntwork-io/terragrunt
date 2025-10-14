package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test data used across tests
var testUnits = []Unit{
	{Name: "app1", Path: "./apps/app1"},
	{Name: "app2", Path: "./apps/app2"},
	{Name: "legacy", Path: "./apps/legacy"},
	{Name: "db", Path: "./libs/db"},
	{Name: "api", Path: "./libs/api"},
	{Name: "web", Path: "./services/web"},
	{Name: "worker", Path: "./services/worker"},
}

func TestFilter_ParseAndEvaluate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		filterString  string
		expectedUnits []Unit
		expectError   bool
	}{
		{
			name:         "simple name filter",
			filterString: "app1",
			expectedUnits: []Unit{
				{Name: "app1", Path: "./apps/app1"},
			},
		},
		{
			name:         "attribute filter",
			filterString: "name=db",
			expectedUnits: []Unit{
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name:         "path filter with wildcard",
			filterString: "./apps/*",
			expectedUnits: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "legacy", Path: "./apps/legacy"},
			},
		},
		{
			name:         "negated filter",
			filterString: "!legacy",
			expectedUnits: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "db", Path: "./libs/db"},
				{Name: "api", Path: "./libs/api"},
				{Name: "web", Path: "./services/web"},
				{Name: "worker", Path: "./services/worker"},
			},
		},
		{
			name:         "union of two filters",
			filterString: "app1 | db",
			expectedUnits: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name:         "complex filter with path and attribute",
			filterString: "./apps/* | db",
			expectedUnits: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "app2", Path: "./apps/app2"},
				{Name: "legacy", Path: "./apps/legacy"},
				{Name: "db", Path: "./libs/db"},
			},
		},
		{
			name:         "multiple unions",
			filterString: "app1 | db | web",
			expectedUnits: []Unit{
				{Name: "app1", Path: "./apps/app1"},
				{Name: "db", Path: "./libs/db"},
				{Name: "web", Path: "./services/web"},
			},
		},
		{
			name:         "recursive wildcard",
			filterString: "./services/**",
			expectedUnits: []Unit{
				{Name: "web", Path: "./services/web"},
				{Name: "worker", Path: "./services/worker"},
			},
		},
		{
			name:          "parse error - empty",
			filterString:  "",
			expectedUnits: nil,
			expectError:   true,
		},
		{
			name:          "parse error - invalid syntax",
			filterString:  "foo |",
			expectedUnits: nil,
			expectError:   true,
		},
		{
			name:          "parse error - incomplete expression",
			filterString:  "name=",
			expectedUnits: nil,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter, err := Parse(tt.filterString)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, filter)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, filter)

			result, err := filter.Evaluate(testUnits)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedUnits, result)

			// Verify String() returns original query
			assert.Equal(t, tt.filterString, filter.String())
		})
	}
}

func TestFilter_Apply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		filterString  string
		units         []Unit
		expectedUnits []Unit
		expectError   bool
	}{
		{
			name:         "apply with simple filter",
			filterString: "app1",
			units:        testUnits,
			expectedUnits: []Unit{
				{Name: "app1", Path: "./apps/app1"},
			},
		},
		{
			name:         "apply with path filter",
			filterString: "./libs/*",
			units:        testUnits,
			expectedUnits: []Unit{
				{Name: "db", Path: "./libs/db"},
				{Name: "api", Path: "./libs/api"},
			},
		},
		{
			name:          "apply with empty units",
			filterString:  "anything",
			units:         []Unit{},
			expectedUnits: []Unit{},
		},
		{
			name:          "apply with parse error",
			filterString:  "!",
			units:         testUnits,
			expectedUnits: nil,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := Apply(tt.filterString, tt.units)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedUnits, result)
		})
	}
}

func TestFilter_Expression(t *testing.T) {
	t.Parallel()

	filterString := "name=foo"
	filter, err := Parse(filterString)
	require.NoError(t, err)

	expr := filter.Expression()
	assert.NotNil(t, expr)

	// Verify it's the correct type
	attrFilter, ok := expr.(*AttributeFilter)
	assert.True(t, ok)
	assert.Equal(t, "name", attrFilter.Key)
	assert.Equal(t, "foo", attrFilter.Value)
}

func TestFilter_RealWorldScenarios(t *testing.T) {
	t.Parallel()

	// Simulate a real-world repository structure
	repoUnits := []Unit{
		{Name: "vpc", Path: "./infrastructure/networking/vpc"},
		{Name: "subnets", Path: "./infrastructure/networking/subnets"},
		{Name: "security-groups", Path: "./infrastructure/networking/security-groups"},
		{Name: "app-server", Path: "./infrastructure/compute/app-server"},
		{Name: "db-server", Path: "./infrastructure/compute/db-server"},
		{Name: "frontend", Path: "./apps/frontend"},
		{Name: "backend", Path: "./apps/backend"},
		{Name: "api", Path: "./apps/api"},
		{Name: "test-app", Path: "./test/test-app"},
	}

	tests := []struct {
		name         string
		filterString string
		description  string
		expected     []string // Just unit names for simplicity
	}{
		{
			name:         "all networking infrastructure",
			filterString: "./infrastructure/networking/*",
			description:  "Select all networking-related units",
			expected:     []string{"vpc", "subnets", "security-groups"},
		},
		{
			name:         "apps and compute infrastructure",
			filterString: "./apps/* | ./infrastructure/compute/*",
			description:  "Select all apps and compute infrastructure",
			expected:     []string{"frontend", "backend", "api", "app-server", "db-server"},
		},
		{
			name:         "specific units by name",
			filterString: "vpc | app-server | frontend",
			description:  "Select specific units by their names",
			expected:     []string{"vpc", "app-server", "frontend"},
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

			result, err := Apply(tt.filterString, repoUnits)
			require.NoError(t, err)

			// Extract just the names for easier comparison
			var resultNames []string
			for _, unit := range result {
				resultNames = append(resultNames, unit.Name)
			}

			assert.ElementsMatch(t, tt.expected, resultNames, tt.description)
		})
	}
}

func TestFilter_EdgeCasesAndErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("filter with no matches", func(t *testing.T) {
		t.Parallel()

		result, err := Apply("nonexistent", testUnits)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("multiple parse and evaluate calls", func(t *testing.T) {
		t.Parallel()

		filter, err := Parse("app1")
		require.NoError(t, err)

		// Evaluate multiple times to ensure statelessness
		result1, err := filter.Evaluate(testUnits)
		require.NoError(t, err)

		result2, err := filter.Evaluate(testUnits)
		require.NoError(t, err)

		assert.Equal(t, result1, result2)
	})

	t.Run("whitespace handling", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			filterString string
		}{
			{"app1 |   app2"},
			{"  app1  |  app2  "},
			{"app1 | app2"},
		}

		expected := []Unit{
			{Name: "app1", Path: "./apps/app1"},
			{Name: "app2", Path: "./apps/app2"},
		}

		for _, tt := range tests {
			result, err := Apply(tt.filterString, testUnits)
			require.NoError(t, err)
			assert.ElementsMatch(t, expected, result)
		}
	})
}
