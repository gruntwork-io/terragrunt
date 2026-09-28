//go:build tf

package test_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

func TestTFFeatureFlagDefaults(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleFlag)
	rootPath := filepath.Join(tmpEnvPath, testSimpleFlag)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	validateOutputs(t, rootPath)
}

func TestTFFeatureFlagCli(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleFlag)
	rootPath := filepath.Join(tmpEnvPath, testSimpleFlag)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --feature int_feature_flag=777 --feature bool_feature_flag=true --feature string_feature_flag=tomato --non-interactive --working-dir "+rootPath,
	)

	expected := expectedDefaults()
	expected["int_feature_flag"] = 777
	expected["bool_feature_flag"] = true
	expected["string_feature_flag"] = "tomato"
	validateOutputsMap(t, rootPath, expected)
}

func TestTFFeatureApplied(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleFlag)
	rootPath := filepath.Join(tmpEnvPath, testSimpleFlag)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --feature bool_feature_flag=true --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "running conditional bool_feature_flag")

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --feature bool_feature_flag=false --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)
	assert.NotContains(t, stdout, "running conditional bool_feature_flag")
}

func TestTFFeatureFlagEnv(t *testing.T) {
	t.Setenv("TG_FEATURE", "int_feature_flag=111,bool_feature_flag=true,string_feature_flag=xyz")

	helpers.CleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleFlag)
	rootPath := filepath.Join(tmpEnvPath, testSimpleFlag)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	expected := expectedDefaults()
	expected["int_feature_flag"] = 111
	expected["bool_feature_flag"] = true
	expected["string_feature_flag"] = "xyz"
	validateOutputsMap(t, rootPath, expected)
}

func TestTFFeatureIncludeFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testIncludeFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testIncludeFlag)
	rootPath := filepath.Join(tmpEnvPath, testIncludeFlag, "app")

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	validateOutputs(t, rootPath)
}

func TestTFFeatureFlagRunAll(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testRunAllFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testRunAllFlag)
	rootPath := filepath.Join(tmpEnvPath, testRunAllFlag)
	app1 := filepath.Join(tmpEnvPath, testRunAllFlag, "app1")
	app2 := filepath.Join(tmpEnvPath, testRunAllFlag, "app2")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	validateOutputs(t, app1)
	validateOutputs(t, app2)
}
