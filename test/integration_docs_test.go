package test_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureQuickStart = "fixtures/docs/01-quick-start"
)

func TestDocsQuickStart(t *testing.T) {
	t.Parallel()

	t.Run("step-01", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-01", "foo")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "Plan: 1 to add, 0 to change, 0 to destroy.")

		stdout, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")

	})

	t.Run("step-01.1", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-01.1", "foo")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan -var content='Hello, Terragrunt!' --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)

		_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve -var content='Hello, Terragrunt!' --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-02", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-02")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all plan -var content='Hello, Terragrunt!' --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)

		_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve -var content='Hello, Terragrunt!' --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-03", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-03")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all plan -var content='Hello, Terragrunt!' --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)

		_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve -var content='Hello, Terragrunt!' --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-04", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-04")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)

		_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-05", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-05")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)

		_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-06", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-06")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.Error(t, err)
	})

	t.Run("step-07", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-07")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-07.1", func(t *testing.T) {
		t.Parallel()

		stepPath := util.JoinPath(testFixtureQuickStart, "step-07.1")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
	})
}
