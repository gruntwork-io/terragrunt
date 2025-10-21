package discovery_test

import (
	"fmt"
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

func TestDiscoveryWithFilters(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

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
			wantUnits:     []string{frontendDir, backendDir, legacyDir, dbDir, cacheDir, externalAppDir},
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
			wantUnits:     []string{frontendDir, backendDir, legacyDir, dbDir, cacheDir, externalAppDir},
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
			filterQueries: []string{"external=true"},
			wantUnits:     []string{externalAppDir},
			wantStacks:    []string{},
		},
		{
			name:          "external filter - internal components",
			filterQueries: []string{"external=false"},
			wantUnits:     []string{frontendDir, backendDir, legacyDir, dbDir, cacheDir},
			wantStacks:    []string{stackDir},
		},
		{
			name:          "negation filter - exclude legacy",
			filterQueries: []string{"!legacy"},
			wantUnits:     []string{frontendDir, backendDir, dbDir, cacheDir, externalAppDir},
			wantStacks:    []string{stackDir},
		},
		{
			name:          "negation filter - exclude apps directory",
			filterQueries: []string{"!./apps/*"},
			wantUnits:     []string{dbDir, cacheDir, externalAppDir},
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
			filters, err := filter.ParseFilterQueries(tt.filterQueries, tmpDir)
			if tt.errorExpected {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Create discovery with filters
			discovery := discovery.NewDiscovery(tmpDir, tt.discoveryOpts...).WithFilters(filters)

			// Discover components with dependencies to test external filtering
			discovery = discovery.WithDiscoverDependencies().WithDiscoverExternalDependencies()

			configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			// Filter results by type
			units := configs.Filter(component.Unit).Paths()
			stacks := configs.Filter(component.Stack).Paths()

			// Verify results
			assert.ElementsMatch(t, tt.wantUnits, units, "Units mismatch for test: %s", tt.name)
			assert.ElementsMatch(t, tt.wantStacks, stacks, "Stacks mismatch for test: %s", tt.name)
		})
	}
}

func TestDiscoveryWithFiltersErrorHandling(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
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
			filters, err := filter.ParseFilterQueries(tt.filterQueries, tmpDir)

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

	// Create a single component for edge case testing
	unitDir := filepath.Join(tmpDir, "unit")
	err := os.MkdirAll(unitDir, 0755)
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
			filterQueries: []string{"{unit}"},
			wantUnits:     []string{unitDir},
			wantStacks:    []string{},
		},
		{
			name:          "filter with spaces in name",
			filterQueries: []string{"unit"},
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
			filterQueries: []string{"!!unit"},
			wantUnits:     []string{unitDir},
			wantStacks:    []string{},
		},
		{
			name:          "empty intersection",
			filterQueries: []string{"unit | nonexistent"},
			wantUnits:     []string{},
			wantStacks:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse filter queries
			filters, err := filter.ParseFilterQueries(tt.filterQueries, tmpDir)
			require.NoError(t, err)

			// Create discovery with filters
			discovery := discovery.NewDiscovery(tmpDir).WithFilters(filters)

			configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			// Filter results by type
			units := configs.Filter(component.Unit).Paths()
			stacks := configs.Filter(component.Stack).Paths()

			// Verify results
			assert.ElementsMatch(t, tt.wantUnits, units, "Units mismatch for test: %s", tt.name)
			assert.ElementsMatch(t, tt.wantStacks, stacks, "Stacks mismatch for test: %s", tt.name)
		})
	}
}

func TestDiscoveryWithFiltersPerformance(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create many components to test performance
	numComponents := 100
	componentDirs := make([]string, numComponents)

	for i := 0; i < numComponents; i++ {
		dir := filepath.Join(tmpDir, "component", "app", "service", fmt.Sprintf("service-%d", i))
		componentDirs[i] = dir
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(""), 0644)
		require.NoError(t, err)
	}

	opts, err := options.NewTerragruntOptionsForTest(tmpDir)
	require.NoError(t, err)

	tests := []struct {
		name          string
		filterQueries []string
		expectedCount int
	}{
		{
			name:          "wildcard filter - all components",
			filterQueries: []string{"./component/**/*"},
			expectedCount: numComponents,
		},
		{
			name:          "specific path filter",
			filterQueries: []string{"./component/app/service/service-0"},
			expectedCount: 1,
		},
		{
			name:          "pattern filter",
			filterQueries: []string{"./component/app/service/service-*"},
			expectedCount: numComponents,
		},
		{
			name:          "negation filter",
			filterQueries: []string{"!./component/app/service/service-0"},
			expectedCount: numComponents - 1,
		},
		{
			name:          "multiple filters",
			filterQueries: []string{"./component/app/service/service-0", "./component/app/service/service-1"},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse filter queries
			filters, err := filter.ParseFilterQueries(tt.filterQueries, tmpDir)
			require.NoError(t, err)

			// Create discovery with filters
			discovery := discovery.NewDiscovery(tmpDir).WithFilters(filters)

			// Measure discovery time
			configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			// Verify count
			assert.Len(t, configs, tt.expectedCount, "Component count mismatch for test: %s", tt.name)
		})
	}
}

func TestDiscoveryWithFiltersAndDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create test directory structure with dependencies
	appDir := filepath.Join(tmpDir, "app")
	dbDir := filepath.Join(tmpDir, "db")
	vpcDir := filepath.Join(tmpDir, "vpc")
	// Create external component outside the working directory to make it truly external
	externalDir := filepath.Join(filepath.Dir(tmpDir), "external")
	externalAppDir := filepath.Join(externalDir, "app")

	testDirs := []string{appDir, dbDir, vpcDir, externalAppDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Clean up external directory after test
	t.Cleanup(func() {
		os.RemoveAll(externalDir)
	})

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
			name:          "filter internal dependencies only",
			filterQueries: []string{"external=false"},
			wantUnits:     []string{appDir, dbDir, vpcDir},
			wantStacks:    []string{},
		},
		{
			name:          "filter external dependencies only",
			filterQueries: []string{"external=true"},
			wantUnits:     []string{externalAppDir},
			wantStacks:    []string{},
		},
		{
			name:          "filter specific component and its dependencies",
			filterQueries: []string{"app"},
			wantUnits:     []string{appDir, externalAppDir},
			wantStacks:    []string{},
		},
		{
			name:          "filter with negation - exclude external",
			filterQueries: []string{"!external=true"},
			wantUnits:     []string{appDir, dbDir, vpcDir},
			wantStacks:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse filter queries
			filters, err := filter.ParseFilterQueries(tt.filterQueries, tmpDir)
			require.NoError(t, err)

			// Create discovery with filters and dependencies
			discovery := discovery.NewDiscovery(tmpDir).
				WithFilters(filters).
				WithDiscoverDependencies().
				WithDiscoverExternalDependencies()

			configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			// Filter results by type
			units := configs.Filter(component.Unit).Paths()
			stacks := configs.Filter(component.Stack).Paths()

			// Verify results
			assert.ElementsMatch(t, tt.wantUnits, units, "Units mismatch for test: %s", tt.name)
			assert.ElementsMatch(t, tt.wantStacks, stacks, "Stacks mismatch for test: %s", tt.name)
		})
	}
}

func TestDiscoveryWithReadingFilters(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

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
			filters, err := filter.ParseFilterQueries(tt.filterQueries, tmpDir)
			require.NoError(t, err)

			// Create discovery with filters and ReadFiles enabled
			discovery := discovery.NewDiscovery(tmpDir).
				WithFilters(filters).
				WithReadFiles()

			configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
			require.NoError(t, err)

			// Filter results by type
			units := configs.Filter(component.Unit).Paths()
			stacks := configs.Filter(component.Stack).Paths()

			// Verify results
			assert.ElementsMatch(t, tt.wantUnits, units, "Units mismatch for test: %s", tt.name)
			assert.ElementsMatch(t, tt.wantStacks, stacks, "Stacks mismatch for test: %s", tt.name)
		})
	}
}

func TestDiscoveryWithReadingFiltersAndAbsolutePaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

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
	filters, err := filter.ParseFilterQueries(filterQueries, tmpDir)
	require.NoError(t, err)

	discovery := discovery.NewDiscovery(tmpDir).
		WithFilters(filters).
		WithReadFiles()

	configs, err := discovery.Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	// Should find the app component when filtering by absolute path
	units := configs.Filter(component.Unit).Paths()
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
			filters, err := filter.ParseFilterQueries(tt.filterQueries, tmpDir)

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
