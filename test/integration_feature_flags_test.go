package test_test

import (
	"bytes"
	"encoding/json"
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
	testErrorEmptyFlag = "fixtures/feature-flags/error-empty-flag"
)

func TestFailOnEmptyFeatureFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testErrorEmptyFlag)
	tmpEnvPath := helpers.CopyEnvironment(t, testErrorEmptyFlag)
	rootPath := filepath.Join(tmpEnvPath, testErrorEmptyFlag)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)
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
		assert.EqualValues(t, expected, outputs[key].Value)
	}
}
