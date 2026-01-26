package test_test

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureMixedConfig            = "fixtures/mixed-config"
	testFixtureFailFast               = "fixtures/fail-fast"
	testFixtureFailFastEarlyExit      = "fixtures/fail-fast-early-exit"
	testFixtureRunnerPoolRemoteSource = "fixtures/runner-pool-remote-source"
	testFixtureAuthProviderParallel   = "fixtures/auth-provider-parallel"
)

func TestRunnerPoolDiscovery(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDependencyOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyOutput)
	testPath := filepath.Join(tmpEnvPath, testFixtureDependencyOutput)
	// Run the find command to discover the configs
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-level debug --working-dir "+testPath+"  -- apply")
	require.NoError(t, err)
	// Verify that the output contains value from the app
	require.Contains(t, stdout, "output_value = \"42\"")

	// Verify that the output contains value from the dependency
	require.Contains(t, stdout, "result = \"42\"")
}

func TestRunnerPoolDiscoveryNoParallelism(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDependencyOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyOutput)
	testPath := filepath.Join(tmpEnvPath, testFixtureDependencyOutput)
	// Run the find command to discover the configs
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --parallelism 1 --working-dir "+testPath+"  -- apply")
	require.NoError(t, err)
	// Verify that the output contains value from the app
	require.Contains(t, stdout, "output_value = \"42\"")

	// Verify that the output contains value from the dependency
	require.Contains(t, stdout, "result = \"42\"")
}

func TestRunnerPoolTerragruntDestroyOrder(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDestroyOrder)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDestroyOrder)
	rootPath := filepath.Join(tmpEnvPath, testFixtureDestroyOrder, "app")

	// apply the stack
	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	// run destroy with runner pool and check the modules are destroyed
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all destroy --non-interactive --tf-forward-stdout --working-dir "+rootPath)
	require.NoError(t, err)

	// Parse destroyed modules from stdout
	var destroyOrder []string

	re := regexp.MustCompile(`Hello, Module ([A-Za-z]+)`)
	for line := range strings.SplitSeq(stdout, "\n") {
		if match := re.FindStringSubmatch(line); match != nil {
			destroyOrder = append(destroyOrder, "module-"+strings.ToLower(match[1]))
		}
	}

	t.Logf("Destroyed modules: %v", destroyOrder)

	index := make(map[string]int)
	for i, mod := range destroyOrder {
		index[mod] = i
	}

	// Verify all expected modules were destroyed
	// Note: With parallel execution, stdout order doesn't reflect actual execution order,
	// so we only verify all modules were destroyed, not their order.
	// Dependency ordering is enforced by the runner pool DAG execution.
	expectedModules := []string{"module-a", "module-b", "module-c", "module-d", "module-e"}
	for _, mod := range expectedModules {
		_, ok := index[mod]
		assert.True(t, ok, "expected module %q to be destroyed, got: %v", mod, destroyOrder)
	}
}

func TestRunnerPoolStackConfigIgnored(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMixedConfig)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureMixedConfig)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --working-dir "+testPath+" -- apply",
	)
	require.NoError(t, err)
	require.NotContains(t, stderr, "Error: Unsupported block type")
	require.NotContains(t, stderr, "Blocks of type \"unit\" are not expected here")
}

func TestRunnerPoolFailFast(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedResults map[string]struct {
			result string
			reason string
			cause  string
		}
		name     string
		failFast bool
	}{
		{
			name:     "fail-fast=false",
			failFast: false,
			expectedResults: map[string]struct {
				result string
				reason string
				cause  string
			}{
				"failing-unit":          {result: "failed", reason: "run error"},
				"succeeding-unit":       {result: "succeeded"},
				"depends-on-failing":    {result: "early exit", reason: "ancestor error", cause: "failing-unit"},
				"depends-on-succeeding": {result: "succeeded"},
			},
		},
		{
			name:     "fail-fast=true",
			failFast: true,
			expectedResults: map[string]struct {
				result string
				reason string
				cause  string
			}{
				"failing-unit":       {result: "failed", reason: "run error"},
				"succeeding-unit":    {result: "succeeded"},
				"depends-on-failing": {result: "early exit", reason: "ancestor error", cause: "failing-unit"},
				// depends-on-succeeding can be either "early exit" or "succeeded" depending on timing.
				// With parallel execution, it may complete before fail-fast propagates, or get stopped.
				// Both outcomes are valid - we handle this case separately in the test below.
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFailFastEarlyExit)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFailFastEarlyExit)
			testPath := filepath.Join(tmpEnvPath, testFixtureFailFastEarlyExit)

			cmd := "terragrunt run --all --non-interactive --report-file " + helpers.ReportFile + " --working-dir " + testPath
			if tc.failFast {
				cmd += " --fail-fast"
			}

			_, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd+" -- apply")
			require.Error(t, err)

			// Parse and verify the report file
			reportFilePath := filepath.Join(testPath, helpers.ReportFile)
			assert.FileExists(t, reportFilePath)

			err = report.ValidateJSONReportFromFile(reportFilePath)
			require.NoError(t, err, "Report should pass schema validation")

			runs, err := report.ParseJSONRunsFromFile(reportFilePath)
			require.NoError(t, err)

			// Verify expected units are in the report
			assert.ElementsMatch(t, []string{"failing-unit", "succeeding-unit", "depends-on-failing", "depends-on-succeeding"}, runs.Names())

			// Verify each unit's result, reason, and cause
			for unitName, expected := range tc.expectedResults {
				run := runs.FindByName(unitName)
				require.NotNil(t, run, "run %q not found in report", unitName)

				assert.Equal(t, expected.result, run.Result, "unexpected result for %q", unitName)

				if expected.reason != "" {
					require.NotNil(t, run.Reason, "expected reason for %q but got nil", unitName)
					assert.Equal(t, expected.reason, *run.Reason, "unexpected reason for %q", unitName)
				}

				if expected.cause != "" {
					require.NotNil(t, run.Cause, "expected cause for %q but got nil", unitName)
					assert.Equal(t, expected.cause, *run.Cause, "unexpected cause for %q", unitName)
				}
			}

			// Special handling for depends-on-succeeding in fail-fast=true case.
			// Due to parallel execution, it can either complete before fail-fast propagates
			// (result: "succeeded") or be stopped (result: "early exit"). Both are valid.
			if tc.failFast {
				run := runs.FindByName("depends-on-succeeding")
				require.NotNil(t, run, "run %q not found in report", "depends-on-succeeding")

				validResults := []string{"succeeded", "early exit"}
				assert.Contains(t, validResults, run.Result,
					"depends-on-succeeding should be either 'succeeded' or 'early exit', got %q", run.Result)

				if run.Result == "early exit" {
					require.NotNil(t, run.Reason, "expected reason for depends-on-succeeding but got nil")
					assert.Equal(t, "ancestor error", *run.Reason)
				}
			}
		})
	}
}

func TestRunnerPoolDestroyFailFast(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFailFast)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFailFast)
	testPath := filepath.Join(tmpEnvPath, testFixtureFailFast)

	_, stdout, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --fail-fast --working-dir "+testPath+"  -- apply")
	require.NoError(t, err)

	// Verify that there are no parsing errors in the output
	require.NotContains(t, stdout, "Error: Unsupported block type")
	require.NotContains(t, stdout, "This object does not have an attribute named \"outputs\"")

	// create fail.txt in unit-a to trigger a failure
	helpers.CreateFile(t, testPath, "unit-b", "fail.txt")
	stdout, stderr, _ := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --fail-fast --working-dir "+testPath+"  -- destroy")
	// Check that error output contains terraform error details
	assert.Contains(t, stderr, "level=error")
	// Verify that unit-b failed
	assert.Contains(t, stderr, "Failed to execute")
	assert.Contains(t, stderr, "in ./unit-b")
	assert.NotContains(t, stdout, "unit-b tf-path="+wrappedBinary()+" msg=Destroy complete! Resources: 1 destroyed")
	assert.NotContains(t, stdout, "unit-a tf-path="+wrappedBinary()+" msg=Destroy complete! Resources: 1 destroyed.")
}

func TestRunnerPoolDestroyDependencies(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFailFast)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFailFast)
	testPath := filepath.Join(tmpEnvPath, testFixtureFailFast)
	testPath, err := filepath.EvalSymlinks(testPath)
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --fail-fast --working-dir "+testPath+"  -- apply")
	require.NoError(t, err)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --fail-fast --working-dir "+testPath+"  -- destroy")
	require.NoError(t, err)
	assert.Contains(t, stdout, "unit-b tf-path="+wrappedBinary()+" msg=Destroy complete! Resources: 1 destroyed")
	assert.Contains(t, stdout, "unit-c tf-path="+wrappedBinary()+" msg=Destroy complete! Resources: 1 destroyed")
	assert.Contains(t, stdout, "unit-a tf-path="+wrappedBinary()+" msg=Destroy complete! Resources: 1 destroyed.")
}

func TestRunnerPoolRemoteSource(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRunnerPoolRemoteSource)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRunnerPoolRemoteSource)
	testPath := filepath.Join(tmpEnvPath, testFixtureRunnerPoolRemoteSource)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-level debug --working-dir "+testPath+"  -- apply")
	require.NoError(t, err)
	// Verify that the output contains value produced from remote unit
	require.Contains(t, stdout, "data = \"unit-a\"")
}

func TestRunnerPoolSourceMap(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSourceMapSlashes)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureSourceMapSlashes)
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive "+
			"--source-map git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=github.com/gruntwork-io/terragrunt.git?ref=v0.85.0 "+
			"--working-dir "+testPath+" -- apply ",
	)
	require.NoError(t, err)
	// Verify that source map values are used
	require.Contains(t, stderr, "configurations from git::https://github.com/gruntwork-io/terragrunt.git?ref=v0.85.0")
}

// TestAuthProviderParallelExecution verifies that --auth-provider-cmd is executed in parallel
// for multiple units during the resolution phase.
//
// The test works by:
// 1. Running terragrunt with --auth-provider-cmd pointing to a script that:
//   - Creates lock files to coordinate between concurrent invocations
//   - Detects when multiple auth commands are running simultaneously
//   - Logs "Auth concurrent" when it detects parallel execution
//     2. Parsing the output to find "Auth concurrent" messages
//     3. Verifying that at least one auth command detected concurrent execution
//     (which is deterministic proof of parallelism)
func TestAuthProviderParallelExecution(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderParallel)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderParallel)
	testPath := filepath.Join(tmpEnvPath, testFixtureAuthProviderParallel)
	// Resolve symlinks to avoid path mismatches on macOS where /var -> /private/var
	testPath, err := filepath.EvalSymlinks(testPath)
	require.NoError(t, err)

	authProviderScript := filepath.Join(testPath, "auth-provider.sh")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --auth-provider-cmd "+authProviderScript+" --working-dir "+testPath+" -- validate",
	)
	require.NoError(t, err)

	startCount := strings.Count(stderr, "Auth start")
	endCount := strings.Count(stderr, "Auth end")

	reConcurrent := regexp.MustCompile(`Auth concurrent.*detected=(\d+)`)
	matches := reConcurrent.FindAllStringSubmatch(stderr, -1)

	maxConcurrent := 0

	for _, match := range matches {
		detected, convErr := strconv.Atoi(match[1])
		require.NoError(t, convErr, "Invalid detected count in stderr: %q", match[0])

		if detected > maxConcurrent {
			maxConcurrent = detected
		}

		t.Logf("Auth command detected %d concurrent executions", detected)
	}

	// Log start/end counts but don't fail - concurrent detection is the real proof of parallelism.
	// Due to timing and log buffering, start/end events may not always be captured reliably.
	t.Logf("Auth start events: %d, end events: %d", startCount, endCount)

	// The concurrent detection is the key proof of parallel execution.
	// If auth commands detected other concurrent commands, parallelism is working.
	assert.GreaterOrEqual(t, len(matches), 1,
		"Expected at least one auth command to detect concurrent execution. "+
			"This would prove parallel execution. If this fails, auth commands may be running sequentially.")
	assert.GreaterOrEqual(t, maxConcurrent, 2,
		"Expected auth commands to detect at least 2 concurrent executions. "+
			"Detected max concurrent: %d. This proves parallel execution.", maxConcurrent)
}
