package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscovery(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test directory structure
	testDirs := []string{
		"unit1",
		"unit2",
		"stack1",
		".hidden/hidden-unit",
		"nested/unit4",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create test files
	testFiles := map[string]string{
		"unit1/terragrunt.hcl":               "",
		"unit2/terragrunt.hcl":               "",
		"stack1/terragrunt.stack.hcl":        "",
		".hidden/hidden-unit/terragrunt.hcl": "",
		"nested/unit4/terragrunt.hcl":        "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name          string
		discovery     *Discovery
		wantUnits     []string
		wantStacks    []string
		errorExpected bool
	}{
		{
			name:       "basic discovery without hidden",
			discovery:  NewDiscovery(tmpDir),
			wantUnits:  []string{"unit1", "unit2", "nested/unit4"},
			wantStacks: []string{"stack1"},
		},
		{
			name:       "discovery with hidden",
			discovery:  NewDiscovery(tmpDir).WithHidden(),
			wantUnits:  []string{"unit1", "unit2", ".hidden/hidden-unit", "nested/unit4"},
			wantStacks: []string{"stack1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, err := tt.discovery.Discover()
			if !tt.errorExpected {
				require.NoError(t, err)
			}

			units := configs.Filter(ConfigTypeUnit).Paths()
			stacks := configs.Filter(ConfigTypeStack).Paths()

			assert.ElementsMatch(t, units, tt.wantUnits)
			assert.ElementsMatch(t, stacks, tt.wantStacks)
		})
	}
}

func TestDiscoveredConfigsSort(t *testing.T) {
	// Setup
	configs := DiscoveredConfigs{
		{Path: "c", Type: ConfigTypeUnit},
		{Path: "a", Type: ConfigTypeUnit},
		{Path: "b", Type: ConfigTypeStack},
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
	// Setup
	configs := DiscoveredConfigs{
		{Path: "a", Type: ConfigTypeUnit},
		{Path: "b", Type: ConfigTypeStack},
		{Path: "c", Type: ConfigTypeUnit},
	}

	// Test unit filtering
	t.Run("filter units", func(t *testing.T) {
		units := configs.Filter(ConfigTypeUnit)
		require.Len(t, units, 2)
		assert.Equal(t, ConfigTypeUnit, units[0].Type)
		assert.Equal(t, ConfigTypeUnit, units[1].Type)
		assert.ElementsMatch(t, []string{"a", "c"}, units.Paths())
	})

	// Test stack filtering
	t.Run("filter stacks", func(t *testing.T) {
		stacks := configs.Filter(ConfigTypeStack)
		require.Len(t, stacks, 1)
		assert.Equal(t, ConfigTypeStack, stacks[0].Type)
		assert.Equal(t, "b", stacks[0].Path)
	})
}
