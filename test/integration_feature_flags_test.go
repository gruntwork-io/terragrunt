package test_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/util"
)

const (
	testSimpleFlag  = "fixtures/feature-flags/simple-flag"
	testIncludeFlag = "fixtures/feature-flags/include-flag"
	testRunAllFlag  = "fixtures/feature-flags/run-all"
)

func TestFeatureFlagDefaults(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := copyEnvironment(t, testSimpleFlag)
	rootPath := util.JoinPath(tmpEnvPath, testSimpleFlag)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	validateOutputs(t, rootPath)
}

func TestFeatureFlagCli(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := copyEnvironment(t, testSimpleFlag)
	rootPath := util.JoinPath(tmpEnvPath, testSimpleFlag)

	runTerragrunt(t, "terragrunt apply -auto-approve --feature int_feature_flag=777 --feature bool_feature_flag=true --feature string_feature_flag=tomato --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	expected := expectedDefaults()
	expected["int_feature_flag"] = 777
	expected["bool_feature_flag"] = true
	expected["string_feature_flag"] = "tomato"
	validateOutputsMap(t, rootPath, expected)
}

func TestFeatureApplied(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := copyEnvironment(t, testSimpleFlag)
	rootPath := util.JoinPath(tmpEnvPath, testSimpleFlag)

	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --feature bool_feature_flag=true --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "running conditional bool_feature_flag")

	stdout, _, err = runTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --feature bool_feature_flag=false --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)
	assert.NotContains(t, stdout, "running conditional bool_feature_flag")
}

func TestFeatureFlagEnv(t *testing.T) {
	t.Setenv("TERRAGRUNT_FEATURE", "int_feature_flag=111,bool_feature_flag=true,string_feature_flag=xyz")

	cleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := copyEnvironment(t, testSimpleFlag)
	rootPath := util.JoinPath(tmpEnvPath, testSimpleFlag)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	expected := expectedDefaults()
	expected["int_feature_flag"] = 111
	expected["bool_feature_flag"] = true
	expected["string_feature_flag"] = "xyz"
	validateOutputsMap(t, rootPath, expected)
}

func TestFeatureIncludeFlag(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testIncludeFlag)
	tmpEnvPath := copyEnvironment(t, testIncludeFlag)
	rootPath := util.JoinPath(tmpEnvPath, testIncludeFlag, "app")

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	validateOutputs(t, rootPath)
}

func TestFeatureFlagRunAll(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testRunAllFlag)
	tmpEnvPath := copyEnvironment(t, testRunAllFlag)
	rootPath := util.JoinPath(tmpEnvPath, testRunAllFlag)
	app1 := util.JoinPath(tmpEnvPath, testRunAllFlag, "app1")
	app2 := util.JoinPath(tmpEnvPath, testRunAllFlag, "app2")

	runTerragrunt(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	validateOutputs(t, app1)
	validateOutputs(t, app2)
}

func expectedDefaults() map[string]interface{} {
	return map[string]interface{}{
		"string_feature_flag": "test",
		"int_feature_flag":    666,
		"bool_feature_flag":   false,
	}
}

func validateOutputs(t *testing.T, rootPath string) {
	t.Helper()
	validateOutputsMap(t, rootPath, expectedDefaults())
}

func validateOutputsMap(t *testing.T, rootPath string, expected map[string]interface{}) {
	t.Helper()
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	cmd := "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir " + rootPath
	err := runTerragruntCommand(t, cmd, &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	// Validate outputs against expected values
	for key, expected := range expected {
		assert.EqualValues(t, expected, outputs[key].Value)
	}
}
