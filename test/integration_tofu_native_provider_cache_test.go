//go:build tofu

package test_test

import (
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureNativeProviderCache = "fixtures/native-provider-cache"
)

func TestNativeProviderCacheExperimentBasic(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNativeProviderCache)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureNativeProviderCache)
	unitPath := util.JoinPath(testPath, "unit")

	cmd := "terragrunt init --log-level debug --experiment native-provider-cache --non-interactive --working-dir " + unitPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Regexp(t, `Using hashicorp\/null [^ ]+ from the shared cache directory`, stdout)
	assert.Contains(t, stderr, "Native provider cache enabled")
}

func TestNativeProviderCacheExperimentRunAll(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNativeProviderCache)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureNativeProviderCache)
	unitPath := util.JoinPath(testPath, "unit")

	// clone the unit dir 9 times
	for i := range 9 {
		helpers.CopyDir(t, unitPath, util.JoinPath(testPath, "unit-"+strconv.Itoa(i)))
	}

	cmd := "terragrunt run --all init --experiment native-provider-cache --non-interactive --working-dir " + testPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Regexp(t, `Using hashicorp\/null [^ ]+ from the shared cache directory`, stdout)
	assert.Contains(t, stderr, "Native provider cache enabled")
}
