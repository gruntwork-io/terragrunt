package test_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureRegressions                       = "fixtures/regressions"
	testFixtureDependencyGenerate                = "fixtures/regressions/dependency-generate"
	testFixtureDependencyEmptyConfigPath         = "fixtures/regressions/dependency-empty-config-path"
	testFixtureDisabledDependencyEmptyConfigPath = "fixtures/regressions/disabled-dependency-empty-config-path"
	testFixtureParsingDeprecated                 = "fixtures/parsing/exposed-include-with-deprecated-inputs"
	testFixtureSensitiveValues                   = "fixtures/regressions/sensitive-values"
	testFixtureStackDetection                    = "fixtures/regressions/multiple-stacks"
	testFixtureScopeEscape                       = "fixtures/regressions/5195-scope-escape"
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
	// During run --all, the cache is created at the root working directory level
	generatedFile := util.JoinPath(rootPath, ".terragrunt-cache")
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

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

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
// Note: As of #5195 fix, external dependencies are excluded by default, so --queue-exclude-external
// is now deprecated and acts as a no-op. This test verifies the behavior still works correctly.
func TestRunAllWithGenerateAndExpose_WithProviderCacheAndExcludeExternal(t *testing.T) {
	t.Parallel()

	testFixture := "fixtures/regressions/parsing-run-all-with-generate"
	helpers.CleanupTerraformFolder(t, testFixture)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixture)
	rootPath := util.JoinPath(tmpEnvPath, testFixture, "services-info")

	// Use --queue-exclude-external as in the repro steps (now deprecated, acts as no-op since
	// external dependencies are excluded by default after #5195 fix)
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

// TestDisabledDependencyEmptyConfigPath_NoCycleError tests that disabled dependencies with empty
// config_path values do not cause cycle detection errors during discovery.
// This is a regression test for issue #4977 where setting enabled = false on a dependency
// with an empty config_path ("") was still causing terragrunt to throw cycle errors.
//
// The expected behavior is that disabled dependencies should be completely ignored during
// dependency graph construction and cycle detection, regardless of their config_path value.
//
// See: https://github.com/gruntwork-io/terragrunt/issues/4977
func TestDisabledDependencyEmptyConfigPath_NoCycleError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDisabledDependencyEmptyConfigPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDisabledDependencyEmptyConfigPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDisabledDependencyEmptyConfigPath)
	helpers.CreateGitRepo(t, rootPath)

	unitBPath := util.JoinPath(rootPath, "unit-b")
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --working-dir "+unitBPath,
	)

	require.NoError(t, err, "plan should succeed when disabled dependency has empty config_path")

	combinedOutput := stdout + stderr
	assert.NotContains(t, combinedOutput, "cycle",
		"Should not see cycle detection errors for disabled dependencies")
	assert.NotContains(t, combinedOutput, "Cycle detected",
		"Should not see 'Cycle detected' error")

	assert.NotContains(t, combinedOutput, "has empty config_path",
		"Should not see empty config_path error for disabled dependency")

	_, runAllStderr, runAllErr := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all plan --non-interactive --working-dir "+rootPath,
	)

	require.NoError(t, runAllErr, "run --all plan should succeed")

	assert.NotContains(t, runAllStderr, "cycle",
		"run --all should not see cycle errors")
	assert.NotContains(t, runAllStderr, "dependency graph",
		"run --all should not see dependency graph errors")
}

func TestMultipleStacksDetection(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDetection)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDetection)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStackDetection, "live")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+rootPath)

	require.NoError(t, err)

	assert.Contains(t, stderr, "terragrunt.stack.hcl")
	assert.Contains(t, stderr, "unit1")
	assert.Contains(t, stderr, "unit2")

	assert.NotContains(t, stderr, "appv2.terragrunt.stack.hcl")
	assert.NotContains(t, stderr, "unit4")
	assert.NotContains(t, stderr, "unit3")
}

// flushTrackingWriter wraps a writer and tracks writes and output size changes (which indicate flushes)
type flushTrackingWriter struct {
	w      io.Writer
	signal chan<- struct{}
	mu     sync.Mutex
	writes int
	once   bool
}

func (ftw *flushTrackingWriter) Write(p []byte) (int, error) {
	ftw.mu.Lock()
	ftw.writes++

	shouldSignal := !ftw.once && ftw.signal != nil
	if shouldSignal {
		ftw.once = true
	}

	ftw.mu.Unlock()

	if shouldSignal {
		select {
		case ftw.signal <- struct{}{}:
		default:
		}
	}

	return ftw.w.Write(p)
}

func (ftw *flushTrackingWriter) getWriteCount() int {
	ftw.mu.Lock()
	defer ftw.mu.Unlock()

	return ftw.writes
}

// TestOutputFlushOnInterrupt verifies that buffered output is flushed when context is cancelled.
func TestOutputFlushOnInterrupt(t *testing.T) {
	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows - signal handling differs")
	}

	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDependencyOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyOutput)
	testPath := util.JoinPath(tmpEnvPath, testFixtureDependencyOutput, "app")
	dependencyPath := util.JoinPath(tmpEnvPath, testFixtureDependencyOutput, "dependency")

	// Initialize and apply dependency first so outputs are available
	helpers.RunTerragrunt(t, "terragrunt init --non-interactive --working-dir "+dependencyPath)
	helpers.RunTerragrunt(t, "terragrunt apply --auto-approve --non-interactive --working-dir "+dependencyPath)
	helpers.RunTerragrunt(t, "terragrunt init --non-interactive --working-dir "+testPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstWrite := make(chan struct{}, 1)

	var stdoutBuf, stderrBuf strings.Builder

	stdout := &flushTrackingWriter{w: &stdoutBuf, signal: firstWrite}
	stderr := &flushTrackingWriter{w: &stderrBuf, signal: firstWrite}
	cmdErr := make(chan error, 1)

	go func() {
		cmdErr <- helpers.RunTerragruntCommandWithContext(t, ctx, "terragrunt run --all apply --non-interactive --working-dir "+testPath, stdout, stderr)
	}()

	// Wait for first write, then cancel to test flush on interrupt
	var outputBeforeCancel string

	var writesBeforeCancel int

	select {
	case <-firstWrite:
		outputBeforeCancel = stdoutBuf.String() + stderrBuf.String()
		writesBeforeCancel = stdout.getWriteCount() + stderr.getWriteCount()
		t.Logf("First write detected (%d bytes, %d writes), cancelling context", len(outputBeforeCancel), writesBeforeCancel)
		cancel()
	case <-cmdErr:
		t.Fatal("Command finished before we could interrupt it")
	case <-time.After(3 * time.Second):
		t.Fatal("No output appeared before timeout")
	}

	// Wait briefly for flush to occur after cancellation
	time.Sleep(200 * time.Millisecond)

	outputAfterCancel := stdoutBuf.String() + stderrBuf.String()
	writesAfterCancel := stdout.getWriteCount() + stderr.getWriteCount()

	// Wait for command to finish or timeout
	select {
	case <-cmdErr:
		// Command finished
	case <-time.After(5 * time.Second):
		t.Logf("Command still running after cancellation")
	}

	output := stdoutBuf.String() + stderrBuf.String()
	totalWrites := stdout.getWriteCount() + stderr.getWriteCount()

	t.Logf("Output length: before cancel=%d, after cancel=%d, final=%d", len(outputBeforeCancel), len(outputAfterCancel), len(output))
	t.Logf("Total writes: before cancel=%d, after cancel=%d, final=%d", writesBeforeCancel, writesAfterCancel, totalWrites)

	// Verify that output increased after cancellation (indicating flush occurred)
	require.Greater(t, len(outputAfterCancel), len(outputBeforeCancel), "Output should increase after cancellation due to flush")
	require.Greater(t, totalWrites, writesBeforeCancel, "Additional writes should occur after cancellation (flush writes)")
	require.NotEmpty(t, output, "Expected output to be flushed after cancellation")
}

// TestRunAllDoesNotIncludeExternalDepsInQueue tests that running `terragrunt run --all` from a subdirectory
// does NOT include external dependencies in the execution queue.
// This is a regression test for issue #5195 where v0.94.0 incorrectly included external dependencies
// in the run queue, causing dangerous operations like destroy to execute against unintended modules.
//
// The test structure is:
//
//	fixtures/regressions/5195-scope-escape/
//	├── bastion/           <- Run from here, has dependency on module2
//	│   ├── terragrunt.hcl (depends on ../module2 with mock_outputs)
//	│   └── main.tf
//	├── module1/           <- Has dependency on bastion
//	│   ├── terragrunt.hcl
//	│   └── main.tf
//	└── module2/           <- External dependency of bastion
//	    ├── terragrunt.hcl
//	    └── main.tf
//
// Expected behavior (v0.93.13 and after fix):
//   - External dependency (module2) is discovered but EXCLUDED from execution
//   - Summary shows "Excluded 1" for the external dep
//   - Only bastion (Unit .) is actually executed
//
// Bug behavior (v0.94.0):
//   - External dependency is included and EXECUTED (not excluded)
//   - This causes unintended operations on external modules
//
// See: https://github.com/gruntwork-io/terragrunt/issues/5195
func TestRunAllDoesNotIncludeExternalDepsInQueue(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureScopeEscape)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureScopeEscape)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureScopeEscape)
	bastionPath := util.JoinPath(rootPath, "bastion")

	// Initialize git repo - this is important because discovery uses git root for scope
	helpers.CreateGitRepo(t, rootPath)

	// Run terragrunt run --all plan from the bastion directory
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all plan --log-level debug --non-interactive --working-dir "+bastionPath,
	)

	// The command should succeed
	require.NoError(t, err)

	// External dependencies should be excluded (not executed)
	// The log should show they were excluded during discovery
	assert.Contains(t, stderr, "Excluded external dependency",
		"External dependencies should be logged as excluded")

	// Should see bastion (displayed as "." since it's the current directory)
	assert.Contains(t, stderr, "Unit .",
		"Should discover the current directory (bastion) as '.'")

	// Report shows 2 units (bastion + excluded external dep)
	assert.Contains(t, stdout, "2 units",
		"Should have 2 units total (bastion + excluded external dep)")
	assert.Contains(t, stdout, "Succeeded    1",
		"Only bastion should succeed")
	assert.Contains(t, stdout, "Excluded     1",
		"External dependency should be excluded in report")
}

// TestRunAllFromParentDiscoversAllModules verifies that running from the parent directory
// correctly discovers all modules in the hierarchy. This is the control test for
// TestRunAllDoesNotEscapeWorkingDir.
func TestRunAllFromParentDiscoversAllModules(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureScopeEscape)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureScopeEscape)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureScopeEscape)

	// Initialize git repo - this is important because discovery uses git root for scope
	helpers.CreateGitRepo(t, rootPath)

	// Run terragrunt run --all plan from the parent directory (live/)
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all plan --non-interactive --working-dir "+rootPath,
	)

	// The command should succeed in terms of discovery
	_ = err

	// Should see all three modules when running from parent directory
	assert.Contains(t, stderr, "bastion",
		"Should discover bastion when running from parent directory")
	assert.Contains(t, stderr, "module1",
		"Should discover module1 when running from parent directory")
	assert.Contains(t, stderr, "module2",
		"Should discover module2 when running from parent directory")
}
