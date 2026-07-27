//go:build tf

package test_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFFilterFlagWithRunAllGraphExpressions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		filterQuery   string
		expectedUnits []string
		expectError   bool
	}{
		{
			// service -> db, cache, vpc (all dependencies)
			// So "service..." should execute service and all its dependencies
			name:          "dependency traversal - service... executes dependencies",
			filterQuery:   "service...",
			expectedUnits: []string{"service", "db", "cache", "vpc"},
			expectError:   false,
		},
		{
			// vpc has dependents: db, cache, service (all depend on vpc)
			// So "...vpc" should execute all: vpc, db, cache, service
			name:          "dependent traversal - ...vpc executes all dependents",
			filterQuery:   "...vpc",
			expectedUnits: []string{"vpc", "db", "cache", "service"},
			expectError:   false,
		},
		{
			// db has dependency (vpc) and dependent (service)
			// So "...db..." should execute all: vpc, db, service
			name:          "both directions - ...db... executes related units",
			filterQuery:   "...db...",
			expectedUnits: []string{"vpc", "db", "service"},
			expectError:   false,
		},
		{
			// cache has dependency (vpc) and dependent (service)
			// So "...cache..." should execute all: vpc, cache, service
			name:          "both directions - ...cache... executes related units",
			filterQuery:   "...cache...",
			expectedUnits: []string{"vpc", "cache", "service"},
			expectError:   false,
		},
		{
			// "service..." finds service, db, cache, vpc
			// "^service..." excludes service, so only dependencies should execute
			name:          "exclude target - ^service... executes only dependencies",
			filterQuery:   "^service...",
			expectedUnits: []string{"db", "cache", "vpc"},
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureRunFilter)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRunFilter)
			workingDir := filepath.Join(tmpEnvPath, testFixtureRunFilter)

			reportFile := filepath.Join(workingDir, "report.json")
			cmd := "terragrunt run --all --non-interactive --working-dir " + workingDir + " --filter '" + tc.filterQuery + "' --report-file " + reportFile + " --report-format json -- plan"
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)

				return
			}

			require.FileExists(t, reportFile)

			runs, parseErr := report.ParseJSONRunsFromFile(reportFile)
			require.NoError(t, parseErr)

			reportUnits := runs.Names()

			reportUnitMap := make(map[string]struct{})
			for _, unit := range reportUnits {
				reportUnitMap[unit] = struct{}{}
			}

			assert.ElementsMatch(t, tc.expectedUnits, reportUnits)
		})
	}
}

func TestTFFilterFlagWithRunAllGraphExpressionsVerifyExecutionOrder(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRunFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRunFilter)
	workingDir := filepath.Join(tmpEnvPath, testFixtureRunFilter)

	// Test that "service..." executes vpc, db, cache (dependencies) before service
	reportFile := filepath.Join(workingDir, "report.json")
	cmd := "terragrunt run --all --non-interactive --working-dir " + workingDir + " --filter 'service...' --report-file " + reportFile + " --report-format json -- plan"
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	require.FileExists(t, reportFile)

	runs, parseErr := report.ParseJSONRunsFromFile(reportFile)
	require.NoError(t, parseErr)

	// Verify execution order: dependencies (vpc, db, cache) should start before service
	// We expect: vpc, db, cache should have started before service
	dependencies := []string{"vpc", "db", "cache"}
	dependent := "service"

	service := runs.FindByName(dependent)
	require.NotNil(t, service)

	// Verify each dependency started before service
	for _, depName := range dependencies {
		dep := runs.FindByName(depName)
		require.NotNil(t, dep)

		assert.True(
			t,
			dep.Started.Before(service.Started),
		)
	}
}

// TestTFFilterFlagWithRunAllCombinedGitAndGraphExpressions tests the `run --all` command
// with combined git + graph filter expressions.
func TestTFFilterFlagWithRunAllCombinedGitAndGraphExpressions(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) string {
		t.Helper()

		tmpDir := helpers.TmpDirWOSymlinks(t)

		runner := helpers.InitTestGitRunner(t, tmpDir)

		// Create a dependency chain: service -> cache -> vpc
		// We'll modify 'cache' and use git+graph filter

		vpcDir := filepath.Join(tmpDir, "vpc")
		err := os.MkdirAll(vpcDir, 0755)
		require.NoError(t, err)

		vpcHCL := `# VPC unit`
		err = os.WriteFile(filepath.Join(vpcDir, "terragrunt.hcl"), []byte(vpcHCL), 0644)
		require.NoError(t, err)

		vpcTF := `# VPC TF`
		err = os.WriteFile(filepath.Join(vpcDir, "main.tf"), []byte(vpcTF), 0644)
		require.NoError(t, err)

		cacheDir := filepath.Join(tmpDir, "cache")
		err = os.MkdirAll(cacheDir, 0755)
		require.NoError(t, err)

		cacheHCL := `# Cache unit
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    value = "mock value"
  }
}
`
		err = os.WriteFile(filepath.Join(cacheDir, "terragrunt.hcl"), []byte(cacheHCL), 0644)
		require.NoError(t, err)

		cacheTF := `# Cache TF`
		err = os.WriteFile(filepath.Join(cacheDir, "main.tf"), []byte(cacheTF), 0644)
		require.NoError(t, err)

		serviceDir := filepath.Join(tmpDir, "service")
		err = os.MkdirAll(serviceDir, 0755)
		require.NoError(t, err)

		serviceHCL := `# Service unit
dependency "cache" {
  config_path = "../cache"

  mock_outputs = {
    value = "mock value"
  }
}
`
		err = os.WriteFile(filepath.Join(serviceDir, "terragrunt.hcl"), []byte(serviceHCL), 0644)
		require.NoError(t, err)

		serviceTF := `# Service TF`
		err = os.WriteFile(filepath.Join(serviceDir, "main.tf"), []byte(serviceTF), 0644)
		require.NoError(t, err)

		// Initial commit
		require.NoError(t, runner.Add(t.Context(), "."))
		require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

		modifiedCacheHCL := `# Cache unit (MODIFIED)
dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    value = "mock value"
  }
}
`
		err = os.WriteFile(
			filepath.Join(cacheDir, "terragrunt.hcl"),
			[]byte(modifiedCacheHCL),
			0644,
		)
		require.NoError(t, err)

		require.NoError(t, runner.Add(t.Context(), "."))
		require.NoError(t, runner.Commit(t.Context(), "Modify cache"))

		return tmpDir
	}

	testCases := []struct {
		name          string
		filterQuery   string
		description   string
		expectedUnits []string
	}{
		{
			name:          "git filter only - run baseline",
			filterQuery:   "[HEAD~1...HEAD]",
			expectedUnits: []string{"cache"},
			description:   "Baseline: run with git filter should execute cache",
		},
		{
			name:          "dependencies of git changes - run",
			filterQuery:   "[HEAD~1...HEAD]...",
			expectedUnits: []string{"cache", "vpc"},
			description:   "Should run cache and its dependency vpc",
		},
		{
			name:          "dependents of git changes - run",
			filterQuery:   "...[HEAD~1...HEAD]",
			expectedUnits: []string{"cache", "service"},
			description:   "Should run cache and its dependent service",
		},
		{
			name:          "both directions - run",
			filterQuery:   "...[HEAD~1...HEAD]...",
			expectedUnits: []string{"vpc", "cache", "service"},
			description:   "Should run vpc (dep), cache (target), service (dependent)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := setup(t)

			reportFile := filepath.Join(tmpDir, "report.json")
			cmd := "terragrunt run --all --non-interactive --no-color --working-dir " + tmpDir +
				" --filter '" + tc.filterQuery + "' --report-file " + reportFile + " --report-format json -- plan"

			_, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			require.FileExists(t, reportFile)

			runs, parseErr := report.ParseJSONRunsFromFile(reportFile)
			require.NoError(t, parseErr)

			assert.ElementsMatch(t, tc.expectedUnits, runs.Names())
		})
	}
}
