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
	testFixtureMixedConfig = "fixtures/mixed-config"
	testFixtureFailFast    = "fixtures/fail-fast"
)

func TestRunnerPoolDiscovery(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDependencyOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyOutput)
	testPath := util.JoinPath(tmpEnvPath, testFixtureDependencyOutput)
	// Run the find command to discover the configs
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-level debug --experiment runner-pool --working-dir "+testPath+"  -- apply")
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
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --experiment runner-pool --parallelism 1 --working-dir "+testPath+"  -- apply")
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
	helpers.RunTerragrunt(t, "terragrunt run --experiment runner-pool --all apply --non-interactive --working-dir "+rootPath)

	// run destroy with runner pool and check the order of the modules
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --experiment runner-pool --all destroy --non-interactive --tf-forward-stdout --working-dir "+rootPath)
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

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --experiment runner-pool --queue-include-external --all --non-interactive --working-dir "+testPath+" -- apply")
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
	_, stderr, _ := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --experiment runner-pool --fail-fast --working-dir "+testPath+"  -- apply")

	assert.Contains(t, stderr, "unit-b did not run due to early exit")
	assert.Contains(t, stderr, "unit-c did not run due to early exit")
}
