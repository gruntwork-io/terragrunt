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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureSops        = "fixtures/sops"
	testFixtureSopsErrors  = "fixtures/sops-errors"
	testFixtureSopsMissing = "fixtures/sops-missing"

	// PGP fingerprint of the "Terragrunt Tests" key used for SOPS encryption.
	sopsPGPFingerprint = "3EF98802EEDCAF0C688B81F419546E0C123C664E"
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
  secret_value = lookup(local.secret, "value", "DECRYPTION_FAILED")
  unit_name    = "%s"
}
`

// generateSopsMultiUnitFixtures creates numUnits directories, each with a
// main.tf, terragrunt.hcl, and a SOPS-encrypted secret.enc.json file.
// Requires the sops CLI and the test PGP key imported in GPG.
func generateSopsMultiUnitFixtures(t *testing.T, numUnits int) string {
	t.Helper()

	sopsPath, err := exec.LookPath("sops")
	require.NoError(t, err, "sops CLI required for this test")

	dir := t.TempDir()

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

		// Write plaintext, encrypt with sops, remove plaintext
		plainFile := filepath.Join(unitDir, "secret.plain.json")
		require.NoError(t, os.WriteFile(plainFile,
			[]byte(fmt.Sprintf(`{"value":"secret-from-%s"}`, unitName)), 0644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, sopsPath, "--encrypt",
			"--pgp", sopsPGPFingerprint,
			"--input-type", "json", "--output-type", "json",
			plainFile)

		out, err := cmd.Output()
		require.NoError(t, err, "sops encrypt failed for %s: %s", unitName, err)

		require.NoError(t, os.WriteFile(
			filepath.Join(unitDir, "secret.enc.json"), out, 0644))
		require.NoError(t, os.Remove(plainFile))
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
// when multiple units with different encrypted secrets are processed in parallel via run --all.
// This is a regression test for https://github.com/gruntwork-io/terragrunt/issues/5515
//
// The test uses 48 units to maximize parallelism and increase the chance of triggering
// any race conditions in the SOPS decryption code.
func TestSOPSDecryptedCorrectlyRunAllMultipleUnits(t *testing.T) {
	t.Parallel()

	// Generate list of 48 units - more units = more parallelism = higher chance of race
	const numUnits = 48

	units := make([]struct {
		name          string
		expectedValue string
	}, numUnits)

	for i := 0; i < numUnits; i++ {
		unitNum := fmt.Sprintf("%02d", i+1)
		units[i].name = "unit-" + unitNum
		units[i].expectedValue = "secret-from-unit-" + unitNum
	}

	rootPath := generateSopsMultiUnitFixtures(t, numUnits)

	// Run apply on all 48 units in parallel - this is where the race condition manifests
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --all --non-interactive --working-dir %s -- apply -auto-approve",
			rootPath,
		),
	)

	// Verify each unit got its own correct decrypted secret value
	for _, unit := range units {
		unitPath := filepath.Join(rootPath, unit.name)
		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}

		err := helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+unitPath,
			&stdout,
			&stderr,
		)
		require.NoError(t, err, "Failed to get output for %s: %s", unit.name, stderr.String())

		outputs := map[string]helpers.TerraformOutput{}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs), "Failed to parse output for %s", unit.name)

		// Check for the specific failure mode from issue #5515:
		// If SOPS decryption fails due to race, try() returns {} and lookup returns "DECRYPTION_FAILED"
		secretValue, ok := outputs["secret_value"].Value.(string)
		require.True(t, ok, "secret_value should be a string for %s", unit.name)

		if secretValue == "DECRYPTION_FAILED" {
			t.Fatalf("SOPS race condition detected! Unit %s got DECRYPTION_FAILED instead of %s. "+
				"This indicates the sops_decrypt_file failed and try() returned empty {}.",
				unit.name, unit.expectedValue)
		}

		assert.Equal(t, unit.expectedValue, secretValue,
			"Unit %s should have correct decrypted secret value", unit.name)
		assert.Equal(t, unit.name, outputs["unit_name"].Value,
			"Unit %s should have correct unit name", unit.name)
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
