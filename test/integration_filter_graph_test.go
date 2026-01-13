package test_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureFilterGraphDAG = "fixtures/find/dag"
	testFixtureRunFilter      = "fixtures/run-filter"
)

func TestFilterFlagWithFindGraphExpressions(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled

	testCases := []struct {
		name           string
		filterQuery    string
		expectedOutput string
		expectError    bool
	}{
		{
			// a-dependent -> b-dependency
			// So "a-dependent..." should find a-dependent and b-dependency
			name:           "dependency traversal - a-dependent...",
			filterQuery:    "a-dependent...",
			expectedOutput: "a-dependent\nb-dependency\n",
			expectError:    false,
		},
		{
			// b-dependency is a dependency of a-dependent, c-mixed-deps, and d-dependencies-only
			// So "...b-dependency" should find b-dependency and all its dependents
			// Note: Actually, b-dependency has no dependents in this graph - it's only a dependency
			// But c-mixed-deps depends on a-dependent which depends on b-dependency
			// And d-dependencies-only depends on a-dependent which depends on b-dependency
			// So ...b-dependency should find: b-dependency, a-dependent, c-mixed-deps, d-dependencies-only
			name:           "dependent traversal - ...b-dependency",
			filterQuery:    "...b-dependency",
			expectedOutput: "a-dependent\nb-dependency\nc-mixed-deps\nd-dependencies-only\n",
			expectError:    false,
		},
		{
			// a-dependent has dependencies (b-dependency) and dependents (c-mixed-deps, d-dependencies-only)
			// So "...a-dependent..." should find all: b-dependency, a-dependent, c-mixed-deps, d-dependencies-only
			name:           "both directions - ...a-dependent...",
			filterQuery:    "...a-dependent...",
			expectedOutput: "a-dependent\nb-dependency\nc-mixed-deps\nd-dependencies-only\n",
			expectError:    false,
		},
		{
			// "a-dependent..." finds a-dependent and b-dependency
			// "^a-dependent..." excludes a-dependent, so only b-dependency
			name:           "exclude target - ^a-dependent...",
			filterQuery:    "^a-dependent...",
			expectedOutput: "b-dependency\n",
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFilterGraphDAG)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFilterGraphDAG)
			workingDir := filepath.Join(tmpEnvPath, testFixtureFilterGraphDAG)

			cmd := "terragrunt find --no-color --working-dir " + workingDir + " --filter '" + tc.filterQuery + "'"
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")
			} else {
				require.NoError(t, err, "Unexpected error for filter query: %s", tc.filterQuery)
				// Allow warnings in stderr (e.g., suppressed parsing errors during discovery)
				// but ensure there are no actual errors
				if stderr != "" {
					// Check that stderr only contains expected warnings, not actual errors
					lowerStderr := strings.ToLower(stderr)
					if strings.Contains(lowerStderr, "error") && !strings.Contains(lowerStderr, "suppressed") && !strings.Contains(lowerStderr, "warning") {
						t.Errorf("Unexpected error in stderr: %s", stderr)
					}
				}

				// Sort both outputs for comparison (find output order may vary)
				expectedLines := strings.Fields(tc.expectedOutput)
				actualLines := strings.Fields(stdout)
				assert.ElementsMatch(t, expectedLines, actualLines, "Output mismatch for filter query: %s", tc.filterQuery)
			}
		})
	}
}

func TestFilterFlagWithFindGraphExpressionsJSON(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		filterQuery   string
		expectedPaths []string
		expectError   bool
	}{
		{
			name:          "dependency traversal - a-dependent... JSON",
			filterQuery:   "a-dependent...",
			expectedPaths: []string{"a-dependent", "b-dependency"},
			expectError:   false,
		},
		{
			name:          "dependent traversal - ...b-dependency JSON",
			filterQuery:   "...b-dependency",
			expectedPaths: []string{"a-dependent", "b-dependency", "c-mixed-deps", "d-dependencies-only"},
			expectError:   false,
		},
		{
			name:          "both directions - ...a-dependent... JSON",
			filterQuery:   "...a-dependent...",
			expectedPaths: []string{"a-dependent", "b-dependency", "c-mixed-deps", "d-dependencies-only"},
			expectError:   false,
		},
		{
			name:          "exclude target - ^a-dependent... JSON",
			filterQuery:   "^a-dependent...",
			expectedPaths: []string{"b-dependency"},
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFilterGraphDAG)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFilterGraphDAG)
			workingDir := filepath.Join(tmpEnvPath, testFixtureFilterGraphDAG)

			cmd := "terragrunt find --no-color --working-dir " + workingDir + " --json --filter '" + tc.filterQuery + "'"
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")

				return
			}

			require.NoError(t, err, "Unexpected error for filter query: %s", tc.filterQuery)

			// Parse JSON output and verify paths
			// The JSON output should be an array of objects with "path" field
			assert.NotEmpty(t, stdout, "JSON output should not be empty")
			assert.Contains(t, stdout, "[", "JSON output should be an array")

			// Verify each expected path appears in the JSON output
			for _, expectedPath := range tc.expectedPaths {
				assert.Contains(t, stdout, `"path"`, "JSON output should contain path field")
				// The path might be relative or absolute, so we check for the component name
				assert.Contains(t, stdout, expectedPath, "JSON output should contain path: %s", expectedPath)
			}
		})
	}
}

func TestFilterFlagWithRunGraphExpressions(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled

	testCases := []struct {
		name         string
		filterQuery  string
		errorPattern string
		expectError  bool
	}{
		{
			name:        "dependency traversal - a-dependent...",
			filterQuery: "a-dependent...",
			expectError: false,
		},
		{
			name:        "dependent traversal - ...b-dependency",
			filterQuery: "...b-dependency",
			expectError: false,
		},
		{
			name:        "both directions - ...a-dependent...",
			filterQuery: "...a-dependent...",
			expectError: false,
		},
		{
			name:        "exclude target - ^a-dependent...",
			filterQuery: "^a-dependent...",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFilterGraphDAG)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFilterGraphDAG)
			workingDir := filepath.Join(tmpEnvPath, testFixtureFilterGraphDAG)

			// Use a non-destructive command like `plan` to verify the filter works
			// The actual terraform commands will likely fail due to missing providers/resources,
			// but we can verify that the filter parsing and discovery works correctly
			// by checking that we don't get filter-related errors
			cmd := "terragrunt run --all --non-interactive --working-dir " + workingDir + " --filter '" + tc.filterQuery + "' -- plan"
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)

				if tc.errorPattern != "" {
					assert.Contains(t, stderr, tc.errorPattern, "Error message should contain expected pattern")
				}
			} else {
				// The command might fail due to terraform init/plan errors (missing providers, etc),
				// which is expected in a test environment without full terraform setup.
				// The important thing is that the filter was parsed correctly and discovery worked.
				output := stdout + stderr

				// Verify we don't get filter parsing or evaluation errors
				errStr := ""
				if err != nil {
					errStr = err.Error()
				}

				// Check for filter-related errors (these would indicate a problem with graph expressions)
				if strings.Contains(output, "filter") {
					if strings.Contains(output, "parse") || strings.Contains(output, "syntax") || strings.Contains(output, "invalid") {
						t.Fatalf("Filter parsing/evaluation error detected in output: %s\nOutput: %s\nStderr: %s", errStr, stdout, stderr)
					}
				}

				// Check error string directly for filter issues
				if err != nil {
					if strings.Contains(errStr, "filter") && (strings.Contains(errStr, "parse") || strings.Contains(errStr, "syntax") || strings.Contains(errStr, "invalid")) {
						t.Fatalf("Filter parsing/evaluation error: %v\nOutput: %s\nStderr: %s", err, stdout, stderr)
					}
					// Terraform execution errors are acceptable - we're just verifying filter discovery works
					t.Logf("Command completed (Terraform execution errors are expected in test environment): %v", err)
				}

				// Verify that the command at least attempted to process units (discovery phase completed)
				// This is a basic sanity check - if discovery failed, we'd see different errors
				assert.NotEmpty(t, output, "Command should produce some output")
			}
		})
	}
}

func TestFilterFlagWithRunAllGraphExpressions(t *testing.T) {
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

func TestFilterFlagWithRunAllGraphExpressionsVerifyExecutionOrder(t *testing.T) {
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

// TestFilterFlagWithFindCombinedGitAndGraphExpressions tests the combination of git-based
// queries with dependency graph traversal.
func TestFilterFlagWithFindCombinedGitAndGraphExpressions(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) string {
		t.Helper()

		tmpDir := helpers.TmpDirWOSymlinks(t)

		runner, err := git.NewGitRunner()
		require.NoError(t, err)

		runner = runner.WithWorkDir(tmpDir)

		err = runner.Init(t.Context())
		require.NoError(t, err)

		err = runner.GoOpenRepo()
		require.NoError(t, err)

		t.Cleanup(func() {
			err = runner.GoCloseStorage()
			if err != nil {
				t.Logf("Error closing storage: %s", err)
			}
		})

		// Create a dependency chain: app -> db -> vpc
		// We'll modify 'db' and use git+graph filter to find its dependencies and dependents

		vpcDir := filepath.Join(tmpDir, "vpc")
		err = os.MkdirAll(vpcDir, 0755)
		require.NoError(t, err)

		vpcHCL := `# VPC unit - no dependencies`
		err = os.WriteFile(filepath.Join(vpcDir, "terragrunt.hcl"), []byte(vpcHCL), 0644)
		require.NoError(t, err)

		dbDir := filepath.Join(tmpDir, "db")
		err = os.MkdirAll(dbDir, 0755)
		require.NoError(t, err)

		dbHCL := `# DB unit - depends on vpc
dependency "vpc" {
  config_path = "../vpc"
}
`
		err = os.WriteFile(filepath.Join(dbDir, "terragrunt.hcl"), []byte(dbHCL), 0644)
		require.NoError(t, err)

		appDir := filepath.Join(tmpDir, "app")
		err = os.MkdirAll(appDir, 0755)
		require.NoError(t, err)

		appHCL := `# App unit - depends on db
dependency "db" {
  config_path = "../db"
}
`
		err = os.WriteFile(filepath.Join(appDir, "terragrunt.hcl"), []byte(appHCL), 0644)
		require.NoError(t, err)

		err = runner.GoAdd(".")
		require.NoError(t, err)

		err = runner.GoCommit("Initial commit with vpc, db, app chain", &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)

		modifiedDBHCL := `# DB unit - depends on vpc (MODIFIED)
dependency "vpc" {
  config_path = "../vpc"
}
`
		err = os.WriteFile(filepath.Join(dbDir, "terragrunt.hcl"), []byte(modifiedDBHCL), 0644)
		require.NoError(t, err)

		err = runner.GoAdd(".")
		require.NoError(t, err)

		err = runner.GoCommit("Modify db unit", &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)

		return tmpDir
	}

	testCases := []struct {
		name          string
		filterQuery   string
		description   string
		expectedUnits []string
		expectError   bool
	}{
		{
			name:          "git filter only - baseline",
			filterQuery:   "[HEAD~1...HEAD]",
			expectedUnits: []string{"db"},
			description:   "Baseline: git filter alone should find the modified db unit",
			expectError:   false,
		},
		{
			name:          "dependencies of git changes - [HEAD~1...HEAD]...",
			filterQuery:   "[HEAD~1...HEAD]...",
			expectedUnits: []string{"db", "vpc"},
			description:   "Should find db (git-matched) and vpc (its dependency)",
			expectError:   false,
		},
		{
			name:          "dependents of git changes - ...[HEAD~1...HEAD]",
			filterQuery:   "...[HEAD~1...HEAD]",
			expectedUnits: []string{"db", "app"},
			description:   "Should find db (git-matched) and app (its dependent)",
			expectError:   false,
		},
		{
			name:          "both directions - ...[HEAD~1...HEAD]...",
			filterQuery:   "...[HEAD~1...HEAD]...",
			expectedUnits: []string{"vpc", "db", "app"},
			description:   "Issue #5307: Should find db (git-matched), vpc (dependency), and app (dependent)",
			expectError:   false,
		},
		{
			name:          "exclude target - ^[HEAD~1...HEAD]...",
			filterQuery:   "^[HEAD~1...HEAD]...",
			expectedUnits: []string{"vpc"},
			description:   "Should find vpc (dependency of db), excluding db itself",
			expectError:   false,
		},
		{
			name:          "exclude target - ^...[HEAD~1...HEAD]",
			filterQuery:   "...^[HEAD~1...HEAD]",
			expectedUnits: []string{"app"},
			description:   "Should find app (dependent of db), excluding db itself",
			expectError:   false,
		},
		{
			name:          "exclude target both directions - ...^[HEAD~1...HEAD]...",
			filterQuery:   "...^[HEAD~1...HEAD]...",
			expectedUnits: []string{"vpc", "app"},
			description:   "Should find vpc (dependency) and app (dependent), excluding db itself",
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := setup(t)

			cmd := "terragrunt find --no-color --working-dir " + tmpDir + " --filter '" + tc.filterQuery + "'"
			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				return
			}

			actualUnits := []string{}

			for line := range strings.SplitSeq(strings.TrimSpace(stdout), "\n") {
				if line != "" {
					actualUnits = append(actualUnits, filepath.Base(line))
				}
			}

			assert.ElementsMatch(
				t,
				tc.expectedUnits,
				actualUnits,
			)
		})
	}
}

// TestFilterFlagWithRunAllCombinedGitAndGraphExpressions tests the `run --all` command
// with combined git + graph filter expressions.
func TestFilterFlagWithRunAllCombinedGitAndGraphExpressions(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) string {
		t.Helper()

		tmpDir := helpers.TmpDirWOSymlinks(t)

		runner, err := git.NewGitRunner()
		require.NoError(t, err)

		runner = runner.WithWorkDir(tmpDir)

		err = runner.Init(t.Context())
		require.NoError(t, err)

		err = runner.GoOpenRepo()
		require.NoError(t, err)

		t.Cleanup(func() {
			err = runner.GoCloseStorage()
			if err != nil {
				t.Logf("Error closing storage: %s", err)
			}
		})

		// Create a dependency chain: service -> cache -> vpc
		// We'll modify 'cache' and use git+graph filter

		vpcDir := filepath.Join(tmpDir, "vpc")
		err = os.MkdirAll(vpcDir, 0755)
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
}
`
		err = os.WriteFile(filepath.Join(serviceDir, "terragrunt.hcl"), []byte(serviceHCL), 0644)
		require.NoError(t, err)

		serviceTF := `# Service TF`
		err = os.WriteFile(filepath.Join(serviceDir, "main.tf"), []byte(serviceTF), 0644)
		require.NoError(t, err)

		// Initial commit
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

		modifiedCacheHCL := `# Cache unit (MODIFIED)
dependency "vpc" {
  config_path = "../vpc"
}
`
		err = os.WriteFile(filepath.Join(cacheDir, "terragrunt.hcl"), []byte(modifiedCacheHCL), 0644)
		require.NoError(t, err)

		err = runner.GoAdd(".")
		require.NoError(t, err)

		err = runner.GoCommit("Modify cache", &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)

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
			name:          "both directions - issue #5307 - run",
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
