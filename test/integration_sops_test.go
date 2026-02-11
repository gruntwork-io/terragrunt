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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureSops        = "fixtures/sops"
	testFixtureSopsErrors  = "fixtures/sops-errors"
	testFixtureSopsMissing = "fixtures/sops-missing"
)

const sopsMultiUnitMainTf = `variable "secret_value" {
  type = string
}

variable "unit_name" {
  type = string
}

output "secret_value" {
  value = var.secret_value
}

output "unit_name" {
  value = var.unit_name
}
`

const sopsMultiUnitTerragruntHcl = `locals {
  secret = try(jsondecode(sops_decrypt_file("${get_terragrunt_dir()}/secret.enc.json")), {})
}

inputs = {
  secret_value = lookup(local.secret, "example_key", "DECRYPTION_FAILED")
  unit_name    = "%s"
}
`

// generateSopsMultiUnitFixtures creates numUnits directories, each with a
// main.tf, terragrunt.hcl, and a copy of the existing SOPS-encrypted secrets.json.
// Only requires the test PGP key imported in GPG — no sops CLI needed.
func generateSopsMultiUnitFixtures(t *testing.T, numUnits int) string {
	t.Helper()

	dir := t.TempDir()

	// Reuse existing SOPS-encrypted file from the sops fixture as template
	encData, err := os.ReadFile("fixtures/sops/secrets.json")
	require.NoError(t, err, "failed to read SOPS template file")

	for i := 1; i <= numUnits; i++ {
		unitName := fmt.Sprintf("unit-%02d", i)
		unitDir := filepath.Join(dir, unitName)
		require.NoError(t, os.MkdirAll(unitDir, 0755))

		require.NoError(t, os.WriteFile(
			filepath.Join(unitDir, "main.tf"),
			[]byte(sopsMultiUnitMainTf), 0644))

		require.NoError(t, os.WriteFile(
			filepath.Join(unitDir, "terragrunt.hcl"),
			[]byte(fmt.Sprintf(sopsMultiUnitTerragruntHcl, unitName)), 0644))

		require.NoError(t, os.WriteFile(
			filepath.Join(unitDir, "secret.enc.json"), encData, 0644))
	}

	return dir
}

func TestSOPSDecryptedCorrectly(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureSops)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSops)
	rootPath := filepath.Join(tmpEnvPath, testFixtureSops)

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
	rootPath := filepath.Join(tmpEnvPath, testFixtureSops)
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

	helpers.CleanupTerraformFolder(t, testFixtureSops)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSops)
	rootPath := filepath.Join(tmpEnvPath, testFixtureSops)
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

// TestSOPSDecryptedCorrectlyRunAllMultipleUnits tests that SOPS decryption works correctly
// when multiple units with the same encrypted secret are processed in parallel via run --all.
func TestSOPSDecryptedCorrectlyRunAllMultipleUnits(t *testing.T) {
	t.Parallel()

	const numUnits = 12

	rootPath := generateSopsMultiUnitFixtures(t, numUnits)

	// Run apply on all units in parallel — this is where the race condition manifests
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --all --non-interactive --working-dir %s -- apply -auto-approve",
			rootPath,
		),
	)

	// Verify each unit successfully decrypted the secret
	for i := 1; i <= numUnits; i++ {
		unitName := fmt.Sprintf("unit-%02d", i)
		unitPath := filepath.Join(rootPath, unitName)
		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}

		err := helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+unitPath,
			&stdout,
			&stderr,
		)
		require.NoError(t, err, "Failed to get output for %s: %s", unitName, stderr.String())

		outputs := map[string]helpers.TerraformOutput{}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs), "Failed to parse output for %s", unitName)

		// Check for the specific failure mode from issue #5515:
		// If SOPS decryption fails due to race, try() returns {} and lookup returns "DECRYPTION_FAILED"
		secretValue, ok := outputs["secret_value"].Value.(string)
		require.True(t, ok, "secret_value should be a string for %s", unitName)

		if secretValue == "DECRYPTION_FAILED" {
			t.Fatalf("SOPS race condition detected! Unit %s got DECRYPTION_FAILED. "+
				"This indicates sops_decrypt_file failed and try() returned empty {}.",
				unitName)
		}

		assert.Equal(t, "example_value", secretValue,
			"Unit %s should have correct decrypted secret value", unitName)
		assert.Equal(t, unitName, outputs["unit_name"].Value,
			"Unit %s should have correct unit name", unitName)
	}
}

func TestSOPSTerragruntLogSopsErrors(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSopsErrors)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureSopsErrors)

	// apply and check for errors
	_, errorOut, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply --non-interactive --working-dir "+testPath)
	require.Error(t, err)

	assert.Contains(t, errorOut, "error decrypting key: [error decrypting key")
	assert.Contains(t, errorOut, "error base64-decoding encrypted data key: illegal base64 data at input byte")
}

func TestSOPSDecryptOnMissing(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureSopsMissing)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSopsMissing)
	rootPath := filepath.Join(tmpEnvPath, testFixtureSopsMissing)

	// apply and check for errors
	_, errorOut, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply --log-level debug --non-interactive --working-dir "+rootPath,
	)
	require.Error(t, err)

	errorOut = strings.ReplaceAll(errorOut, "\n", " ")

	assert.Contains(t, errorOut, "Encountered error while evaluating locals in file ./terragrunt.hcl")
	// Check for the missing file error components separately since they may be split across log lines
	assert.Contains(t, errorOut, "missing.yaml", "Error should reference the missing SOPS file")
	assert.Contains(t, errorOut, "no such file", "Error should indicate file does not exist")
}
