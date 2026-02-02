package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscovery_GraphExpressionFilters tests graph expression filter functionality.
func TestDiscovery_GraphExpressionFilters(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// To speed up this test, make the temporary directory a git repository.
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	// Create dependency graph: vpc -> db -> app
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")

	testDirs := []string{vpcDir, dbDir, appDir}
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
`,
		filepath.Join(dbDir, "terragrunt.hcl"): `
dependency "vpc" {
	config_path = "../vpc"
}
`,
		filepath.Join(vpcDir, "terragrunt.hcl"): ``,
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

	tests := []struct {
		name          string
		filterQueries []string
		wantUnits     []string
	}{
		{
			name:          "dependency discovery - app...",
			filterQueries: []string{"app..."},
			wantUnits:     []string{appDir, dbDir, vpcDir},
		},
		{
			name:          "braced path with dependencies - {./app}...",
			filterQueries: []string{"{./app}..."},
			wantUnits:     []string{appDir, dbDir, vpcDir},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(l, tt.filterQueries)
			require.NoError(t, err)

			d := discovery.NewDiscovery(tmpDir).
				WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
				WithFilters(filters)

			components, err := d.Discover(ctx, l, opts)
			require.NoError(t, err)

			units := components.Filter(component.UnitKind).Paths()
			assert.ElementsMatch(t, tt.wantUnits, units, "Units mismatch for test: %s", tt.name)
		})
	}
}

// TestDiscovery_GraphExpressionFilters_ComplexGraph tests graph expressions with a more complex graph.
func TestDiscovery_GraphExpressionFilters_ComplexGraph(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// To speed up this test, make the temporary directory a git repository.
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	// Create complex graph: vpc -> [db, cache] -> app
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	cacheDir := filepath.Join(tmpDir, "cache")
	appDir := filepath.Join(tmpDir, "app")

	testDirs := []string{vpcDir, dbDir, cacheDir, appDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(appDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../db"
}

dependency "cache" {
	config_path = "../cache"
}
`,
		filepath.Join(dbDir, "terragrunt.hcl"): `
dependency "vpc" {
	config_path = "../vpc"
}
`,
		filepath.Join(cacheDir, "terragrunt.hcl"): `
dependency "vpc" {
	config_path = "../vpc"
}
`,
		filepath.Join(vpcDir, "terragrunt.hcl"): ``,
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

	t.Run("dependency traversal from app finds all dependencies", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(l, []string{"app..."})
		require.NoError(t, err)

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithFilters(filters)

		configs, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)

		units := configs.Filter(component.UnitKind).Paths()
		assert.ElementsMatch(t, []string{appDir, dbDir, cacheDir, vpcDir}, units)
	})
}

// TestDiscovery_GraphExpressionFilters_OnlyMatchingComponentsTriggerDiscovery tests selective graph discovery.
func TestDiscovery_GraphExpressionFilters_OnlyMatchingComponentsTriggerDiscovery(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create components: app depends on db, but there's also an unrelated component
	appDir := filepath.Join(tmpDir, "app")
	dbDir := filepath.Join(tmpDir, "db")
	unrelatedDir := filepath.Join(tmpDir, "unrelated")

	testDirs := []string{appDir, dbDir, unrelatedDir}
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
		filepath.Join(dbDir, "terragrunt.hcl"):        ``,
		filepath.Join(unrelatedDir, "terragrunt.hcl"): ``,
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

	t.Run("graph expression only discovers dependencies of matching component", func(t *testing.T) {
		t.Parallel()

		// Filter for app and its dependencies - unrelated should not be included
		filters, err := filter.ParseFilterQueries(l, []string{"app..."})
		require.NoError(t, err)

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithFilters(filters)

		configs, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)

		units := configs.Filter(component.UnitKind).Paths()
		// Should include app and db, but NOT unrelated
		assert.ElementsMatch(t, []string{appDir, dbDir}, units)
		assert.NotContains(t, units, unrelatedDir)
	})
}

// TestDiscovery_GraphExpressionFilters_FiltersAppliedAfterDiscovery tests additional filters after graph discovery.
func TestDiscovery_GraphExpressionFilters_FiltersAppliedAfterDiscovery(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create dependency graph: vpc -> db -> app
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")

	testDirs := []string{vpcDir, dbDir, appDir}
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
		filepath.Join(vpcDir, "terragrunt.hcl"): ``,
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

	t.Run("additional filters applied after graph discovery", func(t *testing.T) {
		t.Parallel()

		// Graph expression discovers app and its dependencies, then additional filter excludes vpc
		filters, err := filter.ParseFilterQueries(l, []string{"app...", "!vpc"})
		require.NoError(t, err)

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithFilters(filters)

		configs, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)

		units := configs.Filter(component.UnitKind).Paths()
		// Should include app and db (from graph), but exclude vpc (from filter)
		assert.ElementsMatch(t, []string{appDir, dbDir}, units)
		assert.NotContains(t, units, vpcDir)
	})
}

// TestDiscovery_ReadingAttributeFilters tests reading attribute filter functionality.
func TestDiscovery_ReadingAttributeFilters(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create shared configuration files
	sharedHCL := filepath.Join(tmpDir, "shared.hcl")
	sharedTFVars := filepath.Join(tmpDir, "shared.tfvars")
	commonVars := filepath.Join(tmpDir, "common", "variables.hcl")
	dbConfig := filepath.Join(tmpDir, "database.yaml")

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "common"), 0755))

	require.NoError(t, os.WriteFile(sharedHCL, []byte(`
locals {
	common_value = "test"
}
`), 0644))

	require.NoError(t, os.WriteFile(sharedTFVars, []byte(`
test_var = "value"
another_var = "test"
`), 0644))

	require.NoError(t, os.WriteFile(commonVars, []byte(`
locals {
	vpc_cidr = "10.0.0.0/16"
}
`), 0644))

	require.NoError(t, os.WriteFile(dbConfig, []byte(`
locals {
	db_host = "localhost"
	db_port = 5432
}
`), 0644))

	// Create test components with different file reads
	frontendDir := filepath.Join(tmpDir, "apps", "frontend")
	backendDir := filepath.Join(tmpDir, "apps", "backend")
	legacyDir := filepath.Join(tmpDir, "apps", "legacy")
	dbDir := filepath.Join(tmpDir, "libs", "db")
	cacheDir := filepath.Join(tmpDir, "libs", "cache")

	testDirs := []string{frontendDir, backendDir, legacyDir, dbDir, cacheDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create test files with different file reading patterns
	// Note: Only read_terragrunt_config and read_tfvars_file populate the Reading slice
	testFiles := map[string]string{
		filepath.Join(frontendDir, "terragrunt.hcl"): `
locals {
	shared = read_terragrunt_config("../../shared.hcl")
	vars = read_tfvars_file("../../shared.tfvars")
}
`,
		filepath.Join(backendDir, "terragrunt.hcl"): `
locals {
	shared = read_terragrunt_config("../../shared.hcl")
	common = read_terragrunt_config("../../common/variables.hcl")
}
`,
		filepath.Join(legacyDir, "terragrunt.hcl"): `
locals {
	# Uses a file that will be tracked
	db_config = read_terragrunt_config("../../database.yaml")
}
`,
		filepath.Join(dbDir, "terragrunt.hcl"): `
locals {
	common = read_terragrunt_config("../../common/variables.hcl")
	db_config = read_terragrunt_config("../../database.yaml")
}
`,
		filepath.Join(cacheDir, "terragrunt.hcl"): `
# No file reads
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

	tests := []struct {
		name          string
		filterQueries []string
		wantUnits     []string
	}{
		{
			name:          "filter by exact file - shared.hcl",
			filterQueries: []string{"reading=shared.hcl"},
			wantUnits:     []string{frontendDir, backendDir},
		},
		{
			name:          "filter by exact file - database.yaml",
			filterQueries: []string{"reading=database.yaml"},
			wantUnits:     []string{legacyDir, dbDir},
		},
		{
			name:          "filter by glob - shared prefix",
			filterQueries: []string{"reading=shared*"},
			wantUnits:     []string{frontendDir, backendDir},
		},
		{
			name:          "filter by exact nested path",
			filterQueries: []string{"reading=common/variables.hcl"},
			wantUnits:     []string{backendDir, dbDir},
		},
		{
			name:          "negation - exclude components reading shared.hcl",
			filterQueries: []string{"!reading=shared.hcl"},
			wantUnits:     []string{legacyDir, dbDir, cacheDir},
		},
		{
			name:          "negation with glob - exclude components reading database.yaml",
			filterQueries: []string{"!reading=database.yaml"},
			wantUnits:     []string{frontendDir, backendDir, cacheDir},
		},
		{
			name:          "intersection - apps directory reading shared.hcl",
			filterQueries: []string{"./apps/* | reading=shared.hcl"},
			wantUnits:     []string{frontendDir, backendDir},
		},
		{
			name:          "intersection - libs directory with common variables",
			filterQueries: []string{"./libs/* | reading=common/variables.hcl"},
			wantUnits:     []string{dbDir},
		},
		{
			name:          "multiple filters - union semantics",
			filterQueries: []string{"reading=shared.hcl", "reading=database.yaml"},
			wantUnits:     []string{frontendDir, backendDir, legacyDir, dbDir},
		},
		{
			name:          "complex - apps not reading database.yaml",
			filterQueries: []string{"./apps/* | !reading=database.yaml"},
			wantUnits:     []string{frontendDir, backendDir},
		},
		{
			name:          "no matches - nonexistent file",
			filterQueries: []string{"reading=nonexistent.hcl"},
			wantUnits:     []string{},
		},
		{
			name:          "components that don't read any files",
			filterQueries: []string{"cache"},
			wantUnits:     []string{cacheDir},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(l, tt.filterQueries)
			require.NoError(t, err)

			d := discovery.NewDiscovery(tmpDir).
				WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
				WithFilters(filters).
				WithReadFiles()

			configs, err := d.Discover(ctx, l, opts)
			require.NoError(t, err)

			units := configs.Filter(component.UnitKind).Paths()
			assert.ElementsMatch(t, tt.wantUnits, units, "Units mismatch for test: %s", tt.name)
		})
	}
}

// TestDiscovery_ReadingAttributeFiltersAbsolutePaths tests reading attribute filter with absolute paths.
func TestDiscovery_ReadingAttributeFiltersAbsolutePaths(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create a shared file with absolute path
	sharedFile := filepath.Join(tmpDir, "shared.hcl")
	require.NoError(t, os.WriteFile(sharedFile, []byte(`
locals {
	value = "test"
}
`), 0644))

	// Create test component
	appDir := filepath.Join(tmpDir, "app")
	require.NoError(t, os.MkdirAll(appDir, 0755))

	terragruntConfig := filepath.Join(appDir, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(terragruntConfig, []byte(`
locals {
	shared = read_terragrunt_config("../shared.hcl")
}
`), 0644))

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
	}

	ctx := t.Context()

	// Test with absolute path filter
	filterQueries := []string{"reading=" + sharedFile}
	filters, err := filter.ParseFilterQueries(l, filterQueries)
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters).
		WithReadFiles()

	configs, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Should find the app component when filtering by absolute path
	units := configs.Filter(component.UnitKind).Paths()
	assert.ElementsMatch(t, []string{appDir}, units, "Should find component by absolute path to read file")
}

// TestDiscovery_ReadingAttributeFiltersErrorHandling tests error handling for invalid reading filters.
func TestDiscovery_ReadingAttributeFiltersErrorHandling(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	appDir := filepath.Join(tmpDir, "app")
	require.NoError(t, os.MkdirAll(appDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(""), 0644))

	tests := []struct {
		name                 string
		filterQueries        []string
		errorExpectedOnParse bool
	}{
		{
			name:                 "invalid glob pattern in reading filter",
			filterQueries:        []string{"reading=[invalid"},
			errorExpectedOnParse: true,
		},
		{
			name:                 "valid reading filter - no error",
			filterQueries:        []string{"reading=*.hcl"},
			errorExpectedOnParse: false,
		},
	}

	l := logger.CreateLogger()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse filter queries
			_, err := filter.ParseFilterQueries(l, tt.filterQueries)
			if tt.errorExpectedOnParse {
				require.Error(t, err, "Expected error for filter: %v", tt.filterQueries)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDiscovery_AttributeFilters tests path, name, type, and external attribute filters.
func TestDiscovery_AttributeFilters(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create test directory structure
	appsDir := filepath.Join(tmpDir, "apps")
	frontendDir := filepath.Join(appsDir, "frontend")
	backendDir := filepath.Join(appsDir, "backend")
	legacyDir := filepath.Join(appsDir, "legacy")

	libsDir := filepath.Join(tmpDir, "libs")
	dbDir := filepath.Join(libsDir, "db")
	cacheDir := filepath.Join(libsDir, "cache")

	stackDir := filepath.Join(tmpDir, "stack")

	testDirs := []string{
		frontendDir,
		backendDir,
		legacyDir,
		dbDir,
		cacheDir,
		stackDir,
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(frontendDir, "terragrunt.hcl"): ``,
		filepath.Join(backendDir, "terragrunt.hcl"):  ``,
		filepath.Join(legacyDir, "terragrunt.hcl"):   ``,
		filepath.Join(dbDir, "terragrunt.hcl"):       ``,
		filepath.Join(cacheDir, "terragrunt.hcl"):    ``,
		filepath.Join(stackDir, "terragrunt.stack.hcl"): `
unit "test" {
  source = "."
  path   = "test"
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

	tests := []struct {
		name          string
		filterQueries []string
		wantUnits     []string
		wantStacks    []string
	}{
		{
			name:          "path filter - apps directory",
			filterQueries: []string{"./apps/*"},
			wantUnits:     []string{frontendDir, backendDir, legacyDir},
			wantStacks:    []string{},
		},
		{
			name:          "path filter with wildcard",
			filterQueries: []string{"./libs/*"},
			wantUnits:     []string{dbDir, cacheDir},
			wantStacks:    []string{},
		},
		{
			name:          "name filter - specific component",
			filterQueries: []string{"frontend"},
			wantUnits:     []string{frontendDir},
			wantStacks:    []string{},
		},
		{
			name:          "name filter with equals",
			filterQueries: []string{"name=backend"},
			wantUnits:     []string{backendDir},
			wantStacks:    []string{},
		},
		{
			name:          "type filter - units only",
			filterQueries: []string{"type=unit"},
			wantUnits:     []string{frontendDir, backendDir, legacyDir, dbDir, cacheDir},
			wantStacks:    []string{},
		},
		{
			name:          "type filter - stacks only",
			filterQueries: []string{"type=stack"},
			wantUnits:     []string{},
			wantStacks:    []string{stackDir},
		},
		{
			name:          "negation filter - exclude legacy",
			filterQueries: []string{"!legacy"},
			wantUnits:     []string{frontendDir, backendDir, dbDir, cacheDir},
			wantStacks:    []string{stackDir},
		},
		{
			name:          "negation filter - exclude apps directory",
			filterQueries: []string{"!./apps/*"},
			wantUnits:     []string{dbDir, cacheDir},
			wantStacks:    []string{stackDir},
		},
		{
			name:          "intersection filter - apps and not legacy",
			filterQueries: []string{"./apps/* | !legacy"},
			wantUnits:     []string{frontendDir, backendDir},
			wantStacks:    []string{},
		},
		{
			name:          "multiple filters - union semantics",
			filterQueries: []string{"./apps/frontend", "./libs/db"},
			wantUnits:     []string{frontendDir, dbDir},
			wantStacks:    []string{},
		},
		{
			name:          "braced path filter",
			filterQueries: []string{"{./apps/*}"},
			wantUnits:     []string{frontendDir, backendDir, legacyDir},
			wantStacks:    []string{},
		},
		{
			name:          "absolute path filter",
			filterQueries: []string{stackDir},
			wantUnits:     []string{},
			wantStacks:    []string{stackDir},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(l, tt.filterQueries)
			require.NoError(t, err)

			d := discovery.NewDiscovery(tmpDir).
				WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
				WithFilters(filters)

			configs, err := d.Discover(ctx, l, opts)
			require.NoError(t, err)

			units := configs.Filter(component.UnitKind).Paths()
			stacks := configs.Filter(component.StackKind).Paths()

			assert.ElementsMatch(t, tt.wantUnits, units, "Units mismatch for test: %s", tt.name)
			assert.ElementsMatch(t, tt.wantStacks, stacks, "Stacks mismatch for test: %s", tt.name)
		})
	}
}

// TestDiscovery_FilterEdgeCases tests edge cases in filter handling.
func TestDiscovery_FilterEdgeCases(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create a single component for edge case testing
	unitDir := filepath.Join(tmpDir, "unit #1")
	err := os.MkdirAll(unitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(""), 0644)
	require.NoError(t, err)

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	tests := []struct {
		name       string
		filters    []string
		wantUnits  []string
		wantStacks []string
	}{
		{
			name:       "filter with spaces in path",
			filters:    []string{"{unit #1}"},
			wantUnits:  []string{unitDir},
			wantStacks: []string{},
		},
		{
			name:       "filter with spaces in name",
			filters:    []string{"unit #1"},
			wantUnits:  []string{unitDir},
			wantStacks: []string{},
		},
		{
			name:       "non-matching filter",
			filters:    []string{"nonexistent"},
			wantUnits:  []string{},
			wantStacks: []string{},
		},
		{
			name:       "non-matching path filter",
			filters:    []string{"./nonexistent/*"},
			wantUnits:  []string{},
			wantStacks: []string{},
		},
		{
			name:       "negation of non-matching filter",
			filters:    []string{"!nonexistent"},
			wantUnits:  []string{unitDir},
			wantStacks: []string{},
		},
		{
			name:       "empty intersection",
			filters:    []string{"unit #1 | nonexistent"},
			wantUnits:  []string{},
			wantStacks: []string{},
		},
		{
			name:       "double negation",
			filters:    []string{"!!unit #1"},
			wantUnits:  []string{unitDir},
			wantStacks: []string{},
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

			configs, err := d.Discover(ctx, l, opts)
			require.NoError(t, err)

			units := configs.Filter(component.UnitKind).Paths()
			stacks := configs.Filter(component.StackKind).Paths()

			assert.ElementsMatch(t, tt.wantUnits, units, "Units mismatch for test: %s", tt.name)
			assert.ElementsMatch(t, tt.wantStacks, stacks, "Stacks mismatch for test: %s", tt.name)
		})
	}
}

// TestDiscovery_FilterErrorHandling tests error handling for invalid filters.
func TestDiscovery_FilterErrorHandling(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	appDir := filepath.Join(tmpDir, "app")
	require.NoError(t, os.MkdirAll(appDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(""), 0644))

	tests := []struct {
		name          string
		filterQueries []string
		errorExpected bool
	}{
		{
			name:          "invalid filter syntax",
			filterQueries: []string{"invalid[filter"},
			errorExpected: true,
		},
		{
			name:          "empty filter query",
			filterQueries: []string{""},
			errorExpected: true,
		},
		{
			name:          "malformed glob pattern",
			filterQueries: []string{"./apps/["},
			errorExpected: true,
		},
		{
			name:          "invalid attribute key",
			filterQueries: []string{"invalid=value"},
			errorExpected: true,
		},
		{
			name:          "invalid type value",
			filterQueries: []string{"type=invalid"},
			errorExpected: true,
		},
		{
			name:          "invalid external value",
			filterQueries: []string{"external=maybe"},
			errorExpected: true,
		},
	}

	l := logger.CreateLogger()
	opts := &options.TerragruntOptions{
		WorkingDir: tmpDir,
	}

	ctx := t.Context()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse filter queries
			filters, err := filter.ParseFilterQueries(l, tt.filterQueries)

			// Some errors occur during parsing (like empty filter), others during evaluation
			if tt.errorExpected && err != nil {
				// Error occurred during parsing - this is expected for some test cases
				return
			}

			require.NoError(t, err) // Parsing should succeed for evaluation error test cases

			// Create discovery with filters
			d := discovery.NewDiscovery(tmpDir).
				WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
				WithFilters(filters)

			// Attempt discovery - errors should occur during evaluation
			_, err = d.Discover(ctx, l, opts)
			if tt.errorExpected {
				require.Error(t, err, "Expected error for filter: %v", tt.filterQueries)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDiscovery_ExternalAttributeFilter tests external attribute filtering.
func TestDiscovery_ExternalAttributeFilter(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create external component outside the working directory to make it truly external
	internalDir := filepath.Join(tmpDir, "internal")
	externalDir := filepath.Join(tmpDir, "external")

	appDir := filepath.Join(internalDir, "app")
	externalAppDir := filepath.Join(externalDir, "app")
	dbDir := filepath.Join(internalDir, "db")

	testDirs := []string{appDir, externalAppDir, dbDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(appDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../db"
}

dependency "external" {
	config_path = "../../external/app"
}
`,
		filepath.Join(dbDir, "terragrunt.hcl"):          ``,
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

	t.Run("external=true filter", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(l, []string{"{./**}... | external=true"})
		require.NoError(t, err)

		d := discovery.NewDiscovery(internalDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: internalDir}).
			WithFilters(filters)

		components, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)

		units := components.Filter(component.UnitKind).Paths()
		assert.ElementsMatch(t, []string{externalAppDir}, units)
	})

	t.Run("external=false filter", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(l, []string{"{./**}... | external=false"})
		require.NoError(t, err)

		d := discovery.NewDiscovery(internalDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: internalDir}).
			WithFilters(filters)

		components, err := d.Discover(ctx, l, opts)
		require.NoError(t, err)

		units := components.Filter(component.UnitKind).Paths()
		assert.ElementsMatch(t, []string{appDir, dbDir}, units)
	})
}

// TestDiscovery_DependentDiscovery_Standalone tests standalone dependent discovery (...vpc).
// This verifies that ...vpc finds all units that depend on vpc.
func TestDiscovery_DependentDiscovery_Standalone(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// To speed up this test, make the temporary directory a git repository.
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	// Create dependency graph: app -> db -> vpc
	// Dependents of vpc: db, app (db depends on vpc, app depends on db which depends on vpc)
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")

	testDirs := []string{vpcDir, dbDir, appDir}
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
		filepath.Join(vpcDir, "terragrunt.hcl"): ``,
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

	// Use ...vpc to find all dependents of vpc
	filters, err := filter.ParseFilterQueries(l, []string{"...vpc"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Should include vpc (target) and db (direct dependent) and app (transitive dependent)
	units := components.Filter(component.UnitKind).Paths()
	assert.ElementsMatch(t, []string{vpcDir, dbDir, appDir}, units, "...vpc should find vpc and all its dependents (db, app)")
}

// TestDiscovery_DependentDiscovery_ExcludeTarget tests dependent discovery with target exclusion (^...vpc).
func TestDiscovery_DependentDiscovery_ExcludeTarget(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// To speed up this test, make the temporary directory a git repository.
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	// Create dependency graph: app -> vpc
	vpcDir := filepath.Join(tmpDir, "vpc")
	appDir := filepath.Join(tmpDir, "app")

	testDirs := []string{vpcDir, appDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(appDir, "terragrunt.hcl"): `
dependency "vpc" {
	config_path = "../vpc"
}
`,
		filepath.Join(vpcDir, "terragrunt.hcl"): ``,
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

	// Use ...^vpc to find dependents but exclude the target (vpc)
	filters, err := filter.ParseFilterQueries(l, []string{"...^vpc"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Should include only app (dependent), not vpc (target is excluded)
	units := components.Filter(component.UnitKind).Paths()
	assert.ElementsMatch(t, []string{appDir}, units, "...^vpc should find only dependents, not the target")
	assert.NotContains(t, units, vpcDir, "vpc should be excluded as the target")
}

// TestDiscovery_DependencyDiscovery_ExcludeTarget tests dependency discovery with target exclusion (^app...).
// This is the inverse of TestDiscovery_DependentDiscovery_ExcludeTarget - it tests excluding the target
// from the dependency direction rather than the dependent direction.
func TestDiscovery_DependencyDiscovery_ExcludeTarget(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// To speed up this test, make the temporary directory a git repository.
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	// Create dependency graph: app -> db -> vpc
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")

	testDirs := []string{vpcDir, dbDir, appDir}
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
		filepath.Join(vpcDir, "terragrunt.hcl"): ``,
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

	// Use ^app... to find dependencies but exclude the target (app)
	filters, err := filter.ParseFilterQueries(l, []string{"^app..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Should include only db and vpc (dependencies), not app (target is excluded)
	units := components.Filter(component.UnitKind).Paths()
	assert.ElementsMatch(t, []string{dbDir, vpcDir}, units, "^app... should find only dependencies, not the target")
	assert.NotContains(t, units, appDir, "app should be excluded as the target")
}

// TestDiscovery_DependentDiscovery_Bidirectional tests bidirectional discovery (...db...).
func TestDiscovery_DependentDiscovery_Bidirectional(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// To speed up this test, make the temporary directory a git repository.
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	// Create dependency graph: app -> db -> vpc
	vpcDir := filepath.Join(tmpDir, "vpc")
	dbDir := filepath.Join(tmpDir, "db")
	appDir := filepath.Join(tmpDir, "app")

	testDirs := []string{vpcDir, dbDir, appDir}
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
		filepath.Join(vpcDir, "terragrunt.hcl"): ``,
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

	// Use ...db... to find both dependencies and dependents of db
	filters, err := filter.ParseFilterQueries(l, []string{"...db..."})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Should include: app (dependent), db (target), vpc (dependency)
	units := components.Filter(component.UnitKind).Paths()
	assert.ElementsMatch(t, []string{appDir, dbDir, vpcDir}, units, "...db... should find dependents, target, and dependencies")
}

// TestDiscovery_DependentDiscovery_OutsideWorkingDir tests that dependent discovery
// finds dependents outside the working directory but within the git root.
// This validates the upward filesystem walking feature.
func TestDiscovery_DependentDiscovery_OutsideWorkingDir(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Initialize git repository at the root
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	// Create structure:
	// /repo (git root)
	// ├── app/
	// │   └── vpc/           <- target
	// │       └── terragrunt.hcl
	// └── other-app/
	//     └── consumer/      <- depends on vpc (outside working dir)
	//         └── terragrunt.hcl
	appDir := filepath.Join(tmpDir, "app")
	vpcDir := filepath.Join(appDir, "vpc")
	otherAppDir := filepath.Join(tmpDir, "other-app")
	consumerDir := filepath.Join(otherAppDir, "consumer")

	testDirs := []string{vpcDir, consumerDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(vpcDir, "terragrunt.hcl"): ``,
		filepath.Join(consumerDir, "terragrunt.hcl"): `
dependency "vpc" {
	config_path = "../../app/vpc"
}
`,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	l := logger.CreateLogger()

	// Set working directory to app/ subdirectory, NOT the git root
	opts := &options.TerragruntOptions{
		WorkingDir:     appDir,
		RootWorkingDir: appDir,
	}

	ctx := t.Context()

	// Use ...vpc to find dependents of vpc
	// consumer is outside the working directory (app/) but should be found
	// via upward filesystem walking bounded by git root
	filters, err := filter.ParseFilterQueries(l, []string{"...vpc"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(appDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: appDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Should include vpc (target) and consumer (dependent outside working dir)
	units := components.Filter(component.UnitKind).Paths()
	assert.Contains(t, units, vpcDir, "vpc should be discovered as the target")
	assert.Contains(t, units, consumerDir, "consumer should be discovered even though it's outside working dir")
	assert.ElementsMatch(t, []string{vpcDir, consumerDir}, units, "...vpc should find vpc and consumer (outside working dir)")
}

// TestDiscovery_DependentDiscovery_OutsideWorkingDir_MultipleLevels tests that dependent discovery
// finds dependents at multiple levels outside the working directory.
func TestDiscovery_DependentDiscovery_OutsideWorkingDir_MultipleLevels(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Initialize git repository at the root
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	// Create structure:
	// /repo (git root)
	// ├── infra/
	// │   └── vpc/              <- target
	// │       └── terragrunt.hcl
	// ├── services/
	// │   └── api/              <- depends on vpc (sibling directory)
	// │       └── terragrunt.hcl
	// └── apps/
	//     └── frontend/         <- depends on api (transitive dependent of vpc)
	//         └── terragrunt.hcl
	infraDir := filepath.Join(tmpDir, "infra")
	vpcDir := filepath.Join(infraDir, "vpc")
	servicesDir := filepath.Join(tmpDir, "services")
	apiDir := filepath.Join(servicesDir, "api")
	appsDir := filepath.Join(tmpDir, "apps")
	frontendDir := filepath.Join(appsDir, "frontend")

	testDirs := []string{vpcDir, apiDir, frontendDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(vpcDir, "terragrunt.hcl"): ``,
		filepath.Join(apiDir, "terragrunt.hcl"): `
dependency "vpc" {
	config_path = "../../infra/vpc"
}
`,
		filepath.Join(frontendDir, "terragrunt.hcl"): `
dependency "api" {
	config_path = "../../services/api"
}
`,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	l := logger.CreateLogger()

	// Set working directory to infra/ subdirectory
	opts := &options.TerragruntOptions{
		WorkingDir:     infraDir,
		RootWorkingDir: infraDir,
	}

	ctx := t.Context()

	// Use ...vpc to find dependents of vpc
	// api and frontend are both outside the working directory (infra/)
	filters, err := filter.ParseFilterQueries(l, []string{"...vpc"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(infraDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: infraDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Should include vpc (target), api (direct dependent), and frontend (transitive dependent)
	units := components.Filter(component.UnitKind).Paths()
	assert.Contains(t, units, vpcDir, "vpc should be discovered as the target")
	assert.Contains(t, units, apiDir, "api should be discovered (direct dependent outside working dir)")
	assert.Contains(t, units, frontendDir, "frontend should be discovered (transitive dependent outside working dir)")
	assert.ElementsMatch(t, []string{vpcDir, apiDir, frontendDir}, units)
}

// TestDiscovery_DependentDiscovery_DirectDependentOnly tests that dependent discovery
// finds direct dependents correctly.
func TestDiscovery_DependentDiscovery_DirectDependentOnly(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// To speed up this test, make the temporary directory a git repository.
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	// Create dependency graph: api -> db, web -> db
	// Both api and web directly depend on db
	dbDir := filepath.Join(tmpDir, "db")
	apiDir := filepath.Join(tmpDir, "api")
	webDir := filepath.Join(tmpDir, "web")
	unrelatedDir := filepath.Join(tmpDir, "unrelated")

	testDirs := []string{dbDir, apiDir, webDir, unrelatedDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	testFiles := map[string]string{
		filepath.Join(apiDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../db"
}
`,
		filepath.Join(webDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../db"
}
`,
		filepath.Join(dbDir, "terragrunt.hcl"):        ``,
		filepath.Join(unrelatedDir, "terragrunt.hcl"): ``,
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

	// Use ...db to find all dependents of db
	filters, err := filter.ParseFilterQueries(l, []string{"...db"})
	require.NoError(t, err)

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	// Should include db (target), api (dependent), web (dependent)
	// Should NOT include unrelated
	units := components.Filter(component.UnitKind).Paths()
	assert.ElementsMatch(t, []string{dbDir, apiDir, webDir}, units, "...db should find db and all its dependents")
	assert.NotContains(t, units, unrelatedDir, "unrelated should not be included")
}
