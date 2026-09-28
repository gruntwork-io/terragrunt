//go:build tf

package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/require"
)

func TestTFErrorsHandling(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testSimpleErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleErrors)
	rootPath := filepath.Join(tmpEnvPath, testSimpleErrors)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	require.NoError(t, err)
}

func TestTFIgnoreError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testIgnoreErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testIgnoreErrors)
	rootPath := filepath.Join(tmpEnvPath, testIgnoreErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Ignoring error example1")
	assert.NotContains(t, stderr, "Ignoring error example2")
}

func TestTFRunAllIgnoreError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testRunAllIgnoreErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testRunAllIgnoreErrors)
	rootPath := filepath.Join(tmpEnvPath, testRunAllIgnoreErrors)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Ignoring error example1")
	assert.NotContains(t, stderr, "Ignoring error example2")
	assert.Contains(t, stdout, "value-from-app-2")
}

func TestTFRetryError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testRetryErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testRetryErrors)
	rootPath := filepath.Join(tmpEnvPath, testRetryErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Encountered retryable error: script_errors")
	assert.NotContains(t, stderr, "aws_errors")
}

func TestTFRetryFailError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testRetryFailErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testRetryFailErrors)
	rootPath := filepath.Join(tmpEnvPath, testRetryFailErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	require.Error(t, err)
	assert.Contains(t, stderr, "Encountered retryable error: script_errors")
}

func TestTFIgnoreSignal(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testIgnoreSignalErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testIgnoreSignalErrors)
	rootPath := filepath.Join(tmpEnvPath, testIgnoreSignalErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Ignoring error example1")
	assert.NotContains(t, stderr, "Ignoring error example2")

	// Signals file is written to original config directory (opts.WorkingDir during error handling)
	signalsFile := filepath.Join(rootPath, "error-signals.json")
	assert.FileExists(t, signalsFile)

	content, err := os.ReadFile(signalsFile)
	require.NoError(t, err, "Failed to read error-signals.json")

	var signals struct {
		Message string `json:"message"`
	}

	err = json.Unmarshal(content, &signals)
	require.NoError(t, err, "Failed to parse error-signals.json")
	assert.Equal(t, "Failed example1", signals.Message, "Unexpected error message")
}

func TestTFRunAllError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testRunAllErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testRunAllErrors)
	rootPath := filepath.Join(tmpEnvPath, testRunAllErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Ignoring error example1")
	assert.NotContains(t, stderr, "Ignoring error example2")
	assert.Contains(t, stderr, "Encountered retryable error: script_errors")
}

func TestTFIgnoreNegativePattern(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testNegativePatternErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testNegativePatternErrors)
	rootPath := filepath.Join(tmpEnvPath, testNegativePatternErrors)

	_, stdout, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	require.Error(t, err)
	assert.Contains(t, stdout, "Error: baz")
}

func TestTFHandleMultiLineErrors(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testMultiLineErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testMultiLineErrors)
	rootPath := filepath.Join(tmpEnvPath, testMultiLineErrors)

	_, stdout, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	require.NoError(t, err)
	assert.Contains(t, stdout, "Ignoring transit gateway not found when creating internal route")
}

func TestTFGetDefaultRetryableErrors(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testGetDefaultErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testGetDefaultErrors)
	rootPath := filepath.Join(tmpEnvPath, testGetDefaultErrors)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	// Verify get_default_retryable_errors() returns a non-empty list
	defaultErrors := outputs["default_retryable_errors"]
	assert.NotEmpty(t, defaultErrors.Value)

	// Verify custom error is passed through
	customError := outputs["custom_error"]
	assert.Equal(t, "my special snowflake", customError.Value)
}

func TestTFNoAutoRetryFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testNoAutoRetry)
	tmpEnvPath := helpers.CopyEnvironment(t, testNoAutoRetry)
	rootPath := filepath.Join(tmpEnvPath, testNoAutoRetry)

	// Test with --no-auto-retry flag - should fail without retry
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --no-auto-retry --non-interactive --working-dir "+rootPath,
	)
	require.Error(t, err)
	assert.Contains(t, stderr, "Transient error")

	// Cleanup for second test - success.txt is created in cache directory (default hook behavior)
	cacheDir := helpers.FindCacheWorkingDir(t, rootPath)
	successFile := filepath.Join(cacheDir, "success.txt")
	err = os.Remove(successFile)
	require.NoError(t, err)
	helpers.CleanupTerraformFolder(t, testNoAutoRetry)

	// Test without flag - should succeed with retry
	_, stderr2, err2 := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err2)
	assert.Contains(t, stderr2, "Encountered retryable error")
}
