package test_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	testSimpleFlag     = "fixtures/feature-flags/simple-flag"
	testIncludeFlag    = "fixtures/feature-flags/include-flag"
	testRunAllFlag     = "fixtures/feature-flags/run-all"
	testRunAllIsolated = "fixtures/feature-flags/run-all-isolated-defaults"
	testErrorEmptyFlag = "fixtures/feature-flags/error-empty-flag"
)

func TestFeatureFlagDefaults(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleFlag)
	rootPath := filepath.Join(tmpEnvPath, testSimpleFlag)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	validateOutputs(t, rootPath)
}

func TestFeatureFlagCli(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleFlag)
	rootPath := filepath.Join(tmpEnvPath, testSimpleFlag)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --feature int_feature_flag=777 --feature bool_feature_flag=true --feature string_feature_flag=tomato --non-interactive --working-dir "+rootPath)

	expected := expectedDefaults()
	expected["int_feature_flag"] = 777
	expected["bool_feature_flag"] = true
	expected["string_feature_flag"] = "tomato"
	validateOutputsMap(t, rootPath, expected)
}

func TestFeatureApplied(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleFlag)
	rootPath := filepath.Join(tmpEnvPath, testSimpleFlag)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --feature bool_feature_flag=true --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "running conditional bool_feature_flag")

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --feature bool_feature_flag=false --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)
	assert.NotContains(t, stdout, "running conditional bool_feature_flag")
}

func TestFeatureFlagEnv(t *testing.T) {
	t.Setenv("TG_FEATURE", "int_feature_flag=111,bool_feature_flag=true,string_feature_flag=xyz")

	helpers.CleanupTerraformFolder(t, testSimpleFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleFlag)
	rootPath := filepath.Join(tmpEnvPath, testSimpleFlag)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	expected := expectedDefaults()
	expected["int_feature_flag"] = 111
	expected["bool_feature_flag"] = true
	expected["string_feature_flag"] = "xyz"
	validateOutputsMap(t, rootPath, expected)
}

func TestFeatureIncludeFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testIncludeFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testIncludeFlag)
	rootPath := filepath.Join(tmpEnvPath, testIncludeFlag, "app")

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	validateOutputs(t, rootPath)
}

func TestFeatureFlagRunAll(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testRunAllFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testRunAllFlag)
	rootPath := filepath.Join(tmpEnvPath, testRunAllFlag)
	app1 := filepath.Join(tmpEnvPath, testRunAllFlag, "app1")
	app2 := filepath.Join(tmpEnvPath, testRunAllFlag, "app2")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	validateOutputs(t, app1)
	validateOutputs(t, app2)
}

// TestFeatureFlagRunAllIsolatesPerUnitDefaults verifies run --all keeps feature defaults scoped to each unit.
func TestFeatureFlagRunAllIsolatesPerUnitDefaults(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testRunAllIsolated)
	tmpEnvPath := helpers.CopyEnvironment(t, testRunAllIsolated)
	rootPath := filepath.Join(tmpEnvPath, testRunAllIsolated)
	livePath := filepath.Join(rootPath, "live")
	targetPath := filepath.Join(livePath, "target-service")
	peerPath := filepath.Join(livePath, "peer-service")
	tfPath := filepath.Join(rootPath, "fake-tf.sh")

	require.NoError(t, os.Chmod(tfPath, 0o755))

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --no-color --inputs-debug --tf-path "+tfPath+" --working-dir "+targetPath,
	)
	require.NoError(t, err)

	targetDirect := readDebugInputs(t, targetPath)
	assert.EqualValues(t, true, targetDirect["effective_toggle"])
	assert.EqualValues(t, true, targetDirect["raw_toggle"])

	removeDebugInputs(t, targetPath, peerPath)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --no-color --parallelism 1 --inputs-debug --tf-path "+tfPath+" --working-dir "+livePath+" -- plan",
	)
	require.NoError(t, err)

	assertDebugInputs(t, targetPath, true, true)
	assertDebugInputs(t, peerPath, false, false)

	removeDebugInputs(t, targetPath, peerPath)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --no-color --parallelism 1 --inputs-debug --feature toggle=true --tf-path "+tfPath+" --working-dir "+livePath+" -- plan",
	)
	require.NoError(t, err)

	assertDebugInputs(t, targetPath, true, true)
	assertDebugInputs(t, peerPath, true, false)

	removeDebugInputs(t, targetPath, peerPath)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --no-color --parallelism 1 --inputs-debug --feature toggle=false --tf-path "+tfPath+" --working-dir "+livePath+" -- plan",
	)
	require.NoError(t, err)

	assertDebugInputs(t, targetPath, false, true)
	assertDebugInputs(t, peerPath, false, false)
}

func TestFailOnEmptyFeatureFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testErrorEmptyFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testErrorEmptyFlag)
	rootPath := filepath.Join(tmpEnvPath, testErrorEmptyFlag)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)
	require.Error(t, err)

	message := err.Error()
	assert.Contains(t, message, "feature flag test1 does not have a default value")
	assert.Contains(t, message, "feature flag test2 does not have a default value")
	assert.Contains(t, message, "feature flag test3 does not have a default value")
}

func expectedDefaults() map[string]any {
	return map[string]any{
		"string_feature_flag": "test",
		"int_feature_flag":    666,
		"bool_feature_flag":   false,
	}
}

func validateOutputs(t *testing.T, rootPath string) {
	t.Helper()
	validateOutputsMap(t, rootPath, expectedDefaults())
}

func validateOutputsMap(t *testing.T, rootPath string, expected map[string]any) {
	t.Helper()

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	cmd := "terragrunt output -no-color -json --non-interactive --working-dir " + rootPath
	err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	// Validate outputs against expected values
	for key, expected := range expected {
		assert.EqualValues(t, expected, outputs[key].Value) //nolint:testifylint
	}
}

// assertDebugInputs verifies the effective and raw toggle values in Terragrunt debug inputs.
func assertDebugInputs(t *testing.T, rootPath string, expectedEffective bool, expectedRaw bool) {
	t.Helper()

	inputs := readDebugInputs(t, rootPath)

	assert.EqualValues(t, expectedEffective, inputs["effective_toggle"])
	assert.EqualValues(t, expectedRaw, inputs["raw_toggle"])
}

// readDebugInputs reads Terragrunt's inputs debug file from the given unit path.
func readDebugInputs(t *testing.T, rootPath string) map[string]any {
	t.Helper()

	debugBytes, err := os.ReadFile(filepath.Join(rootPath, helpers.TerragruntDebugFile))
	require.NoError(t, err)

	inputs := map[string]any{}
	require.NoError(t, json.Unmarshal(debugBytes, &inputs))

	return inputs
}

// removeDebugInputs removes generated Terragrunt inputs debug files from unit paths.
func removeDebugInputs(t *testing.T, paths ...string) {
	t.Helper()

	for _, path := range paths {
		debugPath := filepath.Join(path, helpers.TerragruntDebugFile)
		if err := os.Remove(debugPath); err != nil && !os.IsNotExist(err) {
			require.NoError(t, err)
		}
	}
}
