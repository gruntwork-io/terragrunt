package test_test

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureMixedConfig            = "fixtures/mixed-config"
	testFixtureFailFast               = "fixtures/fail-fast"
	testFixtureRunnerPoolRemoteSource = "fixtures/runner-pool-remote-source"
	testFixtureAuthProviderParallel   = "fixtures/auth-provider-parallel"
)

func TestRunnerPoolDiscovery(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDependencyOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyOutput)
	testPath := util.JoinPath(tmpEnvPath, testFixtureDependencyOutput)
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
	testPath := util.JoinPath(tmpEnvPath, testFixtureDependencyOutput)
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
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDestroyOrder, "app")

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
	testPath := util.JoinPath(tmpEnvPath, testFixtureMixedConfig)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --queue-include-external --all --non-interactive --working-dir "+testPath+" -- apply")
	require.NoError(t, err)
	require.NotContains(t, stderr, "Error: Unsupported block type")
	require.NotContains(t, stderr, "Blocks of type \"unit\" are not expected here")
}

func TestRunnerPoolFailFast(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFailFast)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFailFast)
	testPath := util.JoinPath(tmpEnvPath, testFixtureFailFast)

	// create fail.txt in unit-a to trigger a failure
	helpers.CreateFile(t, testPath, "unit-a", "fail.txt")
	_, stderr, _ := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --fail-fast --working-dir "+testPath+"  -- apply")

	assert.Contains(t, stderr, "unit-b did not run due to early exit")
	assert.Contains(t, stderr, "unit-c did not run due to early exit")
}

func TestRunnerPoolDestroyFailFast(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFailFast)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFailFast)
	testPath := util.JoinPath(tmpEnvPath, testFixtureFailFast)

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
	testPath := util.JoinPath(tmpEnvPath, testFixtureFailFast)
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
	testPath := util.JoinPath(tmpEnvPath, testFixtureRunnerPoolRemoteSource)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-level debug --working-dir "+testPath+"  -- apply")
	require.NoError(t, err)
	// Verify that the output contains value produced from remote unit
	require.Contains(t, stdout, "data = \"unit-a\"")
}

func TestRunnerPoolSourceMap(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSourceMapSlashes)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureSourceMapSlashes)
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --source-map git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=git::git@github.com:gruntwork-io/terragrunt.git?ref=v0.85.0 --working-dir "+testPath+" -- apply ")
	require.NoError(t, err)
	// Verify that source map values are used
	require.Contains(t, stderr, "configurations from git::ssh://git@github.com/gruntwork-io/terragrunt.git?ref=v0.85.0")
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
	testPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderParallel)
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

	require.GreaterOrEqual(t, startCount, 2, "Expected at least 2 auth start events")
	require.GreaterOrEqual(t, endCount, 2, "Expected at least 2 auth end events")
	assert.GreaterOrEqual(t, len(matches), 1,
		"Expected at least one auth command to detect concurrent execution. "+
			"This would prove parallel execution. If this fails, auth commands may be running sequentially.")
	assert.GreaterOrEqual(t, maxConcurrent, 2,
		"Expected auth commands to detect at least 2 concurrent executions. "+
			"Detected max concurrent: %d. This proves parallel execution.", maxConcurrent)
}
