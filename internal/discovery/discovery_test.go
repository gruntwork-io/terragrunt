package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
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
			discovery:  discovery.NewDiscovery(tmpDir).WithNoHidden(),
			wantUnits:  []string{unit1Dir, unit2Dir, nestedUnit4Dir},
			wantStacks: []string{stack1Dir},
		},
		{
			name:       "discovery with hidden",
			discovery:  discovery.NewDiscovery(tmpDir),
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

			units := configs.Filter(component.UnitKind).Paths()
			stacks := configs.Filter(component.StackKind).Paths()

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
				app := component.NewUnit(appDir)
				db := component.NewUnit(dbDir)
				vpc := component.NewUnit(vpcDir)
				return component.Components{app, db, vpc}
			},
		},
		{
			name:      "discovery with dependencies",
			discovery: discovery.NewDiscovery(internalDir).WithDiscoverDependencies(),
			setupExpected: func() component.Components {
				vpc := component.NewUnit(vpcDir)
				db := component.NewUnit(dbDir)
				db.AddDependency(vpc)
				externalApp := component.NewUnit(externalAppDir)
				externalApp.SetExternal()
				app := component.NewUnit(appDir)
				app.AddDependency(db)
				app.AddDependency(externalApp)
				return component.Components{app, db, vpc}
			},
		},
		{
			name:      "discovery with external dependencies",
			discovery: discovery.NewDiscovery(internalDir).WithDiscoverDependencies().WithDiscoverExternalDependencies(),
			setupExpected: func() component.Components {
				vpc := component.NewUnit(vpcDir)
				db := component.NewUnit(dbDir)
				db.AddDependency(vpc)
				externalApp := component.NewUnit(externalAppDir)
				externalApp.SetExternal()
				app := component.NewUnit(appDir)
				app.AddDependency(db)
				app.AddDependency(externalApp)
				return component.Components{app, db, vpc, externalApp}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			components, err := tt.discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			if tt.errorExpected {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Build expected results
			wantDiscovery := tt.setupExpected()

			// nil out the parsed configurations, as it doesn't matter for this test
			for _, c := range components {
				if unit, ok := c.(*component.Unit); ok {
					unit.StoreConfig(nil)
				}
			}

			// Compare basic component properties
			require.Len(t, components, len(wantDiscovery))

			components = components.Sort()
			wantDiscovery = wantDiscovery.Sort()

			for i, c := range components {
				want := wantDiscovery[i]
				assert.Equal(t, evalPath(want.Path()), evalPath(c.Path()), "Component path mismatch at index %d", i)
				assert.Equal(t, want.Kind(), c.Kind(), "Component kind mismatch at index %d", i)
				assert.Equal(t, want.External(), c.External(), "Component external flag mismatch at index %d", i)

				// Compare dependencies
				cfgDeps := c.Dependencies().Sort()
				wantDeps := want.Dependencies().Sort()
				require.Len(t, cfgDeps, len(wantDeps), "Dependencies count mismatch for %s", c.Path())

				for j, dep := range cfgDeps {
					wantDep := wantDeps[j]
					assert.Equal(t, evalPath(wantDep.Path()), evalPath(dep.Path()), "Dependency path mismatch at component %d, dependency %d", i, j)
					assert.Equal(t, wantDep.Kind(), dep.Kind(), "Dependency kind mismatch at component %d, dependency %d", i, j)
					assert.Equal(t, wantDep.External(), dep.External(), "Dependency external flag mismatch at component %d, dependency %d", i, j)

					// Compare nested dependencies (one level deep)
					depDeps := dep.Dependencies().Sort()
					wantDepDeps := wantDep.Dependencies().Sort()
					require.Len(t, depDeps, len(wantDepDeps), "Nested dependencies count mismatch for %s -> %s", c.Path(), dep.Path())

					for k, nestedDep := range depDeps {
						wantNestedDep := wantDepDeps[k]
						assert.Equal(t, evalPath(wantNestedDep.Path()), evalPath(nestedDep.Path()), "Nested dependency path mismatch")
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

	components, err := d.Discover(t.Context(), l, tgOpts)
	require.NoError(t, err)

	// Verify we found all configurations
	assert.Len(t, components, 3)

	// Helper to find config by path
	findUnit := func(path string) *component.Unit {
		for _, c := range components {
			if filepath.Base(c.Path()) == path {
				if unit, ok := c.(*component.Unit); ok {
					return unit
				}
			}
		}

		return nil
	}

	// Verify exclude configurations were parsed correctly
	unit1 := findUnit("unit1")
	require.NotNil(t, unit1)
	require.NotNil(t, unit1.Config())
	require.NotNil(t, unit1.Config().Exclude)
	assert.Contains(t, unit1.Config().Exclude.Actions, "plan")

	unit2 := findUnit("unit2")
	require.NotNil(t, unit2)
	require.NotNil(t, unit2.Config())
	require.NotNil(t, unit2.Config().Exclude)
	assert.Contains(t, unit2.Config().Exclude.Actions, "apply")

	unit3 := findUnit("unit3")
	require.NotNil(t, unit3)

	if unit3.Config() != nil {
		assert.Nil(t, unit3.Config().Exclude)
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

	units := configs.Filter(component.UnitKind).Paths()
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
	units := configs.Filter(component.UnitKind)
	stacks := configs.Filter(component.StackKind)

	assert.Len(t, units, 1)
	assert.Len(t, stacks, 1)

	// Verify that stack configuration is not parsed (Config should be nil)
	stackComp := stacks[0]
	stack, ok := stackComp.(*component.Stack)
	require.True(t, ok, "should be a Stack")
	assert.Nil(t, stack.Config(), "Stack configuration should not be parsed")

	// Verify that unit configuration is parsed (Config should not be nil)
	unitComp := units[0]
	unit, ok := unitComp.(*component.Unit)
	require.True(t, ok, "should be a Unit")
	assert.NotNil(t, unit.Config(), "Unit configuration should be parsed")
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
	assert.ElementsMatch(t, []string{unit1Dir, unit3Dir}, cfgs.Filter(component.UnitKind).Paths())

	// Exclude-by-default and include only unit1
	d = discovery.NewDiscovery(tmpDir).WithExcludeByDefault().WithIncludeDirs([]string{unit1Dir})
	cfgs, err = d.Discover(t.Context(), l, opts)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{unit1Dir}, cfgs.Filter(component.UnitKind).Paths())

	// Strict include behaves the same
	d = discovery.NewDiscovery(tmpDir).WithStrictInclude().WithIncludeDirs([]string{unit3Dir})
	cfgs, err = d.Discover(t.Context(), l, opts)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{unit3Dir}, cfgs.Filter(component.UnitKind).Paths())
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

	d := discovery.NewDiscovery(tmpDir).WithIncludeDirs([]string{filepath.Join(tmpDir, ".hidden", "**")})
	cfgs, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{hiddenUnitDir}, cfgs.Filter(component.UnitKind).Paths())
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

	d := discovery.NewDiscovery(tmpDir)
	cfgs, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)
	assert.Contains(t, cfgs.Filter(component.UnitKind).Paths(), stackHiddenDir)
}

func TestDiscoveryIgnoreExternalDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

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

	d := discovery.NewDiscovery(internalDir).WithDiscoverDependencies()
	components, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	// Find app config and assert it only has internal deps
	var appCfg *component.Unit

	for _, c := range components {
		if c.Path() == appDir {
			if unit, ok := c.(*component.Unit); ok {
				appCfg = unit
			}

			break
		}
	}

	require.NotNil(t, appCfg)
	depPaths := appCfg.Dependencies().Paths()
	assert.Contains(t, depPaths, dbDir)
	assert.Contains(t, depPaths, extApp)
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
	components, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	// Find the app component
	var appComponent *component.Unit

	for _, c := range components {
		if c.Path() == appDir {
			if unit, ok := c.(*component.Unit); ok {
				appComponent = unit
			}

			break
		}
	}

	require.NotNil(t, appComponent, "app component should be discovered")
	require.NotNil(t, appComponent.Reading(), "Reading field should be initialized")

	// Verify Reading field contains the files that were read
	require.NotEmpty(t, appComponent.Reading(), "should have read files")
	assert.Contains(t, appComponent.Reading(), sharedHCL, "should contain shared.hcl")
	assert.Contains(t, appComponent.Reading(), sharedTFVars, "should contain shared.tfvars")
}

func TestDiscoveryExcludesByDefaultWhenFilterFlagIsEnabled(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	unit1Dir := filepath.Join(tmpDir, "unit1")
	require.NoError(t, os.MkdirAll(unit1Dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unit1Dir, "terragrunt.hcl"), []byte(""), 0644))

	unit2Dir := filepath.Join(tmpDir, "unit2")
	require.NoError(t, os.MkdirAll(unit2Dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unit2Dir, "terragrunt.hcl"), []byte(""), 0644))

	unit3Dir := filepath.Join(tmpDir, "unit3")
	require.NoError(t, os.MkdirAll(unit3Dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unit3Dir, "terragrunt.hcl"), []byte(""), 0644))

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir

	l := logger.CreateLogger()

	tt := []struct {
		name    string
		filters []string
		want    []string
	}{
		{
			name:    "include by default",
			filters: []string{},
			want:    []string{unit1Dir, unit2Dir, unit3Dir},
		},
		{
			name:    "exclude by default",
			filters: []string{"unit1"},
			want:    []string{unit1Dir},
		},
		{
			name:    "include by default when only negative filters are present",
			filters: []string{"!unit2"},
			want:    []string{unit1Dir, unit3Dir},
		},
		{
			name:    "exclude by default when positive and negative filters are present",
			filters: []string{"unit1", "!unit2"},
			want:    []string{unit1Dir},
		},
	}

	for _, tt := range tt {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(tt.filters)
			require.NoError(t, err)

			d := discovery.NewDiscovery(tmpDir).WithFilters(filters)
			components, err := d.Discover(t.Context(), l, opts)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, components.Filter(component.UnitKind).Paths())
		})
	}
}

func TestDiscoveryOriginalTerragruntConfigPath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	unitDir := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0755))

	// Create a config that uses get_original_terragrunt_dir() in the terraform source
	// This function relies on OriginalTerragruntConfigPath being set correctly
	configPath := filepath.Join(unitDir, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(configPath, []byte(`
terraform {
  source = "${get_original_terragrunt_dir()}/module"
}
`), 0644))

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir
	// Start with a different config path to simulate the scenario where opts is cloned
	opts.TerragruntConfigPath = tmpDir
	opts.OriginalTerragruntConfigPath = tmpDir

	l := logger.CreateLogger()

	// Create a unit component with the directory
	unit := component.NewUnit(unitDir)

	err := discovery.Parse(unit, t.Context(), l, opts, false, nil)
	require.NoError(t, err)

	// Verify that the config was parsed correctly
	require.NotNil(t, unit.Config())
	require.NotNil(t, unit.Config().Terraform)
	require.NotNil(t, unit.Config().Terraform.Source)

	// The key test: verify that get_original_terragrunt_dir() returned the correct directory
	expectedSource := filepath.Join(unitDir, "module")
	require.Equal(t, expectedSource, *unit.Config().Terraform.Source,
		"terraform source should use the correct unit directory from get_original_terragrunt_dir()")
}

func TestDependentDiscovery_NewDependentDiscovery(t *testing.T) {
	t.Parallel()

	components := component.Components{
		component.NewUnit("./app"),
		component.NewUnit("./db"),
	}

	dd := discovery.NewDependentDiscovery(component.NewThreadSafeComponents(components)).WithMaxDepth(10)
	require.NotNil(t, dd)
	// Verify creation doesn't panic - the struct is properly initialized
}

func TestDependentDiscovery_WithStartingComponents(t *testing.T) {
	t.Parallel()

	allComponents := component.Components{
		component.NewUnit("./vpc"),
		component.NewUnit("./db"),
		component.NewUnit("./app"),
	}

	startingComponents := component.Components{
		component.NewUnit("./db"),
	}

	dd := discovery.NewDependentDiscovery(component.NewThreadSafeComponents(allComponents)).WithMaxDepth(10)
	err := dd.Discover(t.Context(), logger.CreateLogger(), startingComponents)
	require.NoError(t, err)
}

func TestDependencyDiscovery_DiscoverAllDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	appDir := filepath.Join(tmpDir, "app")
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")

	testDirs := []string{appDir, vpcDir, dbDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
		// Create empty terragrunt.hcl files
		err = os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(""), 0644)
		require.NoError(t, err)
	}

	allComponents := component.Components{
		component.NewUnit(vpcDir),
		component.NewUnit(dbDir),
		component.NewUnit(appDir),
	}

	startingComponents := component.Components{
		component.NewUnit(appDir),
	}

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	dd := discovery.NewDependencyDiscovery(component.NewThreadSafeComponents(allComponents)).WithMaxDepth(10)

	require.NotNil(t, dd)
	// Verify the method accepts startingComponents as a parameter and doesn't panic
	err := dd.Discover(t.Context(), logger.CreateLogger(), opts, startingComponents)
	require.NoError(t, err)
}

func TestDependencyDiscovery_SelectiveDiscoveryOnlyProcessesStartingComponents(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create dependency graph: vpc -> db -> app
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")
	unrelatedDir := filepath.Join(tmpDir, "unrelated")

	testDirs := []string{vpcDir, dbDir, appDir, unrelatedDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(appDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../db"
}
`,
		filepath.Join(dbDir, "terragrunt.hcl"): `
dependency "vpc" {
	config_path = "../vpc"
}
`,
		filepath.Join(vpcDir, "terragrunt.hcl"):       ``,
		filepath.Join(unrelatedDir, "terragrunt.hcl"): ``,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Discover all components first
	allDiscovery := discovery.NewDiscovery(tmpDir)
	allComponents, err := allDiscovery.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	// Find the app component (starting component)
	var startingComponent component.Component

	for _, c := range allComponents {
		if c.Path() == appDir {
			startingComponent = c
			break
		}
	}

	require.NotNil(t, startingComponent)

	// Create dependency discovery with only app as starting component
	dependencyDiscovery := discovery.NewDependencyDiscovery(component.NewThreadSafeComponents(allComponents)).WithMaxDepth(100)

	// Discover dependencies starting from app only
	err = dependencyDiscovery.Discover(t.Context(), logger.CreateLogger(), opts, component.Components{startingComponent})
	require.NoError(t, err)

	// Verify that app component now has its dependencies
	// We need to check the actual component's dependencies
	var appComponent component.Component

	for _, c := range allComponents {
		if c.Path() == appDir {
			appComponent = c
			break
		}
	}

	require.NotNil(t, appComponent)

	// Verify app has db as a dependency
	dependencies := appComponent.Dependencies()
	dependencyPaths := dependencies.Paths()
	assert.Contains(t, dependencyPaths, dbDir, "App should have db as a dependency")

	// Verify db has vpc as a dependency
	var dbComponent component.Component

	for _, c := range allComponents {
		if c.Path() == dbDir {
			dbComponent = c
			break
		}
	}

	require.NotNil(t, dbComponent)
	dbDependencies := dbComponent.Dependencies()
	dbDependencyPaths := dbDependencies.Paths()
	assert.Contains(t, dbDependencyPaths, vpcDir, "Db should have vpc as a dependency")
}

func TestDiscoveryDetectsCycle(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	fooDir := filepath.Join(tmpDir, "foo")
	barDir := filepath.Join(tmpDir, "bar")

	testDirs := []string{fooDir, barDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create terragrunt.hcl files with mutual dependencies
	testFiles := map[string]string{
		filepath.Join(fooDir, "terragrunt.hcl"): `
dependency "bar" {
	config_path = "../bar"
}
`,
		filepath.Join(barDir, "terragrunt.hcl"): `
dependency "foo" {
	config_path = "../foo"
}
`,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	l := logger.CreateLogger()

	// Discover components with dependency discovery enabled
	d := discovery.NewDiscovery(tmpDir).WithDiscoverDependencies()
	components, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err, "Discovery should complete even with cycles")

	// Verify that a cycle is detected
	cycleComponent, cycleErr := components.CycleCheck()
	require.Error(t, cycleErr, "Cycle check should detect a cycle between foo and bar")
	assert.Contains(t, cycleErr.Error(), "cycle detected", "Error message should mention cycle")
	assert.NotNil(t, cycleComponent, "Cycle check should return the component that is part of the cycle")

	// Verify both foo and bar are in the discovered components
	componentPaths := components.Paths()
	assert.Contains(t, componentPaths, fooDir, "Foo should be discovered")
	assert.Contains(t, componentPaths, barDir, "Bar should be discovered")
}

func TestDiscoverWithModulesThatIncludeDoesNotDropConfigs(t *testing.T) {
	t.Parallel()

	workingDir := filepath.Join("..", "..", "test", "fixtures", "include-runall")

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(workingDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.ModulesThatInclude = []string{"alpha.hcl"}
	opts.ExcludeByDefault = true

	d := discovery.NewDiscovery(workingDir).
		WithDiscoverDependencies().
		WithParseInclude().
		WithParseExclude().
		WithDiscoverExternalDependencies().
		WithReadFiles()

	configs, err := d.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	assert.NotEmpty(t, configs, "discovery should return configs even when exclude-by-default is set via modules-that-include")
}

func TestDiscoveryDoesntDetectCycleWhenDisabled(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	fooDir := filepath.Join(tmpDir, "foo")
	barDir := filepath.Join(tmpDir, "bar")

	testDirs := []string{fooDir, barDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create terragrunt.hcl files with mutual dependencies
	testFiles := map[string]string{
		filepath.Join(fooDir, "terragrunt.hcl"): `
dependency "bar" {
	config_path = "../bar"

	enabled = false
}
`,
		filepath.Join(barDir, "terragrunt.hcl"): `
dependency "foo" {
	config_path = "../foo"
}
`,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	l := logger.CreateLogger()

	// Discover components with dependency discovery enabled
	d := discovery.NewDiscovery(tmpDir).WithDiscoverDependencies()
	components, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err, "Discovery should complete even with cycles")

	// Verify that a cycle is detected
	_, cycleErr := components.CycleCheck()
	require.NoError(t, cycleErr, "Cycle check should not detect a cycle between foo and bar")

	// Verify both foo and bar are in the discovered components
	componentPaths := components.Paths()
	assert.Contains(t, componentPaths, fooDir, "Foo should be discovered")
	assert.Contains(t, componentPaths, barDir, "Bar should be discovered")
}

func evalPath(p string) string {
	resolved, _ := filepath.EvalSymlinks(p)
	if resolved == "" {
		return p
	}

	return resolved
}
