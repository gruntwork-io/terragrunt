package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

const (
	testSimpleErrors          = "fixtures/errors/default"
	testIgnoreErrors          = "fixtures/errors/ignore"
	testIgnoreSignalErrors    = "fixtures/errors/ignore-signal"
	testRunAllIgnoreErrors    = "fixtures/errors/run-all-ignore"
	testRetryErrors           = "fixtures/errors/retry"
	testRetryFailErrors       = "fixtures/errors/retry-fail"
	testRunAllErrors          = "fixtures/errors/run-all"
	testNegativePatternErrors = "fixtures/errors/ignore-negative-pattern"
	testMultiLineErrors       = "fixtures/errors/multi-line"
	testGetDefaultErrors      = "fixtures/errors/get-default-errors"
	testNoAutoRetry           = "fixtures/errors/no-auto-retry"
)

func TestErrorsHandling(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testSimpleErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleErrors)
	rootPath := util.JoinPath(tmpEnvPath, testSimpleErrors)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)
}

func TestIgnoreError(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testIgnoreErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testIgnoreErrors)
	rootPath := util.JoinPath(tmpEnvPath, testIgnoreErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Ignoring error example1")
	assert.NotContains(t, stderr, "Ignoring error example2")
}

func TestRunAllIgnoreError(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testRunAllIgnoreErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testRunAllIgnoreErrors)
	rootPath := util.JoinPath(tmpEnvPath, testRunAllIgnoreErrors)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	require.NoError(t, err)
	assert.Contains(t, stderr, "Ignoring error example1")
	assert.NotContains(t, stderr, "Ignoring error example2")
	assert.Contains(t, stdout, "value-from-app-2")
}

func TestRetryError(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testRetryErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testRetryErrors)
	rootPath := util.JoinPath(tmpEnvPath, testRetryErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Encountered retryable error: script_errors")
	assert.NotContains(t, stderr, "aws_errors")
}

func TestRetryFailError(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testRetryFailErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testRetryFailErrors)
	rootPath := util.JoinPath(tmpEnvPath, testRetryFailErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	require.Error(t, err)
	assert.Contains(t, stderr, "Encountered retryable error: script_errors")
}

func TestIgnoreSignal(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testIgnoreSignalErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testIgnoreSignalErrors)
	rootPath := util.JoinPath(tmpEnvPath, testIgnoreSignalErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Ignoring error example1")
	assert.NotContains(t, stderr, "Ignoring error example2")

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

func TestRunAllError(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testRunAllErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testRunAllErrors)
	rootPath := util.JoinPath(tmpEnvPath, testRunAllErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	require.NoError(t, err)
	assert.Contains(t, stderr, "Ignoring error example1")
	assert.NotContains(t, stderr, "Ignoring error example2")
	assert.Contains(t, stderr, "Encountered retryable error: script_errors")
}

func TestRunAllFail(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testRunAllErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testRunAllErrors)
	rootPath := util.JoinPath(tmpEnvPath, testRunAllErrors)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --feature unstable=false --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")
	require.NoError(t, err)
}

func TestIgnoreNegativePattern(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testNegativePatternErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testNegativePatternErrors)
	rootPath := util.JoinPath(tmpEnvPath, testNegativePatternErrors)

	_, stdout, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	require.Error(t, err)
	assert.Contains(t, stdout, "Error: baz")
}

func TestHandleMultiLineErrors(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testMultiLineErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testMultiLineErrors)
	rootPath := util.JoinPath(tmpEnvPath, testMultiLineErrors)

	_, stdout, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)
	assert.Contains(t, stdout, "Ignoring transit gateway not found when creating internal route")
}

func TestGetDefaultRetryableErrors(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testGetDefaultErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testGetDefaultErrors)
	rootPath := util.JoinPath(tmpEnvPath, testGetDefaultErrors)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath)
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

func TestNoAutoRetryFlag(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testNoAutoRetry)
	tmpEnvPath := helpers.CopyEnvironment(t, testNoAutoRetry)
	rootPath := util.JoinPath(tmpEnvPath, testNoAutoRetry)

	// Test with --no-auto-retry flag - should fail without retry
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --no-auto-retry --non-interactive --working-dir "+rootPath)
	require.Error(t, err)
	assert.Contains(t, stderr, "Transient error")

	// Cleanup for second test
	successFile := filepath.Join(rootPath, "success.txt")
	err = os.Remove(successFile)
	require.NoError(t, err)
	cleanupTerraformFolder(t, testNoAutoRetry)

	// Test without flag - should succeed with retry
	_, stderr2, err2 := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)
	require.NoError(t, err2)
	assert.Contains(t, stderr2, "Encountered retryable error")
}
