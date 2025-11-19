//go:build tofu

package test_test

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureAutoProviderCacheDir = "fixtures/auto-provider-cache-dir"
	testFixtureTfPathDependency     = "fixtures/tf-path/dependency"
)

func TestAutoProviderCacheDirExperimentBasic(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAutoProviderCacheDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAutoProviderCacheDir)
	unitPath := util.JoinPath(testPath, "basic", "unit")

	cmd := "terragrunt init --log-level debug --non-interactive --working-dir " + unitPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Contains(t, stderr, "using cache key for version files")
	assert.Contains(t, stderr, "Auto provider cache dir enabled")
	assert.Regexp(t, `(Reusing previous version|shared cache directory)`, stdout)
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

	cmd := "terragrunt run --all init --log-level debug --non-interactive --working-dir " + testPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Auto provider cache dir enabled")
	assert.Contains(t, stderr, "using cache key for version files")
	assert.Regexp(t, `(Reusing previous version|shared cache directory)`, stdout)
}

func TestAutoProviderCacheDirDisabled(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAutoProviderCacheDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAutoProviderCacheDir)
	unitPath := util.JoinPath(testPath, "basic", "unit")

	cmd := "terragrunt init --log-level debug --no-auto-provider-cache-dir --non-interactive --working-dir " + unitPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.NotContains(t, stderr, "Auto provider cache dir enabled")
	assert.NotRegexp(t, `Using hashicorp\/null [^ ]+ from the shared cache directory`, stdout)
}

func TestTfPathRespectedForDependencies(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureTfPathDependency)
	rootPath := helpers.CopyEnvironment(t, testFixtureTfPathDependency)
	testPath := util.JoinPath(rootPath, testFixtureTfPathDependency)
	testPath, err := filepath.EvalSymlinks(testPath)
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all --non-interactive --tf-path %s --working-dir %s -- apply",
			util.JoinPath(testPath, "custom-tf.sh"),
			testPath,
		),
	)
	require.NoError(
		t,
		err,
		"Expected tf-path to be respected for dependency lookups, but it was overridden by terraform_binary in config",
	)
	assert.Contains(t, stderr, "Custom TF script used in ./app")
	assert.Contains(t, stderr, "Custom TF script used in ./dep")
}
