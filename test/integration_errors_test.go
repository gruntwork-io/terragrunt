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
)

func TestErrorsHandling(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testSimpleErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleErrors)
	rootPath := util.JoinPath(tmpEnvPath, testSimpleErrors)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	require.NoError(t, err)
}

func TestIgnoreError(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testIgnoreErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testIgnoreErrors)
	rootPath := util.JoinPath(tmpEnvPath, testIgnoreErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Ignoring error example1")
	assert.NotContains(t, stderr, "Ignoring error example2")
}

func TestRunAllIgnoreError(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testRunAllIgnoreErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testRunAllIgnoreErrors)
	rootPath := util.JoinPath(tmpEnvPath, testRunAllIgnoreErrors)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

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

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Encountered retryable error: script_errors")
	assert.NotContains(t, stderr, "aws_errors")
}

func TestRetryFailError(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testRetryFailErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testRetryFailErrors)
	rootPath := util.JoinPath(tmpEnvPath, testRetryFailErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	require.Error(t, err)
	assert.Contains(t, stderr, "Encountered retryable error: script_errors")
}

func TestIgnoreSignal(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testIgnoreSignalErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testIgnoreSignalErrors)
	rootPath := util.JoinPath(tmpEnvPath, testIgnoreSignalErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

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

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

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

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --feature unstable=false --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	require.Error(t, err)
}

func TestIgnoreNegativePattern(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testNegativePatternErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testNegativePatternErrors)
	rootPath := util.JoinPath(tmpEnvPath, testNegativePatternErrors)

	_, stdout, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	require.Error(t, err)
	assert.Contains(t, stdout, "Error: baz")
}
