package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
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

			units := configs.Filter(component.Unit).Paths()
			stacks := configs.Filter(component.Stack).Paths()

			assert.ElementsMatch(t, units, tt.wantUnits)
			assert.ElementsMatch(t, stacks, tt.wantStacks)
		})
	}
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
		discovery     *discovery.Discovery
		setupExpected func() component.Components
		name          string
		errorExpected bool
	}{
		{
			name:      "discovery without dependencies",
			discovery: discovery.NewDiscovery(internalDir),
			setupExpected: func() component.Components {
				app := &component.Component{Path: appDir, Kind: component.Unit}
				db := &component.Component{Path: dbDir, Kind: component.Unit}
				vpc := &component.Component{Path: vpcDir, Kind: component.Unit}
				return component.Components{app, db, vpc}
			},
		},
		{
			name:      "discovery with dependencies",
			discovery: discovery.NewDiscovery(internalDir).WithDiscoverDependencies(),
			setupExpected: func() component.Components {
				vpc := &component.Component{Path: vpcDir, Kind: component.Unit}
				db := &component.Component{Path: dbDir, Kind: component.Unit}
				db.AddDependency(vpc)
				externalApp := &component.Component{Path: externalAppDir, Kind: component.Unit, External: true}
				app := &component.Component{Path: appDir, Kind: component.Unit}
				app.AddDependency(db)
				app.AddDependency(externalApp)
				return component.Components{app, db, vpc}
			},
		},
		{
			name:      "discovery with external dependencies",
			discovery: discovery.NewDiscovery(internalDir).WithDiscoverDependencies().WithDiscoverExternalDependencies(),
			setupExpected: func() component.Components {
				vpc := &component.Component{Path: vpcDir, Kind: component.Unit}
				db := &component.Component{Path: dbDir, Kind: component.Unit}
				db.AddDependency(vpc)
				externalApp := &component.Component{Path: externalAppDir, Kind: component.Unit, External: true}
				app := &component.Component{Path: appDir, Kind: component.Unit}
				app.AddDependency(db)
				app.AddDependency(externalApp)
				return component.Components{app, db, vpc, externalApp}
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

			// Build expected results
			wantDiscovery := tt.setupExpected()

			// nil out the parsed configurations, as it doesn't matter for this test
			for _, cfg := range configs {
				cfg.Parsed = nil
			}

			// Compare basic component properties
			require.Len(t, configs, len(wantDiscovery))

			configs = configs.Sort()
			wantDiscovery = wantDiscovery.Sort()

			for i, cfg := range configs {
				want := wantDiscovery[i]
				assert.Equal(t, want.Path, cfg.Path, "Component path mismatch at index %d", i)
				assert.Equal(t, want.Kind, cfg.Kind, "Component kind mismatch at index %d", i)
				assert.Equal(t, want.External, cfg.External, "Component external flag mismatch at index %d", i)

				// Compare dependencies
				cfgDeps := cfg.Dependencies().Sort()
				wantDeps := want.Dependencies().Sort()
				require.Len(t, cfgDeps, len(wantDeps), "Dependencies count mismatch for %s", cfg.Path)

				for j, dep := range cfgDeps {
					wantDep := wantDeps[j]
					assert.Equal(t, wantDep.Path, dep.Path, "Dependency path mismatch at component %d, dependency %d", i, j)
					assert.Equal(t, wantDep.Kind, dep.Kind, "Dependency kind mismatch at component %d, dependency %d", i, j)
					assert.Equal(t, wantDep.External, dep.External, "Dependency external flag mismatch at component %d, dependency %d", i, j)

					// Compare nested dependencies (one level deep)
					depDeps := dep.Dependencies().Sort()
					wantDepDeps := wantDep.Dependencies().Sort()
					require.Len(t, depDeps, len(wantDepDeps), "Nested dependencies count mismatch for %s -> %s", cfg.Path, dep.Path)

					for k, nestedDep := range depDeps {
						wantNestedDep := wantDepDeps[k]
						assert.Equal(t, wantNestedDep.Path, nestedDep.Path, "Nested dependency path mismatch")
					}
				}
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
	findConfig := func(path string) *component.Component {
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

	units := configs.Filter(component.Unit).Paths()
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
	units := configs.Filter(component.Unit)
	stacks := configs.Filter(component.Stack)

	assert.Len(t, units, 1)
	assert.Len(t, stacks, 1)

	// Verify that stack configuration is not parsed (Parsed should be nil)
	stackConfig := stacks[0]
	assert.Nil(t, stackConfig.Parsed, "Stack configuration should not be parsed")

	// Verify that unit configuration is parsed (Parsed should not be nil)
	unitConfig := units[0]
	assert.NotNil(t, unitConfig.Parsed, "Unit configuration should be parsed")
}

func TestDiscoveryIncludeExcludeFilters(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	unit1Dir := filepath.Join(tmpDir, "unit1")
	unit2Dir := filepath.Join(tmpDir, "unit2")
	unit3Dir := filepath.Join(tmpDir, "unit3")

	for _, d := range []string{unit1Dir, unit2Dir, unit3Dir} {
		require.NoError(t, os.MkdirAll(d, 0755))
	}

	for _, f := range []string{
		filepath.Join(unit1Dir, "terragrunt.hcl"),
		filepath.Join(unit2Dir, "terragrunt.hcl"),
		filepath.Join(unit3Dir, "terragrunt.hcl"),
	} {
		require.NoError(t, os.WriteFile(f, []byte(""), 0644))
	}

	l := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest(tmpDir)
	require.NoError(t, err)

	// Exclude unit2
	d := discovery.NewDiscovery(tmpDir).WithExcludeDirs([]string{unit2Dir})
	cfgs, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{unit1Dir, unit3Dir}, cfgs.Filter(component.Unit).Paths())

	// Exclude-by-default and include only unit1
	d = discovery.NewDiscovery(tmpDir).WithExcludeByDefault().WithIncludeDirs([]string{unit1Dir})
	cfgs, err = d.Discover(t.Context(), l, opts)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{unit1Dir}, cfgs.Filter(component.Unit).Paths())

	// Strict include behaves the same
	d = discovery.NewDiscovery(tmpDir).WithStrictInclude().WithIncludeDirs([]string{unit3Dir})
	cfgs, err = d.Discover(t.Context(), l, opts)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{unit3Dir}, cfgs.Filter(component.Unit).Paths())
}

func TestDiscoveryHiddenIncludedByIncludeDirs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	hiddenUnitDir := filepath.Join(tmpDir, ".hidden", "hunit")
	require.NoError(t, os.MkdirAll(hiddenUnitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hiddenUnitDir, "terragrunt.hcl"), []byte(""), 0644))

	l := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest(tmpDir)
	require.NoError(t, err)

	// Without hidden, but included via includeDirs pattern
	d := discovery.NewDiscovery(tmpDir).WithIncludeDirs([]string{filepath.Join(tmpDir, ".hidden", "**")})
	cfgs, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{hiddenUnitDir}, cfgs.Filter(component.Unit).Paths())
}

func TestDiscoveryStackHiddenAllowed(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	stackHiddenDir := filepath.Join(tmpDir, ".terragrunt-stack", "u")
	require.NoError(t, os.MkdirAll(stackHiddenDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stackHiddenDir, "terragrunt.hcl"), []byte(""), 0644))

	l := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest(tmpDir)
	require.NoError(t, err)

	// Should be discovered even without WithHidden()
	d := discovery.NewDiscovery(tmpDir)
	cfgs, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)
	assert.Contains(t, cfgs.Filter(component.Unit).Paths(), stackHiddenDir)
}

func TestDiscoveryIgnoreExternalDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	internalDir := filepath.Join(tmpDir, "internal")
	externalDir := filepath.Join(tmpDir, "external")
	appDir := filepath.Join(internalDir, "app")
	dbDir := filepath.Join(internalDir, "db")
	vpcDir := filepath.Join(internalDir, "vpc")
	extApp := filepath.Join(externalDir, "app")

	for _, d := range []string{appDir, dbDir, vpcDir, extApp} {
		require.NoError(t, os.MkdirAll(d, 0755))
	}

	require.NoError(t, os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(`
	dependency "db" { config_path = "../db" }
	dependency "external" { config_path = "../../external/app" }
	`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dbDir, "terragrunt.hcl"), []byte(`
	dependency "vpc" { config_path = "../vpc" }
	`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vpcDir, "terragrunt.hcl"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(extApp, "terragrunt.hcl"), []byte(""), 0644))

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = internalDir
	opts.RootWorkingDir = internalDir

	l := logger.CreateLogger()

	d := discovery.NewDiscovery(internalDir).WithDiscoverDependencies().WithIgnoreExternalDependencies()
	cfgs, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	// Find app config and assert it only has internal deps
	var appCfg *component.Component

	for _, c := range cfgs {
		if c.Path == appDir {
			appCfg = c
			break
		}
	}

	require.NotNil(t, appCfg)
	depPaths := appCfg.Dependencies().Paths()
	assert.Contains(t, depPaths, dbDir)
	assert.NotContains(t, depPaths, extApp)
}

func TestDiscoveryPopulatesReadingField(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	appDir := filepath.Join(tmpDir, "app")
	require.NoError(t, os.MkdirAll(appDir, 0755))

	// Create shared files that will be read
	sharedHCL := filepath.Join(tmpDir, "shared.hcl")
	sharedTFVars := filepath.Join(tmpDir, "shared.tfvars")

	require.NoError(t, os.WriteFile(sharedHCL, []byte(`
		locals {
			common_value = "test"
		}
	`), 0644))

	require.NoError(t, os.WriteFile(sharedTFVars, []byte(`
		test_var = "value"
	`), 0644))

	// Create terragrunt config that reads both files
	terragruntConfig := filepath.Join(appDir, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(terragruntConfig, []byte(`
		locals {
			shared_config = read_terragrunt_config("../shared.hcl")
			tfvars = read_tfvars_file("../shared.tfvars")
		}
	`), 0644))

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	l := logger.CreateLogger()

	// Discover and parse components
	d := discovery.NewDiscovery(tmpDir).WithParseInclude()
	cfgs, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	// Find the app component
	var appComponent *component.Component

	for _, c := range cfgs {
		if c.Path == appDir {
			appComponent = c
			break
		}
	}

	require.NotNil(t, appComponent, "app component should be discovered")
	require.NotNil(t, appComponent.Reading, "Reading field should be initialized")

	// Verify Reading field contains the files that were read
	require.NotEmpty(t, appComponent.Reading, "should have read files")
	assert.Contains(t, appComponent.Reading, sharedHCL, "should contain shared.hcl")
	assert.Contains(t, appComponent.Reading, sharedTFVars, "should contain shared.tfvars")
}
