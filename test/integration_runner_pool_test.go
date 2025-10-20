package test_test

import (
	"regexp"
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
	testFixtureNestedStacksRunnerPool = "fixtures/nested-stacks-runner-pool"
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

	// run destroy with runner pool and check the order of the modules
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all destroy --non-interactive --tf-forward-stdout --working-dir "+rootPath)
	require.NoError(t, err)

	// Parse the destruction order from stdout
	var destroyOrder []string

	re := regexp.MustCompile(`Hello, Module ([A-Za-z]+)`)
	for line := range strings.SplitSeq(stdout, "\n") {
		if match := re.FindStringSubmatch(line); match != nil {
			destroyOrder = append(destroyOrder, "module-"+strings.ToLower(match[1]))
		}
	}

	t.Logf("Actual destroy order: %v", destroyOrder)

	index := make(map[string]int)
	for i, mod := range destroyOrder {
		index[mod] = i
	}

	// Assert the new destroy order: module-b < module-d < module-e < module-a < module-c
	assert.Less(t, index["module-b"], index["module-a"], "module-b should be destroyed before module-a")
	assert.Less(t, index["module-b"], index["module-c"], "module-b should be destroyed before module-c")
	assert.Less(t, index["module-e"], index["module-c"], "module-e should be destroyed before module-c")
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
	assert.Contains(t, stderr, "invocation failed in ./unit-b")
	assert.NotContains(t, stdout, "unit-b tf-path="+wrappedBinary()+" msg=Destroy complete! Resources: 1 destroyed")
	assert.NotContains(t, stdout, "unit-a tf-path="+wrappedBinary()+" msg=Destroy complete! Resources: 1 destroyed.")
}

func TestRunnerPoolDestroyDependencies(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFailFast)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFailFast)
	testPath := util.JoinPath(tmpEnvPath, testFixtureFailFast)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --fail-fast --working-dir "+testPath+"  -- apply")
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

// TestRunnerPoolNestedStacksBug reproduces and tracks bug #4977.
// https://github.com/gruntwork-io/terragrunt/issues/4977
//
// BUG DESCRIPTION:
// When units in a nested stack have dependencies passed via `values` that point to units
// in other stacks, those dependencies are NOT discovered by terragrunt. This causes the
// runner pool to execute units in the wrong order, ignoring the dependency relationships.
//
// Test fixture structure:
//   - Top-level stack with 2 units: id, ecr-cache
//   - Nested stack "network" with 4 units: vpc, tailscale-router, vpc-endpoints, vpc-nat
//   - Nested stack "k8s" with 5 units that should depend on network units via values:
//     eks-cluster, eks-baseline, grafana-baseline, rancher-bootstrap, rancher-baseline
//
// The k8s units have `dependencies { paths = try(values.dependencies, []) }` in their terragrunt.hcl
// and receive network unit paths via values from the parent stack.
//
// EXPECTED BEHAVIOR:
// 1. Network units run first
// 2. After network units complete, k8s units can run (respecting dependencies)
//
// ACTUAL BEHAVIOR (BUG):
// All units run in parallel - the dependencies passed via values are ignored!
// This means k8s units start before network units complete, violating dependencies.
func TestRunnerPoolNestedStacksBug(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacksRunnerPool)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacksRunnerPool)
	testPath := util.JoinPath(tmpEnvPath, testFixtureNestedStacksRunnerPool)

	// Run with --queue-include-external to ensure all units are discovered
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --queue-include-external --log-level debug --working-dir "+testPath+" -- plan",
	)
	require.NoError(t, err)

	// Parse the runner pool execution to check if dependencies are respected
	// Extract which units started in the first batch (before any completions)

	lines := strings.Split(stderr, "\n")
	var firstBatch []string
	var foundStart bool
	inFirstBatch := true

	for _, line := range lines {
		if strings.Contains(line, "Runner Pool Controller: starting with") {
			foundStart = true
			continue
		}
		if foundStart && inFirstBatch {
			if strings.Contains(line, "Runner Pool Controller: running") {
				// Extract the unit path
				parts := strings.Split(line, "Runner Pool Controller: running ")
				if len(parts) == 2 {
					unitPath := strings.TrimSpace(parts[1])
					firstBatch = append(firstBatch, unitPath)
				}
			}
			// Stop collecting after we see the first "succeeded" or "found 0 readyEntries"
			if strings.Contains(line, "succeeded") || strings.Contains(line, "found 0 readyEntries") {
				inFirstBatch = false
			}
		}
	}

	t.Logf("Units that started in first batch: %v", firstBatch)

	// BUG CHECK: The k8s units should NOT be in the first batch because they depend on network units
	// If they ARE in the first batch, the bug exists (dependencies are being ignored)

	k8sUnitsInFirstBatch := []string{}
	for _, unit := range firstBatch {
		if strings.Contains(unit, "/k8s/") && (strings.Contains(unit, "eks-") || strings.Contains(unit, "grafana-") || strings.Contains(unit, "rancher-")) {
			k8sUnitsInFirstBatch = append(k8sUnitsInFirstBatch, unit)
		}
	}

	if len(k8sUnitsInFirstBatch) > 0 {
		t.Errorf("BUG #4977 REPRODUCED: K8s units started before their network dependencies completed!")
		t.Errorf("K8s units that incorrectly ran in first batch (should wait for network): %v", k8sUnitsInFirstBatch)
		t.Fatalf("\nExpected behavior: Network units run first, then k8s units\n" +
			"Actual behavior: K8s units ran immediately, ignoring dependencies\n\n" +
			"Root cause: Dependencies passed via 'values' in nested stacks (dependencies { paths = try(values.dependencies, []) }) " +
			"are not being discovered during the dependency discovery phase. The runner pool treats these units as having no dependencies.")
	}

	// If we reach here, dependencies are being properly discovered and respected
	t.Log("SUCCESS: Dependencies passed via values are properly discovered and execution order is correct")
}
