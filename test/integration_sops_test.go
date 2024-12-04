package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureSops        = "fixtures/sops"
	testFixtureSopsErrors  = "fixtures/sops-errors"
	testFixtureSopsMissing = "fixtures/sops-missing"
)

func TestSopsDecryptedCorrectly(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureSops)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSops)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureSops)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, []interface{}{true, false}, outputs["json_bool_array"].Value)
	assert.Equal(t, []interface{}{"example_value1", "example_value2"}, outputs["json_string_array"].Value)
	assert.InEpsilon(t, 1234.56789, outputs["json_number"].Value, 0.0001)
	assert.Equal(t, "example_value", outputs["json_string"].Value)
	assert.Equal(t, "Welcome to SOPS! Edit this file as you please!", outputs["json_hello"].Value)
	assert.Equal(t, []interface{}{true, false}, outputs["yaml_bool_array"].Value)
	assert.Equal(t, []interface{}{"example_value1", "example_value2"}, outputs["yaml_string_array"].Value)
	assert.InEpsilon(t, 1234.5679, outputs["yaml_number"].Value, 0.0001)
	assert.Equal(t, "example_value", outputs["yaml_string"].Value)
	assert.Equal(t, "Welcome to SOPS! Edit this file as you please!", outputs["yaml_hello"].Value)
	assert.Equal(t, "Raw Secret Example", outputs["text_value"].Value)
	assert.Contains(t, outputs["env_value"].Value, "DB_PASSWORD=tomato")
	assert.Contains(t, outputs["ini_value"].Value, "password = potato")
}

func TestSopsDecryptedCorrectlyRunAll(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureSops)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSops)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureSops)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s/../.. --terragrunt-include-dir %s", rootPath, testFixtureSops))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt run-all output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s/../.. --terragrunt-include-dir %s", rootPath, testFixtureSops), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, []interface{}{true, false}, outputs["json_bool_array"].Value)
	assert.Equal(t, []interface{}{"example_value1", "example_value2"}, outputs["json_string_array"].Value)
	assert.InEpsilon(t, 1234.56789, outputs["json_number"].Value, 0.0001)
	assert.Equal(t, "example_value", outputs["json_string"].Value)
	assert.Equal(t, "Welcome to SOPS! Edit this file as you please!", outputs["json_hello"].Value)
	assert.Equal(t, []interface{}{true, false}, outputs["yaml_bool_array"].Value)
	assert.Equal(t, []interface{}{"example_value1", "example_value2"}, outputs["yaml_string_array"].Value)
	assert.InEpsilon(t, 1234.5679, outputs["yaml_number"].Value, 0.0001)
	assert.Equal(t, "example_value", outputs["yaml_string"].Value)
	assert.Equal(t, "Welcome to SOPS! Edit this file as you please!", outputs["yaml_hello"].Value)
	assert.Equal(t, "Raw Secret Example", outputs["text_value"].Value)
	assert.Contains(t, outputs["env_value"].Value, "DB_PASSWORD=tomato")
	assert.Contains(t, outputs["ini_value"].Value, "password = potato")
}

func TestTerragruntLogSopsErrors(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSopsErrors)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureSopsErrors)

	// apply and check for errors
	_, errorOut, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+testPath)
	require.Error(t, err)

	assert.Contains(t, errorOut, "error decrypting key: [error decrypting key")
	assert.Contains(t, errorOut, "error base64-decoding encrypted data key: illegal base64 data at input byte")
}

func TestSopsDecryptOnMissing(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureSopsMissing)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSopsMissing)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureSopsMissing)

	// apply and check for errors
	_, errorOut, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
	require.Error(t, err)

	assert.Contains(t, errorOut, "Encountered error while evaluating locals in file ./terragrunt.hcl")
	assert.Contains(t, errorOut, "./missing.yaml: no such file")

}
