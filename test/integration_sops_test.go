//go:build sops

// sops tests assume that you're going to import the test_pgp_key.asc file into your GPG keyring before
// running the tests. We're not gonna assume that everyone is going to do this, so we're going to skip
// these tests by default.
//
// You can import the key by running the following command:
//
//	gpg --import --no-tty --batch --yes ./test/fixtures/sops/test_pgp_key.asc

package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
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

func TestSOPSDecryptedCorrectly(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureSops)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSops)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureSops)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, []any{true, false}, outputs["json_bool_array"].Value)
	assert.Equal(t, []any{"example_value1", "example_value2"}, outputs["json_string_array"].Value)
	assert.InEpsilon(t, 1234.56789, outputs["json_number"].Value, 0.0001)
	assert.Equal(t, "example_value", outputs["json_string"].Value)
	assert.Equal(t, "Welcome to SOPS! Edit this file as you please!", outputs["json_hello"].Value)
	assert.Equal(t, []any{true, false}, outputs["yaml_bool_array"].Value)
	assert.Equal(t, []any{"example_value1", "example_value2"}, outputs["yaml_string_array"].Value)
	assert.InEpsilon(t, 1234.5679, outputs["yaml_number"].Value, 0.0001)
	assert.Equal(t, "example_value", outputs["yaml_string"].Value)
	assert.Equal(t, "Welcome to SOPS! Edit this file as you please!", outputs["yaml_hello"].Value)
	assert.Equal(t, "Raw Secret Example", outputs["text_value"].Value)
	assert.Contains(t, outputs["env_value"].Value, "DB_PASSWORD=tomato")
	assert.Contains(t, outputs["ini_value"].Value, "password = potato")
}

func TestSOPSDecryptedCorrectlyRunAll(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureSops)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSops)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureSops)
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --all --non-interactive --working-dir %s/../.. --queue-include-dir %s  -- apply -auto-approve",
			rootPath,
			testFixtureSops,
		),
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all --non-interactive --working-dir %s/../.. --queue-include-dir %s  -- output -no-color -json",
			rootPath,
			testFixtureSops,
		),
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, []any{true, false}, outputs["json_bool_array"].Value)
	assert.Equal(t, []any{"example_value1", "example_value2"}, outputs["json_string_array"].Value)
	assert.InEpsilon(t, 1234.56789, outputs["json_number"].Value, 0.0001)
	assert.Equal(t, "example_value", outputs["json_string"].Value)
	assert.Equal(t, "Welcome to SOPS! Edit this file as you please!", outputs["json_hello"].Value)
	assert.Equal(t, []any{true, false}, outputs["yaml_bool_array"].Value)
	assert.Equal(t, []any{"example_value1", "example_value2"}, outputs["yaml_string_array"].Value)
	assert.InEpsilon(t, 1234.5679, outputs["yaml_number"].Value, 0.0001)
	assert.Equal(t, "example_value", outputs["yaml_string"].Value)
	assert.Equal(t, "Welcome to SOPS! Edit this file as you please!", outputs["yaml_hello"].Value)
	assert.Equal(t, "Raw Secret Example", outputs["text_value"].Value)
	assert.Contains(t, outputs["env_value"].Value, "DB_PASSWORD=tomato")
	assert.Contains(t, outputs["ini_value"].Value, "password = potato")
}

func TestSOPSDecryptedCorrectlyRunAllWithFilter(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureSops)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSops)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureSops)
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --all --non-interactive --working-dir %s/../.. --experiment filter-flag --filter ./%s -- apply -auto-approve",
			rootPath,
			testFixtureSops,
		),
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all --non-interactive --working-dir %s/../.. --experiment filter-flag --filter ./%s -- output -no-color -json",
			rootPath,
			testFixtureSops,
		),
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, []any{true, false}, outputs["json_bool_array"].Value)
	assert.Equal(t, []any{"example_value1", "example_value2"}, outputs["json_string_array"].Value)
	assert.InEpsilon(t, 1234.56789, outputs["json_number"].Value, 0.0001)
	assert.Equal(t, "example_value", outputs["json_string"].Value)
	assert.Equal(t, "Welcome to SOPS! Edit this file as you please!", outputs["json_hello"].Value)
	assert.Equal(t, []any{true, false}, outputs["yaml_bool_array"].Value)
	assert.Equal(t, []any{"example_value1", "example_value2"}, outputs["yaml_string_array"].Value)
	assert.InEpsilon(t, 1234.5679, outputs["yaml_number"].Value, 0.0001)
	assert.Equal(t, "example_value", outputs["yaml_string"].Value)
	assert.Equal(t, "Welcome to SOPS! Edit this file as you please!", outputs["yaml_hello"].Value)
	assert.Equal(t, "Raw Secret Example", outputs["text_value"].Value)
	assert.Contains(t, outputs["env_value"].Value, "DB_PASSWORD=tomato")
	assert.Contains(t, outputs["ini_value"].Value, "password = potato")
}

func TestSOPSTerragruntLogSopsErrors(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSopsErrors)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureSopsErrors)

	// apply and check for errors
	_, errorOut, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply --non-interactive --log-level trace --working-dir "+testPath)
	require.Error(t, err)

	assert.Contains(t, errorOut, "error decrypting key: [error decrypting key")
	assert.Contains(t, errorOut, "error base64-decoding encrypted data key: illegal base64 data at input byte")
}

func TestSOPSDecryptOnMissing(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureSopsMissing)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSopsMissing)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureSopsMissing)

	// apply and check for errors
	_, errorOut, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply --non-interactive --log-level trace --working-dir "+rootPath)
	require.Error(t, err)

	errorOut = strings.ReplaceAll(errorOut, "\n", " ")

	assert.Contains(t, errorOut, "Encountered error while evaluating locals in file ./terragrunt.hcl")
	assert.Regexp(t, `\./missing\.yaml:.+no such file`, errorOut)
}
