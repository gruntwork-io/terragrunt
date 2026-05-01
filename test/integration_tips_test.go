package test_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureTips              = "fixtures/tips"
	testFixtureStackTargetTipDir = "fixtures/stacks/basic"
)

// TestTipDebuggingDocsShownOnError verifies that the debugging-docs tip
// is displayed when an error occurs during `run`.
func TestTipDebuggingDocsShownOnError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureTips)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTips)
	rootPath := filepath.Join(tmpEnvPath, testFixtureTips)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"run apply --non-interactive --working-dir "+rootPath,
	)

	require.Error(t, err)
	assert.Contains(t, stderr, "TIP (debugging-docs): For help troubleshooting errors")
	assert.Contains(t, stderr, "docs.terragrunt.com/troubleshooting/debugging")
}

// TestTipDebuggingDocsNotShownWithNoTips verifies that the debugging-docs tip
// is NOT displayed when --no-tips flag is used.
func TestTipDebuggingDocsNotShownWithNoTips(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureTips)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTips)
	rootPath := filepath.Join(tmpEnvPath, testFixtureTips)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"run apply --no-tips --non-interactive --working-dir "+rootPath,
	)

	require.Error(t, err)
	assert.NotContains(t, stderr, "TIP (debugging-docs): For help troubleshooting errors")
}

// TestTipDebuggingDocsNotShownWithNoTipSpecific verifies that the debugging-docs tip
// is NOT displayed when --no-tip debugging-docs flag is used.
func TestTipDebuggingDocsNotShownWithNoTipSpecific(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureTips)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTips)
	rootPath := filepath.Join(tmpEnvPath, testFixtureTips)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"run apply --no-tip debugging-docs --non-interactive --working-dir "+rootPath,
	)

	require.Error(t, err)
	assert.NotContains(t, stderr, "TIP (debugging-docs): For help troubleshooting errors")
}

// TestTipStackTargetShownOnStackGenerate verifies that the stack-target-missing-type-stack
// tip is displayed when `stack generate --filter` points at a stack directory without
// `| type=stack`.
func TestTipStackTargetShownOnStackGenerate(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackTargetTipDir)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackTargetTipDir)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackTargetTipDir, "live")

	_, stderr, _ := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --non-interactive --working-dir "+rootPath+" --filter ./",
	)

	assert.Contains(t, stderr, "TIP (stack-target-missing-type-stack)")
	assert.Contains(t, stderr, "type=stack")
}

// TestTipStackTargetSuppressedWithNoTip verifies that --no-tip suppresses the
// stack-target-missing-type-stack tip.
func TestTipStackTargetSuppressedWithNoTip(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackTargetTipDir)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackTargetTipDir)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackTargetTipDir, "live")

	_, stderr, _ := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --non-interactive --no-tip stack-target-missing-type-stack --working-dir "+rootPath+" --filter .",
	)

	assert.NotContains(t, stderr, "TIP (stack-target-missing-type-stack)")
}

// TestTipStackTargetNotShownWithTypeStackSuffix verifies that the tip is NOT
// displayed when the user already supplies `| type=stack`.
func TestTipStackTargetNotShownWithTypeStackSuffix(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackTargetTipDir)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackTargetTipDir)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackTargetTipDir, "live")

	_, stderr, _ := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --non-interactive --working-dir "+rootPath+" --filter '. | type=stack'",
	)

	assert.NotContains(t, stderr, "TIP (stack-target-missing-type-stack)")
}
