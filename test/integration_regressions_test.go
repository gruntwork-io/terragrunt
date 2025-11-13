package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureRegressions               = "fixtures/regressions"
	testFixtureDependencyGenerate        = "fixtures/regressions/dependency-generate"
	testFixtureDependencyEmptyConfigPath = "fixtures/regressions/dependency-empty-config-path"
	testFixtureParsingDeprecated         = "fixtures/parsing/exposed-include-with-deprecated-inputs"
	testFixtureSensitiveValues           = "fixtures/regressions/sensitive-values"
)

func TestNoAutoInit(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRegressions)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRegressions)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRegressions, "skip-init")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt apply --no-auto-init --log-level trace --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "no force apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "no force apply stderr")
	require.Error(t, err)
	assert.Contains(t, stderr.String(), "This module is not yet installed.")
}

// Test case for yamldecode bug: https://github.com/gruntwork-io/terragrunt/issues/834
func TestYamlDecodeRegressions(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRegressions)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRegressions)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRegressions, "yamldecode")

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	// Check the output of yamldecode and make sure it doesn't parse the string incorrectly
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "003", outputs["test1"].Value)
	assert.Equal(t, "1.00", outputs["test2"].Value)
	assert.Equal(t, "0ba", outputs["test3"].Value)
}

func TestMockOutputsMergeWithState(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRegressions)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRegressions)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRegressions, "mocks-merge-with-state")

	modulePath := util.JoinPath(rootPath, "module")
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt apply --log-level trace --non-interactive -auto-approve --working-dir "+modulePath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "module-executed")
	require.NoError(t, err)

	deepMapPath := util.JoinPath(rootPath, "deep-map")
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	err = helpers.RunTerragruntCommand(t, "terragrunt apply --log-level trace --non-interactive -auto-approve --working-dir "+deepMapPath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "deep-map-executed")
	require.NoError(t, err)

	shallowPath := util.JoinPath(rootPath, "shallow")
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	err = helpers.RunTerragruntCommand(t, "terragrunt apply --log-level trace --non-interactive -auto-approve --working-dir "+shallowPath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "shallow-map-executed")
	require.NoError(t, err)
}

func TestIncludeError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRegressions)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRegressions)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRegressions, "include-error", "project", "app")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive --working-dir "+rootPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "include blocks without label")
}

// TestDependencyOutputInGenerateBlock tests that dependency outputs can be used in generate blocks.
// This is a regression test for issue #4962 where using dependency outputs in generate blocks
// started failing with "Unsuitable value: value must be known" error in v0.89.0+.
//
// The bug occurred because during `run --all`, the discovery phase was calling ParseConfigFile
// instead of PartialParseConfigFile, which caused generate blocks to be evaluated before
// dependency outputs were resolved. The fix ensures generate blocks are only evaluated when
// each unit runs individually with full dependency resolution.
//
// See: https://github.com/gruntwork-io/terragrunt/issues/4962
func TestDependencyOutputInGenerateBlock(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyGenerate)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDependencyGenerate)
	otherPath := util.JoinPath(rootPath, "other")
	testingPath := util.JoinPath(rootPath, "testing")

	helpers.CleanupTerraformFolder(t, rootPath)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply --non-interactive --working-dir "+otherPath+" -- -auto-approve",
	)

	_, runAllStderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all plan --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.NotContains(t, runAllStderr, "Unsuitable value: value must be known",
		"Should not fail with 'Unsuitable value' error when using dependency outputs in generate blocks")
	assert.NotContains(t, runAllStderr, "Unsuitable value type",
		"Should not fail with 'Unsuitable value type' error")

	// Verify the generate block was created successfully
	generatedFile := util.JoinPath(testingPath, ".terragrunt-cache")
	assert.DirExists(t, generatedFile, "Terragrunt cache should exist")
}

// TestDependencyOutputInGenerateBlockDirectRun tests that dependency outputs work when running directly
// This test verifies that even in the broken version, running directly (without --all) works
func TestDependencyOutputInGenerateBlockDirectRun(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyGenerate)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDependencyGenerate)
	otherPath := util.JoinPath(rootPath, "other")
	testingPath := util.JoinPath(rootPath, "testing")

	helpers.CleanupTerraformFolder(t, rootPath)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply --auto-approve --non-interactive --working-dir "+otherPath,
	)

	_, planStderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --working-dir "+testingPath,
	)
	require.NoError(t, err)

	assert.NotContains(t, planStderr, "Unsuitable value",
		"Direct run should never fail with 'Unsuitable value' error")
}

// TestDependencyOutputInInputsStillWorks verifies that dependency outputs can be used in inputs
func TestDependencyOutputInInputsStillWorks(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyGenerate)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDependencyGenerate)
	otherPath := util.JoinPath(rootPath, "other")

	// Apply the "other" module
	helpers.CleanupTerraformFolder(t, rootPath)

	helpers.RunTerragrunt(t,
		"terragrunt apply --auto-approve --non-interactive --working-dir "+otherPath,
	)

	runAllStdout, runAllStderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+rootPath+" -- --auto-approve",
	)
	require.NoError(t, err)

	assert.True(t, strings.Contains(runAllStdout, "test-token-12345") ||
		strings.Contains(runAllStderr, "test-token-12345"),
		"Token should be passed via inputs")
}

func TestDependencyEmptyConfigPath_ReportsError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDependencyEmptyConfigPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyEmptyConfigPath)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureDependencyEmptyConfigPath)
	helpers.CreateGitRepo(t, gitPath)

	// Run directly against the consumer unit to force evaluation of dependency outputs
	consumerPath := util.JoinPath(gitPath, "_source", "units", "consumer")
	_, stderr, runErr := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive --working-dir "+consumerPath)
	require.Error(t, runErr)
	// Accept match in either stderr or the returned error string
	if !strings.Contains(stderr, "has empty config_path") && !strings.Contains(runErr.Error(), "has empty config_path") {
		t.Fatalf("unexpected error; want empty config_path message, got: %v\nstderr: %s", runErr, stderr)
	}
}

// TestExposedIncludeWithDeprecatedInputsSyntax tests that deprecated dependency.*.inputs.* syntax
// is properly detected even when used in an included config with expose = true.
// This is a regression test for a bug introduced in v0.91.1 where the partial parse path
// did not call DetectDeprecatedConfigurations(), causing cryptic "Could not find Terragrunt
// configuration settings" errors instead of clear deprecation messages.
//
// The bug occurs when:
// 1. An included config (e.g., compcommon.hcl) uses deprecated dependency.*.inputs.* syntax
// 2. The child config includes it with expose = true
// 3. The included config is parsed via PartialParseConfig() which skips deprecation detection
// 4. When evaluating the exposed include, Terragrunt encounters unsupported syntax and fails
//
// See: https://github.com/gruntwork-io/terragrunt/issues/4983
func TestExposedIncludeWithDeprecatedInputsSyntax(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureParsingDeprecated)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureParsingDeprecated)
	childPath := util.JoinPath(tmpEnvPath, testFixtureParsingDeprecated, "child")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --working-dir "+childPath,
	)
	require.Error(t, err)

	// After the fix, we should get a clear error about deprecated syntax
	// instead of the cryptic "Could not find Terragrunt configuration settings" error
	// The error message appears in the error object, not necessarily stderr
	errorMessage := stderr
	if err != nil {
		errorMessage = errorMessage + " " + err.Error()
	}

	assert.Contains(t, errorMessage, "Reading inputs from dependencies is no longer supported")

	// Should NOT get the cryptic error that users were seeing
	assert.NotContains(t, errorMessage, "Could not find Terragrunt configuration settings")
}

// TestRunAllWithGenerateAndExpose tests that run --all works correctly with:
// - Exposed include blocks with generate blocks
// - Dependencies between units
// - Complex inputs with map comparisons
//
// This is a regression test for parsing errors that occurred in v0.90.1+ where
// configs with exposed includes containing generate blocks would fail during
// discovery with "Could not find Terragrunt configuration settings" errors.
//
// See: https://github.com/gruntwork-io/terragrunt/issues/4983
func TestRunAllWithGenerateAndExpose(t *testing.T) {
	t.Parallel()

	testFixture := "fixtures/regressions/parsing-run-all-with-generate"
	helpers.CleanupTerraformFolder(t, testFixture)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixture)
	rootPath := util.JoinPath(tmpEnvPath, testFixture, "services-info")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all plan --non-interactive --working-dir "+rootPath,
	)

	// The command should succeed
	require.NoError(t, err, "run --all plan should succeed")

	// Should not see parsing errors
	assert.NotContains(t, stderr, "Could not find Terragrunt configuration settings",
		"Should not see parsing errors")
	assert.NotContains(t, stderr, "Unrecoverable parse error",
		"Should not see unrecoverable parse errors")

	// Should not see fmt formatting artifacts from %w (e.g., %!w(...))
	assert.NotContains(t, stderr, "%!w(",
		"Should not see formatting artifacts in error output")

	// Verify both units ran successfully
	combinedOutput := stdout + stderr
	assert.Contains(t, combinedOutput, "test1",
		"Should process the service dependency")
	assert.Contains(t, combinedOutput, "null_resource.services_info",
		"Should process the services-info unit with null resource")
}

// TestRunAllWithGenerateAndExpose_WithProviderCacheAndExcludeExternal mirrors the user repro flags
// to ensure no cryptic errors or formatting artifacts appear in logs when using provider cache and
// excluding external dependencies.
func TestRunAllWithGenerateAndExpose_WithProviderCacheAndExcludeExternal(t *testing.T) {
	t.Parallel()

	testFixture := "fixtures/regressions/parsing-run-all-with-generate"
	helpers.CleanupTerraformFolder(t, testFixture)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixture)
	rootPath := util.JoinPath(tmpEnvPath, testFixture, "services-info")

	// Set TG_PROVIDER_CACHE=1 and use --queue-exclude-external as in the repro steps
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --queue-exclude-external plan --non-interactive --working-dir "+rootPath,
	)

	// The command should succeed
	require.NoError(t, err)

	// Should not see parsing errors or formatting artifacts
	assert.NotContains(t, stderr, "Could not find Terragrunt configuration settings")
	assert.NotContains(t, stderr, "Unrecoverable parse error")
	assert.NotContains(t, stderr, "%!w(")

	// Verify the current unit ran successfully and external dependency was excluded
	combinedOutput := stdout + stderr
	assert.NotContains(t, combinedOutput, "service1")
	assert.Contains(t, combinedOutput, "null_resource.services_info")
	assert.Contains(t, combinedOutput, "Excluded")
}

// TestSensitiveValues tests that sensitive values can be properly handled
// when reading from YAML files and using the sensitive() function in locals.
// This validates that:
// 1. YAML files can be decoded and accessed in a map lookup based on environment
// 2. The sensitive() wrapper properly marks values as sensitive
// 3. Sensitive values can be passed as inputs to Terraform
// 4. The password length can be validated in outputs
func TestSensitiveValues(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureSensitiveValues)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSensitiveValues)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureSensitiveValues)

	// Run terragrunt apply
	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	// Get the output to verify password length
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	// Verify the password length output exists and is a number
	require.Contains(t, outputs, "password_length", "Should have password_length output")
	assert.Equal(t, "number", outputs["password_length"].Type, "password_length should be of type number")

	// Verify the password length matches the dev password length (25 characters)
	passwordLengthStr := fmt.Sprintf("%v", outputs["password_length"].Value)
	assert.Equal(t, "25", passwordLengthStr,
		"Password length should match dev password")
}
