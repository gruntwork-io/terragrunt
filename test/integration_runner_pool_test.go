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
// Root cause: When a nested stack contains a `dependencies` block pointing to another stack,
// only the FIRST unit in that nested stack is included in the runner pool.
// All other units in the stack are generated but NOT executed.
//
// Test fixture structure:
// - Top-level stack with 2 units: id, ecr-cache
// - Nested stack "network" with 4 units: vpc, tailscale-router, vpc-endpoints, vpc-nat
// - Nested stack "k8s" with 5 units AND a dependencies block: eks-cluster, eks-baseline, grafana-baseline, rancher-bootstrap, rancher-baseline
//
// Expected behavior (bug): Only eks-cluster from k8s stack is included, 4 others are missing
// This test FAILS when the bug exists, demonstrating it needs to be fixed.
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

	// The bug symptom according to issue #4977:
	// 1. All units are GENERATED (shown in "Generating unit X from" messages)
	// 2. But NOT all units are INCLUDED in runner pool execution
	// 3. Specifically, 4 units from k8s nested stack should be missing
	// 4. A "Cycle detected in dependency graph" warning may appear

	// Count how many units are generated
	generatedUnitsCount := 0
	generatingUnits := []string{
		"Generating unit id from",
		"Generating unit ecr-cache from",
		"Generating unit vpc from",
		"Generating unit tailscale-router from",
		"Generating unit vpc-endpoints from",
		"Generating unit vpc-nat from",
		"Generating unit eks-cluster from",
		"Generating unit eks-baseline from",
		"Generating unit grafana-baseline from",
		"Generating unit rancher-bootstrap from",
		"Generating unit rancher-baseline from",
	}

	for _, msg := range generatingUnits {
		if strings.Contains(stderr, msg) {
			generatedUnitsCount++
		}
	}

	// All 11 units should be generated
	assert.Equal(t, 11, generatedUnitsCount, "All 11 units should be generated")

	// Check for the units that should be missing according to bug report
	criticalUnits := []string{
		".terragrunt-stack/k8s/.terragrunt-stack/eks-baseline",
		".terragrunt-stack/k8s/.terragrunt-stack/grafana-baseline",
		".terragrunt-stack/k8s/.terragrunt-stack/rancher-bootstrap",
		".terragrunt-stack/k8s/.terragrunt-stack/rancher-baseline",
	}

	missingCount := 0

	for _, unit := range criticalUnits {
		if !strings.Contains(stderr, unit) {
			t.Logf("Unit missing from runner pool (expected if bug exists): %s", unit)

			missingCount++
		}
	}

	// EXPECTED BEHAVIOR IF BUG EXISTS: missingCount should be 4
	// ACTUAL BEHAVIOR: missingCount is 0 (all units are included)

	// Bug #4977: When a nested stack has a dependencies block pointing to another stack,
	// only the first unit in the nested stack is included in the runner pool.
	// The remaining units are generated but not executed.

	if missingCount > 0 {
		// Bug is reproduced - units are missing from runner pool
		t.Errorf("BUG #4977 REPRODUCED: %d units were generated but NOT included in runner pool execution:", missingCount)

		for _, unit := range criticalUnits {
			if !strings.Contains(stderr, unit) {
				t.Errorf("  - Missing unit: %s", unit)
			}
		}

		t.Fatalf("Bug #4977 exists: Nested stack with dependencies block causes units to be excluded from runner pool. "+
			"All 11 units are generated, but only %d are included in execution. "+
			"This is caused by adding 'dependencies { paths = [\"../network\"] }' to the k8s stack.",
			11-missingCount)
	}

	// If we reach here, bug is fixed - all units are included
	require.NoError(t, err)
	t.Logf("Bug #4977 is FIXED: All %d generated units are correctly included in runner pool", generatedUnitsCount)
}
