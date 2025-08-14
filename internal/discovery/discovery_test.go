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
	terragruntStackDir := filepath.Join(tmpDir, ".terragrunt-stack", "stack-unit")
	customHiddenDir := filepath.Join(tmpDir, ".custom-hidden", "custom-unit")

	testDirs := []string{
		unit1Dir,
		unit2Dir,
		stack1Dir,
		hiddenUnitDir,
		nestedUnit4Dir,
		terragruntStackDir,
		customHiddenDir,
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create test files
	testFiles := map[string]string{
		filepath.Join(unit1Dir, "terragrunt.hcl"):           "",
		filepath.Join(unit2Dir, "terragrunt.hcl"):           "",
		filepath.Join(stack1Dir, "terragrunt.stack.hcl"):    "",
		filepath.Join(hiddenUnitDir, "terragrunt.hcl"):      "",
		filepath.Join(nestedUnit4Dir, "terragrunt.hcl"):     "",
		filepath.Join(terragruntStackDir, "terragrunt.hcl"): "",
		filepath.Join(customHiddenDir, "terragrunt.hcl"):    "",
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
			wantUnits:  []string{unit1Dir, unit2Dir, nestedUnit4Dir, terragruntStackDir},
			wantStacks: []string{stack1Dir},
		},
		{
			name:       "discovery with custom hidden dirs",
			discovery:  discovery.NewDiscovery(tmpDir).WithIncludeHiddenDirs([]string{".terragrunt-stack", ".custom-hidden"}),
			wantUnits:  []string{unit1Dir, unit2Dir, nestedUnit4Dir, terragruntStackDir, customHiddenDir},
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

func TestDiscoveryWithExcludeDirs(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test directory structure
	testDirs := []string{
		"unit1",
		"unit2",
		"unit3",
		"excluded",
		"nested/excluded",
		"nested/included",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files
	testFiles := map[string]string{
		"unit1/terragrunt.hcl":           "",
		"unit2/terragrunt.hcl":           "",
		"unit3/terragrunt.hcl":           "",
		"excluded/terragrunt.hcl":        "",
		"nested/excluded/terragrunt.hcl": "",
		"nested/included/terragrunt.hcl": "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		excludeDirs   []string
		wantUnits     []string
		errorExpected bool
	}{
		{
			name:        "exclude single directory",
			excludeDirs: []string{"excluded"},
			wantUnits:   []string{filepath.Join(tmpDir, "unit1"), filepath.Join(tmpDir, "unit2"), filepath.Join(tmpDir, "unit3"), filepath.Join(tmpDir, "nested/excluded"), filepath.Join(tmpDir, "nested/included")},
		},
		{
			name:        "exclude nested directory",
			excludeDirs: []string{"nested/excluded"},
			wantUnits:   []string{filepath.Join(tmpDir, "unit1"), filepath.Join(tmpDir, "unit2"), filepath.Join(tmpDir, "unit3"), filepath.Join(tmpDir, "excluded"), filepath.Join(tmpDir, "nested/included")},
		},
		{
			name:        "exclude with glob pattern",
			excludeDirs: []string{"excluded*"},
			wantUnits:   []string{filepath.Join(tmpDir, "unit1"), filepath.Join(tmpDir, "unit2"), filepath.Join(tmpDir, "unit3"), filepath.Join(tmpDir, "nested/included")},
		},
		{
			name:        "exclude multiple patterns",
			excludeDirs: []string{"excluded", "nested/excluded"},
			wantUnits:   []string{filepath.Join(tmpDir, "unit1"), filepath.Join(tmpDir, "unit2"), filepath.Join(tmpDir, "unit3"), filepath.Join(tmpDir, "nested/included")},
		},
		{
			name:        "exclude with question mark pattern",
			excludeDirs: []string{"unit?"},
			wantUnits:   []string{filepath.Join(tmpDir, "excluded"), filepath.Join(tmpDir, "nested/excluded"), filepath.Join(tmpDir, "nested/included")},
		},
		{
			name:        "exclude with character class pattern",
			excludeDirs: []string{"unit[12]"},
			wantUnits:   []string{filepath.Join(tmpDir, "unit3"), filepath.Join(tmpDir, "excluded"), filepath.Join(tmpDir, "nested/excluded"), filepath.Join(tmpDir, "nested/included")},
		},
		{
			name:        "exclude with exact path match",
			excludeDirs: []string{"unit1"},
			wantUnits:   []string{filepath.Join(tmpDir, "unit2"), filepath.Join(tmpDir, "unit3"), filepath.Join(tmpDir, "excluded"), filepath.Join(tmpDir, "nested/excluded"), filepath.Join(tmpDir, "nested/included")},
		},
		{
			name:        "exclude with parent directory pattern",
			excludeDirs: []string{"nested"},
			wantUnits:   []string{filepath.Join(tmpDir, "unit1"), filepath.Join(tmpDir, "unit2"), filepath.Join(tmpDir, "unit3"), filepath.Join(tmpDir, "excluded")},
		},
		{
			name:        "exclude with empty pattern",
			excludeDirs: []string{""},
			wantUnits:   []string{filepath.Join(tmpDir, "unit1"), filepath.Join(tmpDir, "unit2"), filepath.Join(tmpDir, "unit3"), filepath.Join(tmpDir, "excluded"), filepath.Join(tmpDir, "nested/excluded"), filepath.Join(tmpDir, "nested/included")},
		},
		{
			name:        "exclude with whitespace pattern",
			excludeDirs: []string{"  excluded  "},
			wantUnits:   []string{filepath.Join(tmpDir, "unit1"), filepath.Join(tmpDir, "unit2"), filepath.Join(tmpDir, "unit3"), filepath.Join(tmpDir, "nested/excluded"), filepath.Join(tmpDir, "nested/included")},
		},
		{
			name:        "no exclude patterns",
			excludeDirs: []string{},
			wantUnits:   []string{filepath.Join(tmpDir, "unit1"), filepath.Join(tmpDir, "unit2"), filepath.Join(tmpDir, "unit3"), filepath.Join(tmpDir, "excluded"), filepath.Join(tmpDir, "nested/excluded"), filepath.Join(tmpDir, "nested/included")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest(tmpDir)
			require.NoError(t, err)

			d := discovery.NewDiscovery(tmpDir)
			if len(tt.excludeDirs) > 0 {
				d = d.WithExcludeDirs(tt.excludeDirs)
			}

			configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
			if !tt.errorExpected {
				require.NoError(t, err)
			}

			units := configs.Filter(discovery.ConfigTypeUnit).Paths()

			assert.ElementsMatch(t, units, tt.wantUnits)
		})
	}
}

func TestMatchesExcludePatterns(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test directory structure
	testDirs := []string{
		"app",
		"app/frontend",
		"app/backend",
		"test",
		"test/unit",
		"test/integration",
		"docs",
		"docs/api",
		"docs/user",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files
	testFiles := map[string]string{
		"app/terragrunt.hcl":              "",
		"app/frontend/terragrunt.hcl":     "",
		"app/backend/terragrunt.hcl":      "",
		"test/terragrunt.hcl":             "",
		"test/unit/terragrunt.hcl":        "",
		"test/integration/terragrunt.hcl": "",
		"docs/terragrunt.hcl":             "",
		"docs/api/terragrunt.hcl":         "",
		"docs/user/terragrunt.hcl":        "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	tests := []struct {
		name        string
		excludeDirs []string
		path        string
		shouldMatch bool
	}{
		// Exact path matching
		{
			name:        "exact path match",
			excludeDirs: []string{"app"},
			path:        "app",
			shouldMatch: true,
		},
		{
			name:        "exact path match with nested",
			excludeDirs: []string{"app/frontend"},
			path:        "app/frontend",
			shouldMatch: true,
		},

		// Subdirectory matching
		{
			name:        "parent pattern matches child",
			excludeDirs: []string{"app"},
			path:        "app/frontend",
			shouldMatch: true,
		},
		{
			name:        "child pattern matches parent",
			excludeDirs: []string{"app/frontend"},
			path:        "app",
			shouldMatch: true,
		},

		// Glob patterns
		{
			name:        "glob pattern with asterisk",
			excludeDirs: []string{"app*"},
			path:        "app",
			shouldMatch: true,
		},
		{
			name:        "glob pattern with asterisk matches nested",
			excludeDirs: []string{"app*"},
			path:        "app/frontend",
			shouldMatch: true,
		},
		{
			name:        "glob pattern with asterisk matches docs",
			excludeDirs: []string{"app*"},
			path:        "docs",
			shouldMatch: false,
		},

		// Question mark patterns
		{
			name:        "question mark pattern",
			excludeDirs: []string{"ap?"},
			path:        "app",
			shouldMatch: true,
		},
		{
			name:        "question mark pattern no match",
			excludeDirs: []string{"ap?"},
			path:        "test",
			shouldMatch: false,
		},

		// Character class patterns
		{
			name:        "character class pattern",
			excludeDirs: []string{"app[abc]"},
			path:        "app",
			shouldMatch: false,
		},
		{
			name:        "character class pattern with range",
			excludeDirs: []string{"test[0-9]"},
			path:        "test",
			shouldMatch: false,
		},

		// Edge cases
		{
			name:        "empty pattern",
			excludeDirs: []string{""},
			path:        "app",
			shouldMatch: false,
		},
		{
			name:        "whitespace pattern",
			excludeDirs: []string{"  app  "},
			path:        "app",
			shouldMatch: true,
		},
		{
			name:        "absolute path pattern",
			excludeDirs: []string{filepath.Join(tmpDir, "app")},
			path:        "app",
			shouldMatch: true,
		},
		{
			name:        "absolute path pattern with nested",
			excludeDirs: []string{filepath.Join(tmpDir, "app/frontend")},
			path:        "app/frontend",
			shouldMatch: true,
		},
		{
			name:        "absolute path pattern parent matches child",
			excludeDirs: []string{filepath.Join(tmpDir, "app")},
			path:        "app/frontend",
			shouldMatch: true,
		},
		{
			name:        "absolute path pattern child matches parent",
			excludeDirs: []string{filepath.Join(tmpDir, "app/frontend")},
			path:        "app",
			shouldMatch: true,
		},
		{
			name:        "no exclude patterns",
			excludeDirs: []string{},
			path:        "app",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := discovery.NewDiscovery(tmpDir)
			if len(tt.excludeDirs) > 0 {
				d = d.WithExcludeDirs(tt.excludeDirs)
			}

			path := filepath.Join(tmpDir, tt.path)
			matches := d.MatchesExcludePatterns(path)

			assert.Equal(t, tt.shouldMatch, matches, "Path: %s, Pattern: %v", tt.path, tt.excludeDirs)
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

func TestDiscoveryWithSingleCustomConfigFilename(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	unit1Dir := filepath.Join(tmpDir, "unit1")
	err := os.MkdirAll(unit1Dir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(unit1Dir, "custom1.hcl"), []byte(""), 0644)
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).WithConfigFilenames([]string{"custom1.hcl"})
	opts, err := options.NewTerragruntOptionsForTest(tmpDir)
	require.NoError(t, err)

	configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	units := configs.Filter(discovery.ConfigTypeUnit).Paths()
	assert.ElementsMatch(t, []string{unit1Dir}, units)
}

func TestDiscoveryWithStackConfigParsing(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test directory structure
	stackDir := filepath.Join(tmpDir, "stack")
	unitDir := filepath.Join(tmpDir, "unit")

	testDirs := []string{
		stackDir,
		unitDir,
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create a stack file with unit blocks (which would cause parsing errors if parsed as unit config)
	stackContent := `
unit "unit_a" {
  source = "${get_repo_root()}/unit_a"
  path   = "unit_a"
}

unit "unit_b" {
  source = "${get_repo_root()}/unit_b"
  path   = "unit_b"
}
`

	// Create a unit file with valid unit configuration
	unitContent := `
terraform {
  source = "."
}

inputs = {
  test = "value"
}
`

	// Create test files
	testFiles := map[string]string{
		filepath.Join(stackDir, "terragrunt.stack.hcl"): stackContent,
		filepath.Join(unitDir, "terragrunt.hcl"):        unitContent,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Test that d with parsing enabled doesn't fail on stack files
	d := discovery.NewDiscovery(tmpDir).WithDiscoverDependencies()

	opts, err := options.NewTerragruntOptionsForTest(tmpDir)
	require.NoError(t, err)

	configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	// Verify that both stack and unit configurations are discovered
	units := configs.Filter(discovery.ConfigTypeUnit)
	stacks := configs.Filter(discovery.ConfigTypeStack)

	assert.Len(t, units, 1)
	assert.Len(t, stacks, 1)

	// Verify that stack configuration is not parsed (Parsed should be nil)
	stackConfig := stacks[0]
	assert.Nil(t, stackConfig.Parsed, "Stack configuration should not be parsed")

	// Verify that unit configuration is parsed (Parsed should not be nil)
	unitConfig := units[0]
	assert.NotNil(t, unitConfig.Parsed, "Unit configuration should be parsed")
}

func TestDiscoveryMatchesIncludePatterns(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test directory structure
	testDirs := []string{
		"app",
		"app/frontend",
		"infra",
		"infra/vpc",
		"shared",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files
	testFiles := map[string]string{
		"app/terragrunt.hcl":          "",
		"app/frontend/terragrunt.hcl": "",
		"infra/terragrunt.hcl":        "",
		"infra/vpc/terragrunt.hcl":    "",
		"shared/terragrunt.hcl":       "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		includeDirs   []string
		wantUnits     []string
		strictInclude bool
	}{
		{
			name:          "no include patterns - should match everything",
			includeDirs:   []string{},
			strictInclude: false,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
		{
			name:          "non-strict include - should include all",
			includeDirs:   []string{"app"},
			strictInclude: false,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
		{
			name:          "strict include with app pattern",
			includeDirs:   []string{"app"},
			strictInclude: true,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
		{
			name:          "strict include with glob pattern",
			includeDirs:   []string{"app*"},
			strictInclude: true,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
		{
			name:          "strict include with multiple patterns",
			includeDirs:   []string{"app", "infra"},
			strictInclude: true,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := discovery.NewDiscovery(tmpDir).WithIncludeDirs(tt.includeDirs)
			if tt.strictInclude {
				d = d.WithStrictInclude()
			}

			opts, err := options.NewTerragruntOptionsForTest(tmpDir)
			require.NoError(t, err)

			configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			units := configs.Filter(discovery.ConfigTypeUnit).Paths()
			assert.ElementsMatch(t, tt.wantUnits, units)
		})
	}
}

func TestDependencyDiscoveryMatchesIncludePatterns(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test directory structure
	testDirs := []string{
		"app",
		"app/frontend",
		"infra",
		"infra/vpc",
		"shared",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files with dependencies
	testFiles := map[string]string{
		"app/terragrunt.hcl": `
dependency "frontend" {
	config_path = "./frontend"
}

dependency "shared" {
	config_path = "../shared"
}
`,
		"app/frontend/terragrunt.hcl": `
dependency "shared" {
	config_path = "../../shared"
}
`,
		"infra/terragrunt.hcl": `
dependency "vpc" {
	config_path = "./vpc"
}
`,
		"infra/vpc/terragrunt.hcl": "",
		"shared/terragrunt.hcl":    "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		includeDirs   []string
		wantUnits     []string
		strictInclude bool
	}{
		{
			name:          "no include patterns - should match everything",
			includeDirs:   []string{},
			strictInclude: false,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
		{
			name:          "strict include with app pattern",
			includeDirs:   []string{"app"},
			strictInclude: true,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
		{
			name:          "strict include with glob pattern",
			includeDirs:   []string{"app*"},
			strictInclude: true,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
		{
			name:          "strict include with multiple patterns",
			includeDirs:   []string{"app", "infra"},
			strictInclude: true,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := discovery.NewDiscovery(tmpDir).WithIncludeDirs(tt.includeDirs).WithDiscoverDependencies()
			if tt.strictInclude {
				d = d.WithStrictInclude()
			}

			opts, err := options.NewTerragruntOptionsForTest(tmpDir)
			require.NoError(t, err)

			configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			units := configs.Filter(discovery.ConfigTypeUnit).Paths()
			assert.ElementsMatch(t, tt.wantUnits, units)
		})
	}
}

func TestDiscoveryWithStrictIncludeMode(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test directory structure
	testDirs := []string{
		"app",
		"app/frontend",
		"infra",
		"infra/vpc",
		"shared",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files
	testFiles := map[string]string{
		"app/terragrunt.hcl":          "",
		"app/frontend/terragrunt.hcl": "",
		"infra/terragrunt.hcl":        "",
		"infra/vpc/terragrunt.hcl":    "",
		"shared/terragrunt.hcl":       "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		includeDirs   []string
		wantUnits     []string
		strictInclude bool
	}{
		{
			name:          "strict include mode - only app directories",
			includeDirs:   []string{"app"},
			strictInclude: true,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
		{
			name:          "strict include mode - only infra directories",
			includeDirs:   []string{"infra"},
			strictInclude: true,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
		{
			name:          "strict include mode - multiple patterns",
			includeDirs:   []string{"app", "infra"},
			strictInclude: true,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
		{
			name:          "non-strict include mode - should include all",
			includeDirs:   []string{"app"},
			strictInclude: false,
			wantUnits:     []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := discovery.NewDiscovery(tmpDir).WithIncludeDirs(tt.includeDirs)
			if tt.strictInclude {
				d = d.WithStrictInclude()
			}

			opts, err := options.NewTerragruntOptionsForTest(tmpDir)
			require.NoError(t, err)

			configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			units := configs.Filter(discovery.ConfigTypeUnit).Paths()
			assert.ElementsMatch(t, tt.wantUnits, units)
		})
	}
}

func TestDiscoveryWithExcludeByDefault(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test directory structure
	testDirs := []string{
		"app",
		"app/frontend",
		"infra",
		"infra/vpc",
		"shared",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files
	testFiles := map[string]string{
		"app/terragrunt.hcl":          "",
		"app/frontend/terragrunt.hcl": "",
		"infra/terragrunt.hcl":        "",
		"infra/vpc/terragrunt.hcl":    "",
		"shared/terragrunt.hcl":       "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	tests := []struct {
		name             string
		includeDirs      []string
		wantUnits        []string
		excludeByDefault bool
	}{
		{
			name:             "exclude by default with app pattern",
			includeDirs:      []string{"app"},
			excludeByDefault: true,
			wantUnits:        []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend")},
		},
		{
			name:             "exclude by default with infra pattern",
			includeDirs:      []string{"infra"},
			excludeByDefault: true,
			wantUnits:        []string{filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc")},
		},
		{
			name:             "exclude by default with multiple patterns",
			includeDirs:      []string{"app", "infra"},
			excludeByDefault: true,
			wantUnits:        []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc")},
		},
		{
			name:             "exclude by default with glob pattern",
			includeDirs:      []string{"app*"},
			excludeByDefault: true,
			wantUnits:        []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend")},
		},
		{
			name:             "exclude by default with no patterns - should include all",
			includeDirs:      []string{},
			excludeByDefault: true,
			wantUnits:        []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := discovery.NewDiscovery(tmpDir).WithIncludeDirs(tt.includeDirs)
			if tt.excludeByDefault {
				d = d.WithExcludeByDefault()
			}

			opts, err := options.NewTerragruntOptionsForTest(tmpDir)
			require.NoError(t, err)

			configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			units := configs.Filter(discovery.ConfigTypeUnit).Paths()
			assert.ElementsMatch(t, tt.wantUnits, units)
		})
	}
}

func TestDiscoveryWithStrictIncludeAndExcludeByDefault(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test directory structure
	testDirs := []string{
		"app",
		"app/frontend",
		"infra",
		"infra/vpc",
		"shared",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
	}

	// Create test files
	testFiles := map[string]string{
		"app/terragrunt.hcl":          "",
		"app/frontend/terragrunt.hcl": "",
		"infra/terragrunt.hcl":        "",
		"infra/vpc/terragrunt.hcl":    "",
		"shared/terragrunt.hcl":       "",
	}

	for path, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644)
		require.NoError(t, err)
	}

	tests := []struct {
		name             string
		includeDirs      []string
		wantUnits        []string
		strictInclude    bool
		excludeByDefault bool
	}{
		{
			name:             "strict include with exclude by default - app only",
			includeDirs:      []string{"app"},
			strictInclude:    true,
			excludeByDefault: true,
			wantUnits:        []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend")},
		},
		{
			name:             "strict include with exclude by default - infra only",
			includeDirs:      []string{"infra"},
			strictInclude:    true,
			excludeByDefault: true,
			wantUnits:        []string{filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc")},
		},
		{
			name:             "strict include with exclude by default - multiple patterns",
			includeDirs:      []string{"app", "infra"},
			strictInclude:    true,
			excludeByDefault: true,
			wantUnits:        []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc")},
		},
		{
			name:             "strict include without exclude by default - should include all",
			includeDirs:      []string{"app"},
			strictInclude:    true,
			excludeByDefault: false,
			wantUnits:        []string{filepath.Join(tmpDir, "app"), filepath.Join(tmpDir, "app", "frontend"), filepath.Join(tmpDir, "infra"), filepath.Join(tmpDir, "infra", "vpc"), filepath.Join(tmpDir, "shared")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := discovery.NewDiscovery(tmpDir).WithIncludeDirs(tt.includeDirs)
			if tt.strictInclude {
				d = d.WithStrictInclude()
			}
			if tt.excludeByDefault {
				d = d.WithExcludeByDefault()
			}

			opts, err := options.NewTerragruntOptionsForTest(tmpDir)
			require.NoError(t, err)

			configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			units := configs.Filter(discovery.ConfigTypeUnit).Paths()
			assert.ElementsMatch(t, tt.wantUnits, units)
		})
	}
}
