package test_test

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureAutoRetryRerun                     = "fixtures/auto-retry/re-run"
	testFixtureAutoRetryExhaust                   = "fixtures/auto-retry/exhaust"
	testFixtureAutoRetryGetDefaultErrors          = "fixtures/auto-retry/get-default-errors"
	testFixtureAutoRetryCustomErrors              = "fixtures/auto-retry/custom-errors"
	testFixtureAutoRetryCustomErrorsNotSet        = "fixtures/auto-retry/custom-errors-not-set"
	testFixtureAutoRetryApplyAllRetries           = "fixtures/auto-retry/apply-all"
	testFixtureAutoRetryConfigurableRetries       = "fixtures/auto-retry/configurable-retries"
	testFixtureAutoRetryConfigurableRetriesError1 = "fixtures/auto-retry/configurable-retries-incorrect-retry-attempts"
	testFixtureAutoRetryConfigurableRetriesError2 = "fixtures/auto-retry/configurable-retries-incorrect-sleep-interval"
)

func TestAutoRetryBasicRerun(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := helpers.CopyEnvironment(t, testFixtureAutoRetryRerun)
	modulePath := util.JoinPath(rootPath, testFixtureAutoRetryRerun)
	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+modulePath, out, os.Stderr)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "Apply complete!")
}

func TestAutoRetrySkip(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := helpers.CopyEnvironment(t, testFixtureAutoRetryRerun)
	modulePath := util.JoinPath(rootPath, testFixtureAutoRetryRerun)
	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-no-auto-retry --terragrunt-non-interactive --terragrunt-working-dir "+modulePath, out, os.Stderr)

	require.Error(t, err)
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryExhaustRetries(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := helpers.CopyEnvironment(t, testFixtureAutoRetryExhaust)
	modulePath := util.JoinPath(rootPath, testFixtureAutoRetryExhaust)
	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+modulePath, out, os.Stderr)

	require.Error(t, err)
	assert.Contains(t, out.String(), "Failed to load backend")
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryCustomRetryableErrors(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := helpers.CopyEnvironment(t, testFixtureAutoRetryCustomErrors)
	modulePath := util.JoinPath(rootPath, testFixtureAutoRetryCustomErrors)
	err := helpers.RunTerragruntCommand(t, "terragrunt apply --auto-approve --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+modulePath, out, os.Stderr)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "My own little error")
	assert.Contains(t, out.String(), "Apply complete!")
}

func TestAutoRetryGetDefaultErrors(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixtureAutoRetryGetDefaultErrors)
	modulePath := util.JoinPath(rootPath, testFixtureAutoRetryGetDefaultErrors)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+modulePath)

	stdout := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+modulePath, &stdout, os.Stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	list, hasVal := outputs["retryable_errors"]
	assert.True(t, hasVal)
	assert.ElementsMatch(t, list.Value, append(options.DefaultRetryableErrors, "my special snowflake"))
}

func TestAutoRetryCustomRetryableErrorsFailsWhenRetryableErrorsNotSet(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := helpers.CopyEnvironment(t, testFixtureAutoRetryCustomErrorsNotSet)
	modulePath := util.JoinPath(rootPath, testFixtureAutoRetryCustomErrorsNotSet)
	err := helpers.RunTerragruntCommand(t, "terragrunt apply --auto-approve --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+modulePath, out, os.Stderr)

	require.Error(t, err)
	assert.Contains(t, out.String(), "My own little error")
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryFlagWithRecoverableError(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := helpers.CopyEnvironment(t, testFixtureAutoRetryRerun)
	modulePath := util.JoinPath(rootPath, testFixtureAutoRetryRerun)
	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-no-auto-retry --terragrunt-non-interactive --terragrunt-working-dir "+modulePath, out, os.Stderr)

	require.Error(t, err)
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryEnvVarWithRecoverableError(t *testing.T) {
	t.Setenv("TERRAGRUNT_NO_AUTO_RETRY", "true")
	out := new(bytes.Buffer)
	rootPath := helpers.CopyEnvironment(t, testFixtureAutoRetryRerun)
	modulePath := util.JoinPath(rootPath, testFixtureAutoRetryRerun)
	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+modulePath, out, os.Stderr)

	require.Error(t, err)
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryApplyAllDependentModuleRetries(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := helpers.CopyEnvironment(t, testFixtureAutoRetryApplyAllRetries)
	modulePath := util.JoinPath(rootPath, testFixtureAutoRetryApplyAllRetries)
	err := helpers.RunTerragruntCommand(t, "terragrunt apply-all -auto-approve --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+modulePath, out, os.Stderr)

	require.NoError(t, err)
	s := out.String()
	assert.Contains(t, s, "app1 output")
	assert.Contains(t, s, "app2 output")
	assert.Contains(t, s, "app3 output")
	assert.Contains(t, s, "Apply complete!")
}

func TestAutoRetryConfigurableRetries(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rootPath := helpers.CopyEnvironment(t, testFixtureAutoRetryConfigurableRetries)
	modulePath := util.JoinPath(rootPath, testFixtureAutoRetryConfigurableRetries)
	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+modulePath, stdout, stderr)
	sleeps := regexp.MustCompile("Sleeping 0s before retrying.").FindAllStringIndex(stderr.String(), -1)

	require.NoError(t, err)
	assert.Len(t, sleeps, 4) // 5 retries, so 4 sleeps
	assert.Contains(t, stdout.String(), "Apply complete!")
}

func TestAutoRetryConfigurableRetriesErrors(t *testing.T) {
	t.Parallel()

	tc := []struct {
		fixture      string
		errorMessage string
	}{
		{testFixtureAutoRetryConfigurableRetriesError1, "cannot have less than 1 max retry"},
		{testFixtureAutoRetryConfigurableRetriesError2, "cannot sleep for less than 0 seconds"},
	}
	for _, tc := range tc {
		tc := tc
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			rootPath := helpers.CopyEnvironment(t, tc.fixture)
			modulePath := util.JoinPath(rootPath, tc.fixture)

			err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+modulePath, stdout, stderr)
			require.Error(t, err)
			assert.NotContains(t, stdout.String(), "Apply complete!")
			assert.Contains(t, err.Error(), tc.errorMessage)
		})
	}
}
