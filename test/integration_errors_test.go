package test_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
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
	testDependencyOutputRetry = "fixtures/errors/dependency-output-retry"
)

func TestRunAllFail(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testRunAllErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testRunAllErrors)
	rootPath := filepath.Join(tmpEnvPath, testRunAllErrors)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --feature unstable=false --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)
	require.Error(t, err)
}

func TestDependencyOutputRetryHonorsAutoRetry(t *testing.T) {
	t.Parallel()

	t.Run("retries_dependency_output_errors", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testDependencyOutputRetry)
		tmpEnvPath := helpers.CopyEnvironment(t, testDependencyOutputRetry)
		rootPath := filepath.Join(tmpEnvPath, testDependencyOutputRetry)
		appPath := filepath.Join(rootPath, "app")
		tfPath := filepath.Join(rootPath, "fake-terraform.sh")
		require.NoError(t, os.Chmod(tfPath, 0o755))

		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --tf-path "+tfPath+" --non-interactive --working-dir "+appPath)
		require.NoError(t, err)
		assert.Contains(t, stderr, "Encountered retryable error: transient_dependency_output")
	})

	t.Run("no_auto_retry_disables_dependency_output_retries", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testDependencyOutputRetry)
		tmpEnvPath := helpers.CopyEnvironment(t, testDependencyOutputRetry)
		rootPath := filepath.Join(tmpEnvPath, testDependencyOutputRetry)
		appPath := filepath.Join(rootPath, "app")
		tfPath := filepath.Join(rootPath, "fake-terraform.sh")
		require.NoError(t, os.Chmod(tfPath, 0o755))

		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --tf-path "+tfPath+" --no-auto-retry --non-interactive --working-dir "+appPath)
		require.Error(t, err)
		assert.Contains(t, stderr, "Transient dependency output error")
		assert.NotContains(t, stderr, "Encountered retryable error: transient_dependency_output")
	})
}
