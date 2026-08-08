package test_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
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
