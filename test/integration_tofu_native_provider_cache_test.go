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
	testFixtureAutoProviderCacheDir = "fixtures/auto-provider-cache-dir"
)

func TestAutoProviderCacheDirExperimentBasic(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAutoProviderCacheDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAutoProviderCacheDir)
	unitPath := util.JoinPath(testPath, "basic", "unit")

	cmd := "terragrunt init --log-level debug --experiment auto-provider-cache-dir --non-interactive --working-dir " + unitPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Regexp(t, `Using hashicorp\/null [^ ]+ from the shared cache directory`, stdout)
	assert.Contains(t, stderr, "Auto provider cache dir enabled")
}

func TestAutoProviderCacheDirExperimentRunAll(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAutoProviderCacheDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAutoProviderCacheDir)
	unitPath := util.JoinPath(testPath, "basic", "unit")

	// clone the unit dir 9 times
	for i := range 9 {
		helpers.CopyDir(t, unitPath, util.JoinPath(testPath, "unit-"+strconv.Itoa(i)))
	}

	cmd := "terragrunt run --all init --log-level debug --experiment auto-provider-cache-dir --non-interactive --working-dir " + testPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Regexp(t, `Using hashicorp\/null [^ ]+ from the shared cache directory`, stdout)
	assert.Contains(t, stderr, "Auto provider cache dir enabled")
}

func TestAutoProviderCacheDirDisabled(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAutoProviderCacheDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAutoProviderCacheDir)
	unitPath := util.JoinPath(testPath, "basic", "unit")

	cmd := "terragrunt init --log-level debug --experiment auto-provider-cache-dir --no-auto-provider-cache-dir --non-interactive --working-dir " + unitPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.NotContains(t, stderr, "Auto provider cache dir enabled")
	assert.NotRegexp(t, `Using hashicorp\/null [^ ]+ from the shared cache directory`, stdout)
}
