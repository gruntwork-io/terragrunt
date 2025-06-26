package test_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

func TestRunnerPoolDiscovery(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDependencyOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyOutput)
	testPath := util.JoinPath(tmpEnvPath, testFixtureDependencyOutput)
	// Run the find command to discover the configs
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-level debug --experiment runner-pool --working-dir "+testPath+"  -- apply")
	fmt.Printf("error: %v\n", err)
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
	for _, line := range strings.Split(stdout, "\n") {
		if match := re.FindStringSubmatch(line); match != nil {
			destroyOrder = append(destroyOrder, "module-"+strings.ToLower(match[1]))
		}
	}
	t.Logf("Actual destroy order: %v", destroyOrder)

	index := make(map[string]int)
	for i, mod := range destroyOrder {
		index[mod] = i
	}

	// module-a must be destroyed before module-b
	assert.Less(t, index["module-a"], index["module-b"], "module-a should be destroyed before module-b")
	// module-c must be destroyed before module-d
	assert.Less(t, index["module-c"], index["module-d"], "module-c should be destroyed before module-d")
	// module-e must be destroyed before module-d
	assert.Less(t, index["module-e"], index["module-d"], "module-e should be destroyed before module-d")
}
