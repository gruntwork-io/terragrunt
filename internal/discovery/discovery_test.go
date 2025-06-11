package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscovery(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test directory structure
	unit1Dir := filepath.Join(tmpDir, "unit1")
	unit2Dir := filepath.Join(tmpDir, "unit2")
	stack1Dir := filepath.Join(tmpDir, "stack1")
	hiddenUnitDir := filepath.Join(tmpDir, ".hidden", "hidden-unit")
	nestedUnit4Dir := filepath.Join(tmpDir, "nested", "unit4")

	testDirs := []string{
		unit1Dir,
		unit2Dir,
		stack1Dir,
		hiddenUnitDir,
		nestedUnit4Dir,
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create test files
	testFiles := map[string]string{
		filepath.Join(unit1Dir, "terragrunt.hcl"):        "",
		filepath.Join(unit2Dir, "terragrunt.hcl"):        "",
		filepath.Join(stack1Dir, "terragrunt.stack.hcl"): "",
		filepath.Join(hiddenUnitDir, "terragrunt.hcl"):   "",
		filepath.Join(nestedUnit4Dir, "terragrunt.hcl"):  "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		discovery     *discovery.Discovery
		wantUnits     []string
		wantStacks    []string
		errorExpected bool
	}{
		{
			name:       "basic discovery without hidden",
			discovery:  discovery.NewDiscovery(tmpDir),
			wantUnits:  []string{unit1Dir, unit2Dir, nestedUnit4Dir},
			wantStacks: []string{stack1Dir},
		},
		{
			name:       "discovery with hidden",
			discovery:  discovery.NewDiscovery(tmpDir).WithHidden(),
			wantUnits:  []string{unit1Dir, unit2Dir, hiddenUnitDir, nestedUnit4Dir},
			wantStacks: []string{stack1Dir},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest(tmpDir)
			require.NoError(t, err)

			configs, err := tt.discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			if !tt.errorExpected {
				require.NoError(t, err)
			}

			units := configs.Filter(discovery.ConfigTypeUnit).Paths()
			stacks := configs.Filter(discovery.ConfigTypeStack).Paths()

			assert.ElementsMatch(t, units, tt.wantUnits)
			assert.ElementsMatch(t, stacks, tt.wantStacks)
		})
	}
}

func TestDiscoveredConfigsSort(t *testing.T) {
	t.Parallel()

	// Setup
	configs := discovery.DiscoveredConfigs{
		{Path: "c", Type: discovery.ConfigTypeUnit},
		{Path: "a", Type: discovery.ConfigTypeUnit},
		{Path: "b", Type: discovery.ConfigTypeStack},
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
	configs := discovery.DiscoveredConfigs{
		{Path: "unit1", Type: discovery.ConfigTypeUnit},
		{Path: "stack1", Type: discovery.ConfigTypeStack},
		{Path: "unit2", Type: discovery.ConfigTypeUnit},
	}

	// Test unit filtering
	t.Run("filter units", func(t *testing.T) {
		t.Parallel()

		units := configs.Filter(discovery.ConfigTypeUnit)
		require.Len(t, units, 2)
		assert.Equal(t, discovery.ConfigTypeUnit, units[0].Type)
		assert.Equal(t, discovery.ConfigTypeUnit, units[1].Type)
		assert.ElementsMatch(t, []string{"unit1", "unit2"}, units.Paths())
	})

	// Test stack filtering
	t.Run("filter stacks", func(t *testing.T) {
		t.Parallel()

		stacks := configs.Filter(discovery.ConfigTypeStack)
		require.Len(t, stacks, 1)
		assert.Equal(t, discovery.ConfigTypeStack, stacks[0].Type)
		assert.Equal(t, "stack1", stacks[0].Path)
	})
}

func TestDiscoveryWithDependencies(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	internalDir := filepath.Join(tmpDir, "internal")
	appDir := filepath.Join(internalDir, "app")
	dbDir := filepath.Join(internalDir, "db")
	vpcDir := filepath.Join(internalDir, "vpc")

	externalDir := filepath.Join(tmpDir, "external")
	externalAppDir := filepath.Join(externalDir, "app")

	testDirs := []string{
		appDir,
		dbDir,
		vpcDir,
		externalAppDir,
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create test files with dependencies
	testFiles := map[string]string{
		filepath.Join(appDir, "terragrunt.hcl"): `
		dependency "db" {
			config_path = "../db"
		}

		dependency "external" {
			config_path = "../../external/app"
		}
		`,
		filepath.Join(dbDir, "terragrunt.hcl"): `
		dependency "vpc" {
			config_path = "../vpc"
		}
		`,
		filepath.Join(vpcDir, "terragrunt.hcl"):         ``,
		filepath.Join(externalAppDir, "terragrunt.hcl"): ``,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Create base options that will be cloned for each test
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = internalDir
	opts.RootWorkingDir = internalDir

	tests := []struct {
		name      string
		discovery *discovery.Discovery
		// Note that when comparing against this,
		// we'll nil out the parsed configurations,
		// as it doesn't matter for this test
		wantDiscovery discovery.DiscoveredConfigs
		errorExpected bool
	}{
		{
			name:      "discovery without dependencies",
			discovery: discovery.NewDiscovery(internalDir),
			wantDiscovery: discovery.DiscoveredConfigs{
				{Path: appDir, Type: discovery.ConfigTypeUnit},
				{Path: dbDir, Type: discovery.ConfigTypeUnit},
				{Path: vpcDir, Type: discovery.ConfigTypeUnit},
			},
		},
		{
			name:      "discovery with dependencies",
			discovery: discovery.NewDiscovery(internalDir).WithDiscoverDependencies(),
			wantDiscovery: discovery.DiscoveredConfigs{
				{Path: appDir, Type: discovery.ConfigTypeUnit, Dependencies: discovery.DiscoveredConfigs{
					{Path: dbDir, Type: discovery.ConfigTypeUnit, Dependencies: discovery.DiscoveredConfigs{
						{Path: vpcDir, Type: discovery.ConfigTypeUnit},
					}},
					{Path: externalAppDir, Type: discovery.ConfigTypeUnit, External: true},
				}},
				{Path: dbDir, Type: discovery.ConfigTypeUnit, Dependencies: discovery.DiscoveredConfigs{
					{Path: vpcDir, Type: discovery.ConfigTypeUnit},
				}},
				{Path: vpcDir, Type: discovery.ConfigTypeUnit},
			},
		},
		{
			name:      "discovery with external dependencies",
			discovery: discovery.NewDiscovery(internalDir).WithDiscoverDependencies().WithDiscoverExternalDependencies(),
			wantDiscovery: discovery.DiscoveredConfigs{
				{Path: appDir, Type: discovery.ConfigTypeUnit, Dependencies: discovery.DiscoveredConfigs{
					{Path: dbDir, Type: discovery.ConfigTypeUnit, Dependencies: discovery.DiscoveredConfigs{
						{Path: vpcDir, Type: discovery.ConfigTypeUnit},
					}},
					{Path: externalAppDir, Type: discovery.ConfigTypeUnit, External: true},
				}},
				{Path: dbDir, Type: discovery.ConfigTypeUnit, Dependencies: discovery.DiscoveredConfigs{
					{Path: vpcDir, Type: discovery.ConfigTypeUnit},
				}},
				{Path: vpcDir, Type: discovery.ConfigTypeUnit},
				{Path: externalAppDir, Type: discovery.ConfigTypeUnit, External: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			configs, err := tt.discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			if tt.errorExpected {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Sort the configs and their dependencies to ensure consistent ordering
			configs = configs.Sort()
			for _, cfg := range configs {
				cfg.Dependencies = cfg.Dependencies.Sort()
				for _, dep := range cfg.Dependencies {
					dep.Dependencies = dep.Dependencies.Sort()
				}
			}

			tt.wantDiscovery = tt.wantDiscovery.Sort()
			for _, cfg := range tt.wantDiscovery {
				cfg.Dependencies = cfg.Dependencies.Sort()
				for _, dep := range cfg.Dependencies {
					dep.Dependencies = dep.Dependencies.Sort()
				}
			}

			// nil out the parsed configurations, as it doesn't matter for this test
			for _, cfg := range configs {
				cfg.Parsed = nil
			}

			assert.Equal(t, tt.wantDiscovery, configs)
		})
	}
}

func TestDiscoveredConfigsCycleCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		configs       discovery.DiscoveredConfigs
		errorExpected bool
	}{
		{
			name: "no cycles",
			configs: discovery.DiscoveredConfigs{
				{
					Path: "a",
					Dependencies: discovery.DiscoveredConfigs{
						{Path: "b"},
					},
				},
				{Path: "b"},
			},
			errorExpected: false,
		},
		{
			name: "direct cycle",
			configs: discovery.DiscoveredConfigs{
				{
					Path: "a",
					Dependencies: discovery.DiscoveredConfigs{
						{
							Path: "b",
							Dependencies: discovery.DiscoveredConfigs{
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
			configs: discovery.DiscoveredConfigs{
				{
					Path: "a",
					Dependencies: discovery.DiscoveredConfigs{
						{
							Path: "b",
							Dependencies: discovery.DiscoveredConfigs{
								{
									Path: "c",
									Dependencies: discovery.DiscoveredConfigs{
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
			configs: discovery.DiscoveredConfigs{
				{
					Path: "a",
					Dependencies: discovery.DiscoveredConfigs{
						{Path: "b"},
						{Path: "c"},
					},
				},
				{
					Path: "b",
					Dependencies: discovery.DiscoveredConfigs{
						{Path: "d"},
					},
				},
				{
					Path: "c",
					Dependencies: discovery.DiscoveredConfigs{
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

func TestDiscoveryWithExclude(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create test directory structure
	testDirs := []string{
		"unit1",
		"unit2",
		"unit3",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files with exclude configurations
	testFiles := map[string]string{
		"unit1/terragrunt.hcl": `
exclude {
  if      = true
  actions = ["plan"]
}`,
		"unit2/terragrunt.hcl": `
exclude {
  if      = true
  actions = ["apply"]
}`,
		"unit3/terragrunt.hcl": "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	tgOpts := options.NewTerragruntOptions()
	tgOpts.WorkingDir = tmpDir

	l := logger.CreateLogger()

	l.Formatter().SetDisabledColors(true)

	// Test discovery with exclude parsing
	d := discovery.NewDiscovery(tmpDir).WithParseExclude()

	cfgs, err := d.Discover(t.Context(), l, tgOpts)
	require.NoError(t, err)

	// Verify we found all configurations
	assert.Len(t, cfgs, 3)

	// Helper to find config by path
	findConfig := func(path string) *discovery.DiscoveredConfig {
		for _, cfg := range cfgs {
			if filepath.Base(cfg.Path) == path {
				return cfg
			}
		}
		return nil
	}

	// Verify exclude configurations were parsed correctly
	unit1 := findConfig("unit1")
	require.NotNil(t, unit1)
	require.NotNil(t, unit1.Parsed)
	require.NotNil(t, unit1.Parsed.Exclude)
	assert.Contains(t, unit1.Parsed.Exclude.Actions, "plan")

	unit2 := findConfig("unit2")
	require.NotNil(t, unit2)
	require.NotNil(t, unit2.Parsed)
	require.NotNil(t, unit2.Parsed.Exclude)
	assert.Contains(t, unit2.Parsed.Exclude.Actions, "apply")

	unit3 := findConfig("unit3")
	require.NotNil(t, unit3)
	if unit3.Parsed != nil {
		assert.Nil(t, unit3.Parsed.Exclude)
	}
}
