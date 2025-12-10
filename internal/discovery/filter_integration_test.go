package discovery_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func TestDiscoveryWithFilters(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Create test directory structure
	appsDir := filepath.Join(tmpDir, "apps")
	frontendDir := filepath.Join(appsDir, "frontend")
	backendDir := filepath.Join(appsDir, "backend")
	legacyDir := filepath.Join(appsDir, "legacy")

	libsDir := filepath.Join(tmpDir, "libs")
	dbDir := filepath.Join(libsDir, "db")
	cacheDir := filepath.Join(libsDir, "cache")

	stackDir := filepath.Join(tmpDir, "stack")
	// Create external component outside the working directory to make it truly external
	externalDir := filepath.Join(filepath.Dir(tmpDir), "external")
	externalAppDir := filepath.Join(externalDir, "app")

	testDirs := []string{
		frontendDir,
		backendDir,
		legacyDir,
		dbDir,
		cacheDir,
		stackDir,
		externalAppDir,
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Clean up external directory after test
	t.Cleanup(func() {
		os.RemoveAll(externalDir)
	})

	// Create test files
	testFiles := map[string]string{
		filepath.Join(frontendDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../../libs/db"
}

dependency "external" {
	config_path = "../../../external/app"
}
`,
		filepath.Join(backendDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../../libs/db"
}

dependency "cache" {
	config_path = "../../libs/cache"
}
`,
		filepath.Join(legacyDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../../libs/db"
}
`,
		filepath.Join(dbDir, "terragrunt.hcl"):          ``,
		filepath.Join(cacheDir, "terragrunt.hcl"):       ``,
		filepath.Join(stackDir, "terragrunt.stack.hcl"): ``,
		filepath.Join(externalAppDir, "terragrunt.hcl"): ``,
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Create base options
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	tests := []struct {
		name          string
		filterQueries []string
		discoveryOpts []discovery.DiscoveryOption
		wantUnits     []string
		wantStacks    []string
		errorExpected bool
	}{
		{
			name:          "no filters - should return all components",
			filterQueries: []string{},
			wantUnits:     []string{frontendDir, backendDir, legacyDir, dbDir, cacheDir},
			wantStacks:    []string{stackDir},
		},
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
			name:          "external filter - external components",
			filterQueries: []string{"{./**}... | external=true"},
			wantUnits:     []string{externalAppDir},
			wantStacks:    []string{},
		},
		{
			name:          "external filter - internal components",
			filterQueries: []string{"{./**}... | external=false"},
			wantUnits:     []string{frontendDir, backendDir, legacyDir, dbDir, cacheDir},
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
			name:          "intersection filter - libs and type unit",
			filterQueries: []string{"./libs/* | type=unit"},
			wantUnits:     []string{dbDir, cacheDir},
			wantStacks:    []string{},
		},
		{
			name:          "multiple filters - union semantics",
			filterQueries: []string{"./apps/frontend", "./libs/db"},
			wantUnits:     []string{frontendDir, dbDir},
			wantStacks:    []string{},
		},
		{
			name:          "multiple filters with negation",
			filterQueries: []string{"./apps/*", "!legacy"},
			wantUnits:     []string{frontendDir, backendDir},
			wantStacks:    []string{},
		},
		{
			name:          "complex filter - apps not legacy and not external",
			filterQueries: []string{"./apps/* | !legacy | !external=true"},
			wantUnits:     []string{frontendDir, backendDir},
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

			// Parse filter queries
			filters, err := filter.ParseFilterQueries(tt.filterQueries)
			if tt.errorExpected {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Create discovery with filters
			discovery := discovery.NewDiscovery(tmpDir, tt.discoveryOpts...).WithFilters(filters)

			// Discover components with dependencies to test external filtering
			discovery = discovery.
				WithDiscoverDependencies().
				WithDiscoverExternalDependencies()

			configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			// Filter results by type
			units := configs.Filter(component.UnitKind).Paths()
			stacks := configs.Filter(component.StackKind).Paths()

			// Verify results
			assert.ElementsMatch(t, tt.wantUnits, units, "Units mismatch for test: %s", tt.name)
			assert.ElementsMatch(t, tt.wantStacks, stacks, "Stacks mismatch for test: %s", tt.name)
		})
	}
}

func TestDiscoveryWithFiltersErrorHandling(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	opts, err := options.NewTerragruntOptionsForTest(tmpDir)
	require.NoError(t, err)

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse filter queries
			filters, err := filter.ParseFilterQueries(tt.filterQueries)

			// Some errors occur during parsing (like empty filter), others during evaluation
			if tt.errorExpected && err != nil {
				// Error occurred during parsing - this is expected for some test cases
				return
			}

			require.NoError(t, err) // Parsing should succeed for evaluation error test cases

			// Create discovery with filters
			discovery := discovery.NewDiscovery(tmpDir).WithFilters(filters)

			// Attempt discovery - errors should occur during evaluation
			_, err = discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			if tt.errorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDiscoveryWithFiltersEdgeCases(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Create a single component for edge case testing
	unitDir := filepath.Join(tmpDir, "unit #1")
	err = os.MkdirAll(unitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(""), 0644)
	require.NoError(t, err)

	opts, err := options.NewTerragruntOptionsForTest(tmpDir)
	require.NoError(t, err)

	tests := []struct {
		name          string
		filterQueries []string
		wantUnits     []string
		wantStacks    []string
	}{
		{
			name:          "filter with spaces in path",
			filterQueries: []string{"{unit #1}"},
			wantUnits:     []string{unitDir},
			wantStacks:    []string{},
		},
		{
			name:          "filter with spaces in name",
			filterQueries: []string{"unit #1"},
			wantUnits:     []string{unitDir},
			wantStacks:    []string{},
		},
		{
			name:          "non-matching filter",
			filterQueries: []string{"nonexistent"},
			wantUnits:     []string{},
			wantStacks:    []string{},
		},
		{
			name:          "non-matching path filter",
			filterQueries: []string{"./nonexistent/*"},
			wantUnits:     []string{},
			wantStacks:    []string{},
		},
		{
			name:          "negation of non-matching filter",
			filterQueries: []string{"!nonexistent"},
			wantUnits:     []string{unitDir},
			wantStacks:    []string{},
		},
		{
			name:          "double negation",
			filterQueries: []string{"!!unit #1"},
			wantUnits:     []string{unitDir},
			wantStacks:    []string{},
		},
		{
			name:          "empty intersection",
			filterQueries: []string{"unit #1 | nonexistent"},
			wantUnits:     []string{},
			wantStacks:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse filter queries
			filters, err := filter.ParseFilterQueries(tt.filterQueries)
			require.NoError(t, err)

			// Create discovery with filters
			discovery := discovery.NewDiscovery(tmpDir).
				WithFilters(filters).
				WithDiscoverExternalDependencies()

			configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			// Filter results by type
			units := configs.Filter(component.UnitKind).Paths()
			stacks := configs.Filter(component.StackKind).Paths()

			// Verify results
			assert.ElementsMatch(t, tt.wantUnits, units, "Units mismatch for test: %s", tt.name)
			assert.ElementsMatch(t, tt.wantStacks, stacks, "Stacks mismatch for test: %s", tt.name)
		})
	}
}

func TestDiscoveryWithReadingFilters(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

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

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	tests := []struct {
		name          string
		filterQueries []string
		wantUnits     []string
		wantStacks    []string
	}{
		{
			name:          "filter by exact file - shared.hcl",
			filterQueries: []string{"reading=shared.hcl"},
			wantUnits:     []string{frontendDir, backendDir},
			wantStacks:    []string{},
		},
		{
			name:          "filter by exact file - database.yaml",
			filterQueries: []string{"reading=database.yaml"},
			wantUnits:     []string{legacyDir, dbDir},
			wantStacks:    []string{},
		},
		{
			name:          "filter by glob - shared prefix",
			filterQueries: []string{"reading=shared*"},
			wantUnits:     []string{frontendDir, backendDir},
			wantStacks:    []string{},
		},
		{
			name:          "filter by exact nested path",
			filterQueries: []string{"reading=common/variables.hcl"},
			wantUnits:     []string{backendDir, dbDir},
			wantStacks:    []string{},
		},
		{
			name:          "filter by glob - database yaml file",
			filterQueries: []string{"reading=database.yaml"},
			wantUnits:     []string{legacyDir, dbDir},
			wantStacks:    []string{},
		},
		{
			name:          "negation - exclude components reading shared.hcl",
			filterQueries: []string{"!reading=shared.hcl"},
			wantUnits:     []string{legacyDir, dbDir, cacheDir},
			wantStacks:    []string{},
		},
		{
			name:          "negation with glob - exclude components reading database.yaml",
			filterQueries: []string{"!reading=database.yaml"},
			wantUnits:     []string{frontendDir, backendDir, cacheDir},
			wantStacks:    []string{},
		},
		{
			name:          "intersection - apps directory reading shared.hcl",
			filterQueries: []string{"./apps/* | reading=shared.hcl"},
			wantUnits:     []string{frontendDir, backendDir},
			wantStacks:    []string{},
		},
		{
			name:          "intersection - libs directory with common variables",
			filterQueries: []string{"./libs/* | reading=common/variables.hcl"},
			wantUnits:     []string{dbDir},
			wantStacks:    []string{},
		},
		{
			name:          "multiple filters - union semantics",
			filterQueries: []string{"reading=shared.hcl", "reading=database.yaml"},
			wantUnits:     []string{frontendDir, backendDir, legacyDir, dbDir},
			wantStacks:    []string{},
		},
		{
			name:          "complex - apps not reading database.yaml",
			filterQueries: []string{"./apps/* | !reading=database.yaml"},
			wantUnits:     []string{frontendDir, backendDir},
			wantStacks:    []string{},
		},
		{
			name:          "no matches - nonexistent file",
			filterQueries: []string{"reading=nonexistent.hcl"},
			wantUnits:     []string{},
			wantStacks:    []string{},
		},
		{
			name:          "components that don't read any files",
			filterQueries: []string{"cache"},
			wantUnits:     []string{cacheDir},
			wantStacks:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse filter queries
			filters, err := filter.ParseFilterQueries(tt.filterQueries)
			require.NoError(t, err)

			// Create discovery with filters and ReadFiles enabled
			discovery := discovery.NewDiscovery(tmpDir).
				WithFilters(filters).
				WithReadFiles()

			configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			// Filter results by type
			units := configs.Filter(component.UnitKind).Paths()
			stacks := configs.Filter(component.StackKind).Paths()

			// Verify results
			assert.ElementsMatch(t, tt.wantUnits, units, "Units mismatch for test: %s", tt.name)
			assert.ElementsMatch(t, tt.wantStacks, stacks, "Stacks mismatch for test: %s", tt.name)
		})
	}
}

func TestDiscoveryWithReadingFiltersAndAbsolutePaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

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

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Test with absolute path filter
	filterQueries := []string{"reading=" + sharedFile}
	filters, err := filter.ParseFilterQueries(filterQueries)
	require.NoError(t, err)

	discovery := discovery.NewDiscovery(tmpDir).
		WithFilters(filters).
		WithReadFiles()

	configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	// Should find the app component when filtering by absolute path
	units := configs.Filter(component.UnitKind).Paths()
	assert.ElementsMatch(t, []string{appDir}, units, "Should find component by absolute path to read file")
}

func TestDiscoveryWithReadingFiltersErrorHandling(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	appDir := filepath.Join(tmpDir, "app")
	require.NoError(t, os.MkdirAll(appDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(""), 0644))

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	tests := []struct {
		name          string
		filterQueries []string
		errorExpected bool
	}{
		{
			name:          "invalid glob pattern in reading filter",
			filterQueries: []string{"reading=[invalid"},
			errorExpected: true,
		},
		{
			name:          "valid reading filter - no error",
			filterQueries: []string{"reading=*.hcl"},
			errorExpected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse filter queries
			filters, err := filter.ParseFilterQueries(tt.filterQueries)

			// Some errors occur during parsing
			if tt.errorExpected && err != nil {
				return
			}

			require.NoError(t, err)

			// Create discovery with filters
			discovery := discovery.NewDiscovery(tmpDir).
				WithFilters(filters).
				WithReadFiles()

			// Attempt discovery
			_, err = discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			if tt.errorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDiscoveryWithGraphExpressionFilters(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// To speed up this test, we'll make the temporary directory a git repository.
	// This creates a lower upper bound for dependent discovery.
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

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	tests := []struct {
		name          string
		filterQueries []string
		wantUnits     []string
	}{
		{
			name:          "selective dependency discovery - app...",
			filterQueries: []string{"app..."},
			wantUnits:     []string{appDir, dbDir, vpcDir},
		},
		{
			name:          "selective dependent discovery - ...vpc",
			filterQueries: []string{"...vpc"},
			wantUnits:     []string{vpcDir, dbDir, appDir},
		},
		{
			name:          "both directions - ...db...",
			filterQueries: []string{"...db..."},
			wantUnits:     []string{dbDir, vpcDir, appDir},
		},
		{
			name:          "exclude target - ^app...",
			filterQueries: []string{"^app..."},
			wantUnits:     []string{dbDir, vpcDir},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse filter queries
			filters, err := filter.ParseFilterQueries(tt.filterQueries)
			require.NoError(t, err)

			// Create discovery with filters
			discovery := discovery.NewDiscovery(tmpDir).WithFilters(filters)

			components, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			// Filter results by type
			units := components.Filter(component.UnitKind).Paths()

			// Verify results
			assert.ElementsMatch(t, tt.wantUnits, units, "Units mismatch for test: %s", tt.name)
		})
	}
}

func TestDiscoveryWithGraphExpressionFilters_ComplexGraph(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// To speed up this test, we'll make the temporary directory a git repository.
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

	// Create test files with dependencies
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

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	t.Run("dependency traversal from app finds all dependencies", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"app..."})
		require.NoError(t, err)

		discovery := discovery.NewDiscovery(tmpDir).WithFilters(filters)
		configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
		require.NoError(t, err)

		units := configs.Filter(component.UnitKind).Paths()
		assert.ElementsMatch(t, []string{appDir, dbDir, cacheDir, vpcDir}, units)
	})

	t.Run("dependent traversal from vpc finds all dependents", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"...vpc"})
		require.NoError(t, err)

		discovery := discovery.NewDiscovery(tmpDir).WithFilters(filters)
		configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
		require.NoError(t, err)

		units := configs.Filter(component.UnitKind).Paths()
		assert.ElementsMatch(t, []string{vpcDir, dbDir, cacheDir, appDir}, units)
	})
}

func TestDiscoveryWithGraphExpressionFilters_OnlyMatchingComponentsTriggerDiscovery(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

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

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	t.Run("graph expression only discovers dependencies of matching component", func(t *testing.T) {
		t.Parallel()

		// Filter for app and its dependencies - unrelated should not be included
		filters, err := filter.ParseFilterQueries([]string{"app..."})
		require.NoError(t, err)

		discovery := discovery.NewDiscovery(tmpDir).WithFilters(filters)
		configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
		require.NoError(t, err)

		units := configs.Filter(component.UnitKind).Paths()
		// Should include app and db, but NOT unrelated
		assert.ElementsMatch(t, []string{appDir, dbDir}, units)
		assert.NotContains(t, units, unrelatedDir)
	})
}

func TestDiscoveryWithGraphExpressionFilters_FiltersAppliedAfterDiscovery(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
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

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	t.Run("additional filters applied after graph discovery", func(t *testing.T) {
		t.Parallel()

		// Graph expression discovers app and its dependencies, then additional filter excludes vpc
		filters, err := filter.ParseFilterQueries([]string{"app...", "!vpc"})
		require.NoError(t, err)

		discovery := discovery.NewDiscovery(tmpDir).WithFilters(filters)
		configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
		require.NoError(t, err)

		units := configs.Filter(component.UnitKind).Paths()
		// Should include app and db (from graph), but exclude vpc (from filter)
		assert.ElementsMatch(t, []string{appDir, dbDir}, units)
		assert.NotContains(t, units, vpcDir)
	})
}

func TestDiscoveryWithGitFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		filterQueries func(string, string) []string
		wantUnits     func(string, string) []string
		wantStacks    []string
		errorExpected bool
	}{
		{
			name: "Git filter - changes between commits",
			filterQueries: func(fromRef, toRef string) []string {
				return []string{"[" + fromRef + "..." + toRef + "]"}
			},
			wantUnits: func(fromDir string, toDir string) []string {
				return []string{
					filepath.Join(fromDir, "cache"),
					filepath.Join(toDir, "app"),
					filepath.Join(toDir, "new"),
				}
			},
			wantStacks: []string{},
		},
		{
			name: "Git filter - single reference (compared to HEAD)",
			filterQueries: func(fromRef, toRef string) []string {
				return []string{"[" + fromRef + "]"}
			},
			wantUnits: func(fromDir string, toDir string) []string {
				return []string{
					filepath.Join(fromDir, "cache"),
					filepath.Join(toDir, "app"),
					filepath.Join(toDir, "new"),
				}
			},
			wantStacks: []string{},
		},
		{
			name: "Git filter combined with path filter",
			filterQueries: func(fromRef, toRef string) []string {
				return []string{"[" + fromRef + "..." + toRef + "] | ./app"}
			},
			wantUnits: func(_ string, toDir string) []string {
				return []string{filepath.Join(toDir, "app")}
			},
			wantStacks: []string{},
		},
		{
			name: "Git filter combined with name filter",
			filterQueries: func(fromRef, toRef string) []string {
				return []string{"[" + fromRef + "..." + toRef + "] | name=new"}
			},
			wantUnits: func(_ string, toDir string) []string {
				return []string{filepath.Join(toDir, "new")}
			},
			wantStacks: []string{},
		},
		{
			name: "Multiple Git filters - union",
			filterQueries: func(fromRef, toRef string) []string {
				return []string{"[" + fromRef + "] | ./app", "[" + fromRef + "] | ./db"}
			},
			wantUnits: func(_ string, toDir string) []string {
				return []string{filepath.Join(toDir, "app")}
			},
			wantStacks: []string{},
		},
		{
			name: "Git filter with negation",
			filterQueries: func(fromRef, toRef string) []string {
				return []string{"[" + fromRef + "..." + toRef + "] | !name=new"}
			},
			wantUnits: func(fromDir string, toDir string) []string {
				return []string{
					filepath.Join(fromDir, "cache"),
					filepath.Join(toDir, "app"),
				}
			},
			wantStacks: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			tmpDir, err := filepath.EvalSymlinks(tmpDir)
			require.NoError(t, err)

			// Initialize Git repository
			runner, err := git.NewGitRunner()
			require.NoError(t, err)

			runner = runner.WithWorkDir(tmpDir)

			err = runner.Init(t.Context())
			require.NoError(t, err)

			err = runner.GoOpenRepo()
			require.NoError(t, err)

			defer runner.GoCloseStorage()

			// Create initial components
			appDir := filepath.Join(tmpDir, "app")
			dbDir := filepath.Join(tmpDir, "db")
			cacheDir := filepath.Join(tmpDir, "cache")

			testDirs := []string{appDir, dbDir, cacheDir}
			for _, dir := range testDirs {
				err = os.MkdirAll(dir, 0755)
				require.NoError(t, err)
			}

			// Create initial files
			initialFiles := map[string]string{
				filepath.Join(appDir, "terragrunt.hcl"):   ``,
				filepath.Join(dbDir, "terragrunt.hcl"):    ``,
				filepath.Join(cacheDir, "terragrunt.hcl"): ``,
			}

			for path, content := range initialFiles {
				err = os.WriteFile(path, []byte(content), 0644)
				require.NoError(t, err)
			}

			// Commit initial state
			err = runner.GoAdd(".")
			require.NoError(t, err)

			err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
				Author: &object.Signature{
					Name:  "Test User",
					Email: "test@example.com",
					When:  time.Now(),
				},
			})
			require.NoError(t, err)

			// Create new components and modify existing ones
			newDir := filepath.Join(tmpDir, "new")
			err = os.MkdirAll(newDir, 0755)
			require.NoError(t, err)

			// Modify app component
			err = os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(`
locals {
	modified = true
}
`), 0644)
			require.NoError(t, err)

			// Add new component
			err = os.WriteFile(filepath.Join(newDir, "terragrunt.hcl"), []byte(``), 0644)
			require.NoError(t, err)

			// Remove cache component
			err = os.RemoveAll(cacheDir)
			require.NoError(t, err)

			// Commit changes
			err = runner.GoAdd(".")
			require.NoError(t, err)

			err = runner.GoCommit("Changes: modified app, added new, removed cache", &gogit.CommitOptions{
				Author: &object.Signature{
					Name:  "Test User",
					Email: "test@example.com",
					When:  time.Now(),
				},
			})
			require.NoError(t, err)

			opts := options.NewTerragruntOptions()
			opts.WorkingDir = tmpDir
			opts.RootWorkingDir = tmpDir

			// Parse filter queries
			filters, err := filter.ParseFilterQueries(tt.filterQueries("HEAD~1", "HEAD"))
			if tt.errorExpected {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			w, err := worktrees.NewWorktrees(
				t.Context(),
				logger.CreateLogger(),
				tmpDir,
				filters.UniqueGitFilters(),
			)
			require.NoError(t, err)

			// Cleanup worktrees after test completes
			t.Cleanup(func() {
				cleanupErr := w.Cleanup(context.Background(), logger.CreateLogger())
				require.NoError(t, cleanupErr)
			})

			// Create discovery with filters
			discovery := discovery.NewDiscovery(tmpDir).WithFilters(filters).WithWorktrees(w)

			configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			if tt.errorExpected {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Filter results by type
			units := configs.Filter(component.UnitKind).Paths()
			stacks := configs.Filter(component.StackKind).Paths()

			worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
			require.NotEmpty(t, worktreePair)

			// Both from and to worktree paths are used directly - no translation to original working dir
			wantUnits := tt.wantUnits(worktreePair.FromWorktree.Path, worktreePair.ToWorktree.Path)

			// Verify results
			assert.ElementsMatch(t, wantUnits, units, "Units mismatch for test: %s", tt.name)
			assert.ElementsMatch(t, tt.wantStacks, stacks, "Stacks mismatch for test: %s", tt.name)
		})
	}
}

func TestDiscoveryWithGitFilters_WorktreeCleanup(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Initialize Git repository
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	defer runner.GoCloseStorage()

	// Create a component
	appDir := filepath.Join(tmpDir, "app")
	err = os.MkdirAll(appDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(``), 0644)
	require.NoError(t, err)

	// Commit initial state
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Make a change
	err = os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(`modified`), 0644)
	require.NoError(t, err)

	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Modified app", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Parse filter with Git references
	filters, err := filter.ParseFilterQueries([]string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	w, err := worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		tmpDir,
		filters.UniqueGitFilters(),
	)
	require.NoError(t, err)

	// Cleanup worktrees - should succeed if worktrees exist
	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.Background(), logger.CreateLogger())
		require.NoError(t, cleanupErr)
	})

	// Create discovery with filters
	discovery := discovery.NewDiscovery(tmpDir).WithFilters(filters).WithWorktrees(w)

	// Discover components - this should create worktrees
	configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)
	require.NotEmpty(t, configs)
}

func TestDiscoveryWithGitFilters_NoChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Initialize Git repository
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	defer runner.GoCloseStorage()

	// Create a component
	appDir := filepath.Join(tmpDir, "app")
	err = os.MkdirAll(appDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(``), 0644)
	require.NoError(t, err)

	// Commit initial state
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Commit again with no changes (empty commit)
	err = runner.GoCommit("Empty commit", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Parse filter with Git references (no changes between commits)
	filters, err := filter.ParseFilterQueries([]string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	w, err := worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		tmpDir,
		filters.UniqueGitFilters(),
	)
	require.NoError(t, err)

	// Create discovery with filters
	discovery := discovery.NewDiscovery(tmpDir).WithFilters(filters).WithWorktrees(w)

	// Discover components
	components, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	// Cleanup worktrees
	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.Background(), logger.CreateLogger())
		require.NoError(t, cleanupErr)
	})

	// Should return no components since nothing changed
	units := components.Filter(component.UnitKind).Paths()
	assert.Empty(t, units, "No components should be returned when there are no changes")
}

// TestDiscoveryWithGitFilters_FromSubdirectory tests that git filter discovery works correctly
// when running from a subdirectory of the git root. This is a regression test for the bug where
// paths were incorrectly duplicated (e.g., "basic/basic/basic-2" instead of "basic/basic-2").
func TestDiscoveryWithGitFilters_FromSubdirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Initialize Git repository at the root
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	defer runner.GoCloseStorage()

	// Create subdirectory structure: basic/basic-1, basic/basic-2
	basicDir := filepath.Join(tmpDir, "basic")
	basic1Dir := filepath.Join(basicDir, "basic-1")
	basic2Dir := filepath.Join(basicDir, "basic-2")

	// Also create a component outside the subdirectory
	otherDir := filepath.Join(tmpDir, "other")

	testDirs := []string{basic1Dir, basic2Dir, otherDir}
	for _, dir := range testDirs {
		err = os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create initial files
	initialFiles := map[string]string{
		filepath.Join(basic1Dir, "terragrunt.hcl"): ``,
		filepath.Join(basic2Dir, "terragrunt.hcl"): ``,
		filepath.Join(otherDir, "terragrunt.hcl"):  ``,
	}

	for path, content := range initialFiles {
		err = os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Commit initial state
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Modify basic-2 component
	err = os.WriteFile(filepath.Join(basic2Dir, "terragrunt.hcl"), []byte(`
locals {
	modified = true
}
`), 0644)
	require.NoError(t, err)

	// Commit changes
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Modified basic-2", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Now run discovery FROM THE SUBDIRECTORY (basic)
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = basicDir     // Running from subdirectory
	opts.RootWorkingDir = basicDir // Also subdirectory

	// Parse filter with Git reference
	filters, err := filter.ParseFilterQueries([]string{"[HEAD~1]"})
	require.NoError(t, err)

	// Create worktrees from the subdirectory
	w, err := worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		basicDir, // Working dir is the subdirectory
		filters.UniqueGitFilters(),
	)
	require.NoError(t, err)

	// Cleanup worktrees after test
	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.Background(), logger.CreateLogger())
		require.NoError(t, cleanupErr)
	})

	// Create discovery with filters from the subdirectory
	discovery := discovery.NewDiscovery(basicDir).WithFilters(filters).WithWorktrees(w)

	configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	// Filter results by type
	units := configs.Filter(component.UnitKind).Paths()

	// With worktree-based execution, discovery runs directly in the worktree path
	// The discovered component should be in the worktree, not translated back to original path
	worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
	require.NotEmpty(t, worktreePair)

	expectedPath := filepath.Join(worktreePair.ToWorktree.Path, "basic", "basic-2")
	assert.ElementsMatch(t, []string{expectedPath}, units,
		"Should discover basic-2 with correct path when running from subdirectory")

	// Verify the path doesn't have duplicated directory names
	// The bug was that paths like "basic/basic-2" became "basic/basic/basic-2"
	// (the "basic" prefix was duplicated before the component name)
	for _, unitPath := range units {
		// The path should not contain "basic/basic/basic-" which would indicate path duplication
		assert.NotContains(t, unitPath, "basic"+string(filepath.Separator)+"basic"+string(filepath.Separator)+"basic-",
			"Path should not have duplicated directory names")
	}
}

// TestDiscoveryWithGitFilters_FromSubdirectory_MultipleCommits tests git filter discovery
// initiated from a subdirectory when comparing against multiple commits back (HEAD~2, HEAD~3).
func TestDiscoveryWithGitFilters_FromSubdirectory_MultipleCommits(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Initialize Git repository at the root
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	t.Cleanup(func() {
		runner.GoCloseStorage()
	})

	// Create subdirectory structure: basic/basic-1, basic/basic-2, basic/basic-3
	basicDir := filepath.Join(tmpDir, "basic")
	basic1Dir := filepath.Join(basicDir, "basic-1")
	basic2Dir := filepath.Join(basicDir, "basic-2")
	basic3Dir := filepath.Join(basicDir, "basic-3")

	// Also create components outside the subdirectory
	otherDir := filepath.Join(tmpDir, "other")
	anotherDir := filepath.Join(tmpDir, "another")

	testDirs := []string{basic1Dir, basic2Dir, basic3Dir, otherDir, anotherDir}
	for _, dir := range testDirs {
		err = os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Commit 1: Initial state with all components
	initialFiles := map[string]string{
		filepath.Join(basic1Dir, "terragrunt.hcl"):  ``,
		filepath.Join(basic2Dir, "terragrunt.hcl"):  ``,
		filepath.Join(basic3Dir, "terragrunt.hcl"):  ``,
		filepath.Join(otherDir, "terragrunt.hcl"):   ``,
		filepath.Join(anotherDir, "terragrunt.hcl"): ``,
	}

	for path, content := range initialFiles {
		err = os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Commit 2: Modify basic-1 and other (outside subdirectory)
	err = os.WriteFile(filepath.Join(basic1Dir, "terragrunt.hcl"), []byte(`
locals {
	version = "v1"
}
`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(otherDir, "terragrunt.hcl"), []byte(`
locals {
	modified = true
}
`), 0644)
	require.NoError(t, err)

	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Commit 2: modify basic-1 and other", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Commit 3: Modify basic-2 and another (outside subdirectory)
	err = os.WriteFile(filepath.Join(basic2Dir, "terragrunt.hcl"), []byte(`
locals {
	version = "v2"
}
`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(anotherDir, "terragrunt.hcl"), []byte(`
locals {
	modified = true
}
`), 0644)
	require.NoError(t, err)

	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Commit 3: modify basic-2 and another", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Commit 4: Modify basic-3
	err = os.WriteFile(filepath.Join(basic3Dir, "terragrunt.hcl"), []byte(`
locals {
	version = "v3"
}
`), 0644)
	require.NoError(t, err)

	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Commit 4: modify basic-3", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	tests := []struct {
		expectedUnitsFunc func(toWorktreePath string) []string
		name              string
		gitRef            string
	}{
		{
			name:   "HEAD~1 from subdirectory - only basic-3",
			gitRef: "HEAD~1",
			expectedUnitsFunc: func(toWorktreePath string) []string {
				return []string{filepath.Join(toWorktreePath, "basic", "basic-3")}
			},
		},
		{
			name:   "HEAD~2 from subdirectory - basic-2 and basic-3, plus another",
			gitRef: "HEAD~2",
			expectedUnitsFunc: func(toWorktreePath string) []string {
				// With worktree-root discovery, we find all changed units including 'another'
				return []string{
					filepath.Join(toWorktreePath, "another"),
					filepath.Join(toWorktreePath, "basic", "basic-2"),
					filepath.Join(toWorktreePath, "basic", "basic-3"),
				}
			},
		},
		{
			name:   "HEAD~3 from subdirectory - basic-1, basic-2, basic-3, plus other and another",
			gitRef: "HEAD~3",
			expectedUnitsFunc: func(toWorktreePath string) []string {
				// With worktree-root discovery, we find all changed units
				return []string{
					filepath.Join(toWorktreePath, "other"),
					filepath.Join(toWorktreePath, "another"),
					filepath.Join(toWorktreePath, "basic", "basic-1"),
					filepath.Join(toWorktreePath, "basic", "basic-2"),
					filepath.Join(toWorktreePath, "basic", "basic-3"),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Run discovery FROM THE SUBDIRECTORY (basic)
			opts := options.NewTerragruntOptions()
			opts.WorkingDir = basicDir
			opts.RootWorkingDir = basicDir

			// Parse filter with Git reference
			filters, err := filter.ParseFilterQueries([]string{"[" + tt.gitRef + "]"})
			require.NoError(t, err)

			// Create worktrees from the subdirectory
			w, err := worktrees.NewWorktrees(
				t.Context(),
				logger.CreateLogger(),
				basicDir,
				filters.UniqueGitFilters(),
			)
			require.NoError(t, err)

			// Cleanup worktrees after test
			t.Cleanup(func() {
				cleanupErr := w.Cleanup(context.Background(), logger.CreateLogger())
				require.NoError(t, cleanupErr)
			})

			// Create discovery with filters from the subdirectory
			disc := discovery.NewDiscovery(basicDir).WithFilters(filters).WithWorktrees(w)

			configs, err := disc.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			// Filter results by type
			units := configs.Filter(component.UnitKind).Paths()

			// Get worktree pair for expected path calculation
			worktreePair := w.WorktreePairs["["+tt.gitRef+"...HEAD]"]
			require.NotEmpty(t, worktreePair)

			// Verify correct units are discovered
			// With worktree-based execution, discovery runs from worktree root
			// and returns worktree paths directly (no translation to original paths)
			expectedUnits := tt.expectedUnitsFunc(worktreePair.ToWorktree.Path)
			assert.ElementsMatch(t, expectedUnits, units,
				"Should discover correct units when running from subdirectory with %s", tt.gitRef)

			// Verify no path duplication
			for _, unitPath := range units {
				assert.NotContains(t, unitPath,
					"basic"+string(filepath.Separator)+"basic"+string(filepath.Separator)+"basic-",
					"Path should not have duplicated directory names")
			}
		})
	}
}
