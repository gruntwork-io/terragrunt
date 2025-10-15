package filter_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discoveredconfig"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test data used across tests
var testConfigs = []*discoveredconfig.DiscoveredConfig{
	{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
	{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
	{Path: "./apps/legacy", Type: discoveredconfig.ConfigTypeUnit},
	{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
	{Path: "./libs/api", Type: discoveredconfig.ConfigTypeUnit},
	{Path: "./services/web", Type: discoveredconfig.ConfigTypeUnit},
	{Path: "./services/worker", Type: discoveredconfig.ConfigTypeUnit},
}

func TestFilter_ParseAndEvaluate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		filterString    string
		expectedConfigs []*discoveredconfig.DiscoveredConfig
		expectError     bool
	}{
		{
			name:         "simple name filter",
			filterString: "app1",
			expectedConfigs: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:         "attribute filter",
			filterString: "name=db",
			expectedConfigs: []*discoveredconfig.DiscoveredConfig{
				{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:         "path filter with wildcard",
			filterString: "./apps/*",
			expectedConfigs: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/legacy", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:         "negated filter",
			filterString: "!legacy",
			expectedConfigs: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./libs/api", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./services/web", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./services/worker", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:         "intersection of path and name",
			filterString: "./apps/* | app1",
			expectedConfigs: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:         "intersection with negation",
			filterString: "./apps/* | !legacy",
			expectedConfigs: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:         "chained intersections",
			filterString: "./apps/* | !legacy | app1",
			expectedConfigs: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:         "recursive wildcard",
			filterString: "./services/**",
			expectedConfigs: []*discoveredconfig.DiscoveredConfig{
				{Path: "./services/web", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./services/worker", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:            "parse error - empty",
			filterString:    "",
			expectedConfigs: nil,
			expectError:     true,
		},
		{
			name:            "parse error - invalid syntax",
			filterString:    "foo |",
			expectedConfigs: nil,
			expectError:     true,
		},
		{
			name:            "parse error - incomplete expression",
			filterString:    "name=",
			expectedConfigs: nil,
			expectError:     true,
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

			result, err := filter.Evaluate(testConfigs)
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.expectedConfigs, result)

			// Verify String() returns original query
			assert.Equal(t, tt.filterString, filter.String())
		})
	}
}

func TestFilter_Apply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		filterString    string
		configs         []*discoveredconfig.DiscoveredConfig
		expectedConfigs []*discoveredconfig.DiscoveredConfig
		expectError     bool
	}{
		{
			name:         "apply with simple filter",
			filterString: "app1",
			configs:      testConfigs,
			expectedConfigs: []*discoveredconfig.DiscoveredConfig{
				{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:         "apply with path filter",
			filterString: "./libs/*",
			configs:      testConfigs,
			expectedConfigs: []*discoveredconfig.DiscoveredConfig{
				{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
				{Path: "./libs/api", Type: discoveredconfig.ConfigTypeUnit},
			},
		},
		{
			name:            "apply with empty configs",
			filterString:    "anything",
			configs:         []*discoveredconfig.DiscoveredConfig{},
			expectedConfigs: []*discoveredconfig.DiscoveredConfig{},
		},
		{
			name:            "apply with parse error",
			filterString:    "!",
			configs:         testConfigs,
			expectedConfigs: nil,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := filter.Apply(tt.filterString, tt.configs)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, result)

				return
			}

			require.NoError(t, err)

			assert.ElementsMatch(t, tt.expectedConfigs, result)
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

	// Simulate a real-world repository structure
	repoConfigs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./infrastructure/networking/vpc", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./infrastructure/networking/subnets", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./infrastructure/networking/security-groups", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./infrastructure/compute/app-server", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./infrastructure/compute/db-server", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/frontend", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/backend", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/api", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./test/test-app", Type: discoveredconfig.ConfigTypeUnit},
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

			result, err := filter.Apply(tt.filterString, repoConfigs)
			require.NoError(t, err)

			// Extract just the names for easier comparison
			var resultNames []string
			for _, cfg := range result {
				resultNames = append(resultNames, filepath.Base(cfg.Path))
			}

			assert.ElementsMatch(t, tt.expected, resultNames, tt.description)
		})
	}
}

func TestFilter_EdgeCasesAndErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("filter with no matches", func(t *testing.T) {
		t.Parallel()

		result, err := filter.Apply("nonexistent", testConfigs)
		require.NoError(t, err)

		assert.Empty(t, result)
	})

	t.Run("multiple parse and evaluate calls", func(t *testing.T) {
		t.Parallel()

		filter, err := filter.Parse("app1")
		require.NoError(t, err)

		// Evaluate multiple times to ensure statelessness
		result1, err := filter.Evaluate(testConfigs)
		require.NoError(t, err)

		result2, err := filter.Evaluate(testConfigs)
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

		expected := []*discoveredconfig.DiscoveredConfig{
			{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
			{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
		}

		for _, tt := range tests {
			result, err := filter.Apply(tt.filterString, testConfigs)
			require.NoError(t, err)

			assert.ElementsMatch(t, expected, result)
		}
	})
}
