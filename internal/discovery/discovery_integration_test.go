package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscovery_BasicWithHiddenDirectories tests discovery with and without hidden directories.
func TestDiscovery_BasicWithHiddenDirectories(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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
		name       string
		wantUnits  []string
		wantStacks []string
		noHidden   bool
	}{
		{
			name:       "discovery without hidden",
			noHidden:   true,
			wantUnits:  []string{unit1Dir, unit2Dir, nestedUnit4Dir},
			wantStacks: []string{stack1Dir},
		},
		{
			name:       "discovery with hidden",
			noHidden:   false,
			wantUnits:  []string{unit1Dir, unit2Dir, hiddenUnitDir, nestedUnit4Dir},
			wantStacks: []string{stack1Dir},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			opts := &options.TerragruntOptions{
				WorkingDir: tmpDir,
			}

			ctx := t.Context()

			d := discovery.NewDiscovery(tmpDir).WithDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: tmpDir,
			})

			if tt.noHidden {
				d = d.WithNoHidden()
			}

			components, err := d.Discover(ctx, l, opts)
			require.NoError(t, err)

			units := components.Filter(component.UnitKind).Paths()
			stacks := components.Filter(component.StackKind).Paths()

			assert.ElementsMatch(t, tt.wantUnits, units)
			assert.ElementsMatch(t, tt.wantStacks, stacks)
		})
	}
}

// TestDiscovery_StackHiddenDiscovered tests that .terragrunt-stack directories are discovered by default.
func TestDiscovery_StackHiddenDiscovered(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	stackHiddenDir := filepath.Join(tmpDir, ".terragrunt-stack", "u")
	require.NoError(t, os.MkdirAll(stackHiddenDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stackHiddenDir, "terragrunt.hcl"), []byte(""), 0644))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	// By default, .terragrunt-stack contents should be discovered
	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir})

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)
	assert.Contains(t, components.Filter(component.UnitKind).Paths(), stackHiddenDir)
}

// TestDiscovery_WithDependencies tests dependency discovery and relationship building.
func TestDiscovery_WithDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     internalDir,
		RootWorkingDir: internalDir,
	}

	ctx := t.Context()

	t.Run("discovery with relationships", func(t *testing.T) {
		t.Parallel()

		d := discovery.NewDiscovery(internalDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: internalDir}).
			WithRelationships()

		components, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)

		// Should discover all internal components
		paths := components.Paths()
		assert.Contains(t, paths, appDir)
		assert.Contains(t, paths, dbDir)
		assert.Contains(t, paths, vpcDir)

		// Find app component and verify dependencies
		var appComponent component.Component

		for _, c := range components {
			if c.Path() == appDir {
				appComponent = c
				break
			}
		}

		require.NotNil(t, appComponent, "app component should be discovered")
		depPaths := appComponent.Dependencies().Paths()
		assert.Contains(t, depPaths, dbDir, "app should depend on db")
		assert.Contains(t, depPaths, externalAppDir, "app should depend on external app")

		// Verify db's dependencies
		var dbComponent component.Component

		for _, c := range components {
			if c.Path() == dbDir {
				dbComponent = c
				break
			}
		}

		require.NotNil(t, dbComponent)
		assert.Contains(t, dbComponent.Dependencies().Paths(), vpcDir, "db should depend on vpc")
	})

	t.Run("discovery with dependency graph filter", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
		require.NoError(t, err)

		d := discovery.NewDiscovery(internalDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: internalDir}).
			WithFilters(filters)

		components, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)

		// Should discover all components including external dependency
		paths := components.Paths()
		assert.Contains(t, paths, appDir)
		assert.Contains(t, paths, dbDir)
		assert.Contains(t, paths, vpcDir)
		assert.Contains(t, paths, externalAppDir)

		// Find external app and verify it's marked as external
		for _, c := range components {
			if c.Path() == externalAppDir {
				assert.True(t, c.External(), "external app should be marked as external")
				break
			}
		}
	})
}

// TestDiscovery_CycleDetection tests that cycles in dependency graphs are detected.
func TestDiscovery_CycleDetection(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	fooDir := filepath.Join(tmpDir, "foo")
	barDir := filepath.Join(tmpDir, "bar")

	testDirs := []string{fooDir, barDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create terragrunt.hcl files with mutual dependencies (cycle)
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

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
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

// TestDiscovery_CycleDetectionWithDisabledDependency tests that disabled dependencies don't create cycles.
func TestDiscovery_CycleDetectionWithDisabledDependency(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	fooDir := filepath.Join(tmpDir, "foo")
	barDir := filepath.Join(tmpDir, "bar")

	testDirs := []string{fooDir, barDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create terragrunt.hcl files where one dependency is disabled
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

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err, "Discovery should complete")

	// Verify that a cycle is NOT detected because one dependency is disabled
	_, cycleErr := components.CycleCheck()
	require.NoError(t, cycleErr, "Cycle check should not detect a cycle when dependency is disabled")

	// Verify both foo and bar are in the discovered components
	componentPaths := components.Paths()
	assert.Contains(t, componentPaths, fooDir, "Foo should be discovered")
	assert.Contains(t, componentPaths, barDir, "Bar should be discovered")
}

// TestDiscovery_WithParseExclude tests that WithParseExclude enables parsing of exclude blocks
// and that the exclude configurations are accessible on the discovered units.
func TestDiscovery_WithParseExclude(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	// WithParseExclude sets requiresParse=true which triggers the parse phase,
	// allowing exclude blocks to be parsed and accessible on the units.
	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithParseExclude()

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Verify we found all configurations
	assert.Len(t, components, 3)

	// Helper to find unit by path
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
	require.NotNil(t, unit1.Config(), "unit1 should have a parsed config")
	require.NotNil(t, unit1.Config().Exclude, "unit1 should have an exclude block")
	assert.Contains(t, unit1.Config().Exclude.Actions, "plan", "unit1 exclude should contain 'plan' action")

	unit2 := findUnit("unit2")
	require.NotNil(t, unit2)
	require.NotNil(t, unit2.Config(), "unit2 should have a parsed config")
	require.NotNil(t, unit2.Config().Exclude, "unit2 should have an exclude block")
	assert.Contains(t, unit2.Config().Exclude.Actions, "apply", "unit2 exclude should contain 'apply' action")

	unit3 := findUnit("unit3")
	require.NotNil(t, unit3)
	// unit3 has an empty config, so Config() may be nil or Exclude may be nil
	if unit3.Config() != nil {
		assert.Nil(t, unit3.Config().Exclude, "unit3 should not have an exclude block")
	}
}

// TestDiscovery_WithCustomConfigFilenames tests discovery with custom config filenames.
func TestDiscovery_WithCustomConfigFilenames(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create units with custom config filenames
	unit1Dir := filepath.Join(tmpDir, "unit1")
	unit2Dir := filepath.Join(tmpDir, "unit2")

	require.NoError(t, os.MkdirAll(unit1Dir, 0755))
	require.NoError(t, os.MkdirAll(unit2Dir, 0755))

	// Standard terragrunt.hcl in unit1
	require.NoError(t, os.WriteFile(filepath.Join(unit1Dir, "terragrunt.hcl"), []byte(""), 0644))
	// Custom config in unit2
	require.NoError(t, os.WriteFile(filepath.Join(unit2Dir, "custom.hcl"), []byte(""), 0644))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	t.Run("discover only custom config filename", func(t *testing.T) {
		t.Parallel()

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithConfigFilenames([]string{"custom.hcl"})

		components, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)

		units := components.Filter(component.UnitKind).Paths()
		assert.Len(t, units, 1)
		assert.ElementsMatch(t, []string{unit2Dir}, units)
	})

	t.Run("discover both standard and custom config filenames", func(t *testing.T) {
		t.Parallel()

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithConfigFilenames([]string{"terragrunt.hcl", "custom.hcl"})

		components, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)

		units := components.Filter(component.UnitKind).Paths()
		assert.Len(t, units, 2)
		assert.ElementsMatch(t, []string{unit1Dir, unit2Dir}, units)
	})
}

// TestDiscovery_WithReadFiles tests that reading field is populated when using reading filters.
// The implementation requires a filter that triggers parsing to populate the reading field.
func TestDiscovery_WithReadFiles(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
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

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Use a reading filter to trigger parsing and populate the reading field
	filters, err := filter.ParseFilterQueries(l, []string{"reading=shared.hcl"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters).
		WithReadFiles()

	components, err := d.Discover(ctx, l, opts)
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
}

// TestDiscovery_WithStackConfigParsing tests that stack files are discovered but not parsed as unit configs.
func TestDiscovery_WithStackConfigParsing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	stackDir := filepath.Join(tmpDir, "stack")
	unitDir := filepath.Join(tmpDir, "unit")

	testDirs := []string{stackDir, unitDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create a stack file with unit blocks
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

	testFiles := map[string]string{
		filepath.Join(stackDir, "terragrunt.stack.hcl"): stackContent,
		filepath.Join(unitDir, "terragrunt.hcl"):        unitContent,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Verify that both stack and unit configurations are discovered
	units := components.Filter(component.UnitKind)
	stacks := components.Filter(component.StackKind)

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

// TestDiscovery_IncludeExcludeFilterSemantics tests include/exclude filter behavior.
func TestDiscovery_IncludeExcludeFilterSemantics(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	tests := []struct {
		name    string
		filters []string
		want    []string
	}{
		{
			name:    "include by default (no filters)",
			filters: []string{},
			want:    []string{unit1Dir, unit2Dir, unit3Dir},
		},
		{
			name:    "exclude by default when positive filter",
			filters: []string{"unit1"},
			want:    []string{unit1Dir},
		},
		{
			name:    "include by default with only negative filter",
			filters: []string{"!unit2"},
			want:    []string{unit1Dir, unit3Dir},
		},
		{
			name:    "exclude by default with positive and negative filters",
			filters: []string{"unit1", "!unit2"},
			want:    []string{unit1Dir},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(l, tt.filters)
			require.NoError(t, err)

			d := discovery.NewDiscovery(tmpDir).
				WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
				WithFilters(filters)

			components, err := d.Discover(ctx, l, opts)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, components.Filter(component.UnitKind).Paths())
		})
	}
}

// TestDiscovery_HiddenIncludedByIncludeDirs tests hidden directories are included when explicitly filtered.
func TestDiscovery_HiddenIncludedByIncludeDirs(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	hiddenUnitDir := filepath.Join(tmpDir, ".hidden", "hunit")
	require.NoError(t, os.MkdirAll(hiddenUnitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hiddenUnitDir, "terragrunt.hcl"), []byte(""), 0644))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	filters, err := filter.ParseFilterQueries(l, []string{"./.hidden/**"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{hiddenUnitDir}, components.Filter(component.UnitKind).Paths())
}

// TestDiscovery_ExternalDependencies tests that external dependencies are correctly identified.
func TestDiscovery_ExternalDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

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

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     internalDir,
		RootWorkingDir: internalDir,
	}

	ctx := t.Context()

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(internalDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: internalDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Find app config and assert it has external dependency
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

	// Verify external dependency is marked as external
	for _, dep := range appCfg.Dependencies() {
		if dep.Path() == extApp {
			assert.True(t, dep.External(), "external app should be marked as external")
		}
	}
}

// TestDiscovery_BreakCycles tests that WithBreakCycles removes cyclic components.
func TestDiscovery_BreakCycles(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	fooDir := filepath.Join(tmpDir, "foo")
	barDir := filepath.Join(tmpDir, "bar")

	testDirs := []string{fooDir, barDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create terragrunt.hcl files with mutual dependencies (cycle)
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

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	filters, err := filter.ParseFilterQueries(l, []string{"{./**}..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters).
		WithBreakCycles()

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err, "Discovery should complete with break cycles enabled")

	// With break cycles enabled, the cycle should be resolved (one component removed)
	_, cycleErr := components.CycleCheck()
	require.NoError(t, cycleErr, "Cycle check should not detect a cycle after breaking")
}

// TestDiscovery_WithNumWorkers tests that the worker count can be configured.
func TestDiscovery_WithNumWorkers(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create a few test units
	for i := range 5 {
		dir := filepath.Join(tmpDir, "unit"+string(rune('a'+i)))
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(""), 0644))
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithNumWorkers(2)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)
	assert.Len(t, components, 5)
}

// TestDiscovery_WithMaxDependencyDepth tests dependency depth limiting.
func TestDiscovery_WithMaxDependencyDepth(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create chain: a -> b -> c -> d
	aDir := filepath.Join(tmpDir, "a")
	bDir := filepath.Join(tmpDir, "b")
	cDir := filepath.Join(tmpDir, "c")
	dDir := filepath.Join(tmpDir, "d")

	for _, dir := range []string{aDir, bDir, cDir, dDir} {
		require.NoError(t, os.MkdirAll(dir, 0755))
	}

	testFiles := map[string]string{
		filepath.Join(aDir, "terragrunt.hcl"): `
dependency "b" {
	config_path = "../b"
}
`,
		filepath.Join(bDir, "terragrunt.hcl"): `
dependency "c" {
	config_path = "../c"
}
`,
		filepath.Join(cDir, "terragrunt.hcl"): `
dependency "d" {
	config_path = "../d"
}
`,
		filepath.Join(dDir, "terragrunt.hcl"): ``,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	t.Run("full depth discovers all", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(l, []string{"a..."})
		require.NoError(t, err)

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithFilters(filters).
			WithMaxDependencyDepth(100)

		components, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)

		paths := components.Paths()
		assert.Contains(t, paths, aDir)
		assert.Contains(t, paths, bDir)
		assert.Contains(t, paths, cDir)
		assert.Contains(t, paths, dDir)
	})

	t.Run("limited depth", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(l, []string{"a..."})
		require.NoError(t, err)

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithFilters(filters).
			WithMaxDependencyDepth(1)

		components, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)

		paths := components.Paths()
		assert.Contains(t, paths, aDir, "a should always be included")
		// With depth 1, we should get at least a and b
		assert.Contains(t, paths, bDir, "b should be included with depth 1")
	})
}

// TestDiscovery_SuppressParseErrors tests that parse errors can be suppressed.
func TestDiscovery_SuppressParseErrors(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	validDir := filepath.Join(tmpDir, "valid")
	invalidDir := filepath.Join(tmpDir, "invalid")

	require.NoError(t, os.MkdirAll(validDir, 0755))
	require.NoError(t, os.MkdirAll(invalidDir, 0755))

	// Valid config
	require.NoError(t, os.WriteFile(filepath.Join(validDir, "terragrunt.hcl"), []byte(""), 0644))
	// Invalid config (should cause parse error)
	require.NoError(t, os.WriteFile(filepath.Join(invalidDir, "terragrunt.hcl"), []byte(`
terraform {
  source = undefined_function()
}
`), 0644))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithParseExclude().
		WithSuppressParseErrors()

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err, "Discovery should succeed with suppressed parse errors")

	// Valid config should be discovered
	paths := components.Paths()
	assert.Contains(t, paths, validDir)
}

// TestDiscovery_WithReport tests that WithReport sets the report.
func TestDiscovery_WithReport(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	unitDir := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(""), 0644))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Test that discovery works with a nil report (should not panic)
	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithReport(nil)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)
	assert.Len(t, components, 1)
}

// TestDiscovery_OriginalTerragruntConfigPath tests that get_original_terragrunt_dir() returns the
// correct directory during parsing. This verifies that phase_parse.go correctly sets
// OriginalTerragruntConfigPath when parsing units.
func TestDiscovery_OriginalTerragruntConfigPath(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	appDir := filepath.Join(tmpDir, "app")
	dbDir := filepath.Join(tmpDir, "db")

	require.NoError(t, os.MkdirAll(appDir, 0755))
	require.NoError(t, os.MkdirAll(dbDir, 0755))

	// Create a config that uses get_original_terragrunt_dir() in the terraform source
	// This function relies on OriginalTerragruntConfigPath being set correctly
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(`
terraform {
  source = "${get_original_terragrunt_dir()}/module"
}

dependency "db" {
  config_path = "../db"
}
`), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(dbDir, "terragrunt.hcl"), []byte(""), 0644))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
		// Start with a different config path to simulate the scenario where opts is cloned
		TerragruntConfigPath:         tmpDir,
		OriginalTerragruntConfigPath: tmpDir,
	}

	ctx := t.Context()

	// Use a dependency traversal filter (app...) to trigger parsing
	filters, err := filter.ParseFilterQueries(l, []string{"app..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
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
	require.NotNil(t, appComponent.Config(), "app config should be parsed")
	require.NotNil(t, appComponent.Config().Terraform, "terraform block should be parsed")
	require.NotNil(t, appComponent.Config().Terraform.Source, "terraform source should be parsed")

	// The key test: verify that get_original_terragrunt_dir() returned the correct directory
	// It should resolve to the app unit's directory, not the initial opts value (tmpDir)
	expectedSource := filepath.Join(appDir, "module")
	assert.Equal(t, expectedSource, *appComponent.Config().Terraform.Source,
		"terraform source should use the correct unit directory from get_original_terragrunt_dir()")
}
