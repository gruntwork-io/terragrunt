package test_test

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"testing"
)

const (
	testExcludeByDefault       = "fixtures/exclude/default"
	testExcludeDisabled        = "fixtures/exclude/disabled"
	testExcludeByAction        = "fixtures/exclude/action"
	testExcludeByFlags         = "fixtures/exclude/feature-flags"
	testExcludeDependencies    = "fixtures/exclude/dependencies"
	testExcludeAllExceptOutput = "fixtures/exclude/all-except-output"
	testExcludeNoRun           = "fixtures/exclude/no-run"
)

func TestExcludeByDefault(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeByDefault)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeByDefault)
	rootPath := filepath.Join(tmpEnvPath, testExcludeByDefault)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	require.NoError(t, err)

	assert.Contains(t, stderr, "app1")
	assert.NotContains(t, stderr, "app2")
}

func TestExcludeDisabled(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeDisabled)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeDisabled)
	rootPath := filepath.Join(tmpEnvPath, testExcludeDisabled)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	require.NoError(t, err)

	assert.Contains(t, stderr, "app1")
	assert.Contains(t, stderr, "app2")
}

func TestExcludeApply(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeByAction)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeByAction)
	rootPath := filepath.Join(tmpEnvPath, testExcludeByAction)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all plan --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)

	assert.Contains(t, stderr, "exclude-apply")
	assert.NotContains(t, stderr, "exclude-plan")

	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	require.NoError(t, err)

	// should be applied only exclude-plan
	assert.Contains(t, stderr, "exclude-plan")
	assert.NotContains(t, stderr, "exclude-apply")
}

func TestExcludeByFeatureFlagDefault(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeByFlags)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeByFlags)
	rootPath := filepath.Join(tmpEnvPath, testExcludeByFlags)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)

	assert.Contains(t, stderr, "Unit app1")
	assert.NotContains(t, stderr, "Unit app2")
}

func TestExcludeByFeatureFlag(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeByFlags)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeByFlags)
	rootPath := filepath.Join(tmpEnvPath, testExcludeByFlags)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all apply --feature exclude2=false --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)

	assert.Contains(t, stderr, "app1")
	assert.Contains(t, stderr, "app2")
}

func TestExcludeAllByFeatureFlag(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeByFlags)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeByFlags)
	rootPath := filepath.Join(tmpEnvPath, testExcludeByFlags)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all apply --feature exclude1=true --feature exclude2=true --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)

	assert.NotContains(t, stderr, "app1")
	assert.NotContains(t, stderr, "app2")
}

func TestExcludeDependencies(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeDependencies)
	rootPath := filepath.Join(tmpEnvPath, testExcludeDependencies)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --feature exclude=false --feature exclude_dependencies=false --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")
	require.NoError(t, err)

	assert.Contains(t, stderr, "dep")
	assert.Contains(t, stderr, "app1")

	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --feature exclude=true --feature exclude_dependencies=false --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	require.NoError(t, err)

	assert.Contains(t, stderr, "Unit dep")
	assert.NotContains(t, stderr, "Unit app1")
}

func TestExcludeAllExceptOutput(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeAllExceptOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeAllExceptOutput)
	rootPath := filepath.Join(tmpEnvPath, testExcludeAllExceptOutput)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")
	require.NoError(t, err)

	assert.NotContains(t, stderr, "app1")
	assert.Contains(t, stderr, "app2")

	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all output --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "app1")
	assert.Contains(t, stderr, "app2")
}

func TestExcludeNoRunSingleUnit(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeNoRun)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeNoRun)
	rootPath := filepath.Join(tmpEnvPath, testExcludeNoRun, "no-run-unit")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Early exit in terragrunt unit")
	assert.Contains(t, stderr, "due to exclude block with no_run = true")
}

func TestExcludeNoRunRunAll(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeNoRun)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeNoRun)
	rootPath := filepath.Join(tmpEnvPath, testExcludeNoRun)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all plan --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Unit normal-unit")
	assert.NotContains(t, stderr, "Unit no-run-unit")
}

func TestExcludeNoRunConditional(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeNoRun)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeNoRun)
	rootPath := filepath.Join(tmpEnvPath, testExcludeNoRun, "conditional-no-run-unit")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Early exit in terragrunt unit")
	assert.Contains(t, stderr, "due to exclude block with no_run = true")

	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --feature enable_unit=true --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)
	assert.NotContains(t, stderr, "Early exit in terragrunt unit")
}

func TestExcludeNoRunIndependentOfActions(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeNoRun)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeNoRun)
	rootPath := filepath.Join(tmpEnvPath, testExcludeNoRun, "no-run-independent")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	require.NoError(t, err)
	assert.NotContains(t, stderr, "Early exit in terragrunt unit")
	assert.NotContains(t, stderr, "due to exclude block with no_run = true")

	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive --working-dir "+rootPath)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Early exit in terragrunt unit")
	assert.Contains(t, stderr, "due to exclude block with no_run = true")
}
