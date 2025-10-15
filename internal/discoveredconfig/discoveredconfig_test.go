package discoveredconfig_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discoveredconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoveredConfigsSort(t *testing.T) {
	t.Parallel()

	// Setup
	configs := discoveredconfig.DiscoveredConfigs{
		{Path: "c", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "a", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "b", Type: discoveredconfig.ConfigTypeStack},
	}

	// Act
	sorted := configs.Sort()

	// Assert
	require.Len(t, sorted, 3)
	assert.Equal(t, "a", sorted[0].Path)
	assert.Equal(t, "b", sorted[1].Path)
	assert.Equal(t, "c", sorted[2].Path)
}

func TestDiscoveredConfigsFilter(t *testing.T) {
	t.Parallel()

	// Setup
	configs := discoveredconfig.DiscoveredConfigs{
		{Path: "unit1", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "stack1", Type: discoveredconfig.ConfigTypeStack},
		{Path: "unit2", Type: discoveredconfig.ConfigTypeUnit},
	}

	// Test unit filtering
	t.Run("filter units", func(t *testing.T) {
		t.Parallel()

		units := configs.Filter(discoveredconfig.ConfigTypeUnit)
		require.Len(t, units, 2)
		assert.Equal(t, discoveredconfig.ConfigTypeUnit, units[0].Type)
		assert.Equal(t, discoveredconfig.ConfigTypeUnit, units[1].Type)
		assert.ElementsMatch(t, []string{"unit1", "unit2"}, units.Paths())
	})

	// Test stack filtering
	t.Run("filter stacks", func(t *testing.T) {
		t.Parallel()

		stacks := configs.Filter(discoveredconfig.ConfigTypeStack)
		require.Len(t, stacks, 1)
		assert.Equal(t, discoveredconfig.ConfigTypeStack, stacks[0].Type)
		assert.Equal(t, "stack1", stacks[0].Path)
	})
}

func TestDiscoveredConfigsCycleCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		configs       discoveredconfig.DiscoveredConfigs
		errorExpected bool
	}{
		{
			name: "no cycles",
			configs: discoveredconfig.DiscoveredConfigs{
				{
					Path: "a",
					Dependencies: discoveredconfig.DiscoveredConfigs{
						{Path: "b"},
					},
				},
				{Path: "b"},
			},
			errorExpected: false,
		},
		{
			name: "direct cycle",
			configs: discoveredconfig.DiscoveredConfigs{
				{
					Path: "a",
					Dependencies: discoveredconfig.DiscoveredConfigs{
						{
							Path: "b",
							Dependencies: discoveredconfig.DiscoveredConfigs{
								{Path: "a"},
							},
						},
					},
				},
				{Path: "b"},
			},
			errorExpected: true,
		},
		{
			name: "indirect cycle",
			configs: discoveredconfig.DiscoveredConfigs{
				{
					Path: "a",
					Dependencies: discoveredconfig.DiscoveredConfigs{
						{
							Path: "b",
							Dependencies: discoveredconfig.DiscoveredConfigs{
								{
									Path: "c",
									Dependencies: discoveredconfig.DiscoveredConfigs{
										{Path: "a"},
									},
								},
							},
						},
					},
				},
				{Path: "b"},
				{Path: "c"},
			},
			errorExpected: true,
		},
		{
			name: "diamond dependency - no cycle",
			configs: discoveredconfig.DiscoveredConfigs{
				{
					Path: "a",
					Dependencies: discoveredconfig.DiscoveredConfigs{
						{Path: "b"},
						{Path: "c"},
					},
				},
				{
					Path: "b",
					Dependencies: discoveredconfig.DiscoveredConfigs{
						{Path: "d"},
					},
				},
				{
					Path: "c",
					Dependencies: discoveredconfig.DiscoveredConfigs{
						{Path: "d"},
					},
				},
				{Path: "d"},
			},
			errorExpected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := tt.configs.CycleCheck()
			if tt.errorExpected {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cycle detected")
				assert.NotNil(t, cfg)
			} else {
				require.NoError(t, err)
				assert.Nil(t, cfg)
			}
		})
	}
}
