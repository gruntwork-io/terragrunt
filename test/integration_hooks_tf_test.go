//go:build tf

package test_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFTerragruntHookIfParameter(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureTerragruntHookIfParameter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTerragruntHookIfParameter)
	rootPath := filepath.Join(tmpEnvPath, testFixtureTerragruntHookIfParameter)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)

	require.NoError(t, err)

	output := stdout.String()

	assert.Contains(t, output, "running before hook")
	assert.NotContains(t, output, "skip after hook")
}

func TestTFTerragruntBeforeHook(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksBeforeOnlyPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksBeforeOnlyPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksBeforeOnlyPath)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	_, exception := os.ReadFile(rootPath + "/file.out")

	require.NoError(t, exception)
}

func TestTFTerragruntInitHookNoSourceNoBackend(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksInitOnceNoSourceNoBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/hooks/init-once")
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksInitOnceNoSourceNoBackend)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	output := stdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(
		t,
		1,
		strings.Count(output, "AFTER_INIT_ONLY_ONCE"),
		"Hooks on init command executed more than once",
	)
	// With source always being "." (current directory), init-from-module executes once
	assert.Equal(
		t,
		1,
		strings.Count(output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE"),
		"Hooks on init-from-module command should execute once",
	)
}

func TestTFTerragruntInitHookWithSourceNoBackend(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksInitOnceWithSourceNoBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/hooks/init-once")
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksInitOnceWithSourceNoBackend)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --working-dir %s --log-level trace",
			rootPath,
		),
		&stdout,
		&stderr,
	)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")

	output := stdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(t, 1, strings.Count(
		output, "AFTER_INIT_ONLY_ONCE\n",
	), "Hooks on init command executed more than once")

	assert.Equal(t, 1, strings.Count(
		output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE\n",
	), "Hooks on init-from-module command executed more than once")
}

func TestTFTerragruntHookRunAllApply(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksAllPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksAllPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksAllPath)
	beforeOnlyPath := filepath.Join(rootPath, "before-only")
	afterOnlyPath := filepath.Join(rootPath, "after-only")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	_, beforeErr := os.ReadFile(beforeOnlyPath + "/file.out")
	require.NoError(t, beforeErr)

	_, afterErr := os.ReadFile(afterOnlyPath + "/file.out")
	require.NoError(t, afterErr)
}

func TestTFTerragruntRunNoHooksSkipsConfiguredHooks(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksNoHooks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksNoHooks)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksNoHooks)
	directPath := filepath.Join(rootPath, "direct")
	stackPath := filepath.Join(rootPath, "stack")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --experiment optional-hooks --no-hooks --non-interactive --working-dir "+directPath+
			" -- plan -input=false",
	)

	require.Error(t, err)
	assert.Contains(t, stderr, "direct_required")
	assertNoHookOutputFiles(t, directPath)

	_, stderr, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --experiment optional-hooks --no-hooks --non-interactive --working-dir "+stackPath+
			" -- plan -input=false",
	)

	require.Error(t, err)
	assert.Contains(t, stderr, "unit_a_required")
	assert.Contains(t, stderr, "unit_b_required")
	assertNoHookOutputFiles(
		t,
		filepath.Join(stackPath, "unit-a"),
		filepath.Join(stackPath, "unit-b"),
	)
}

func TestTFTerragruntHookApplyAll(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksAllPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksAllPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksAllPath)
	beforeOnlyPath := filepath.Join(rootPath, "before-only")
	afterOnlyPath := filepath.Join(rootPath, "after-only")

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	_, beforeErr := os.ReadFile(beforeOnlyPath + "/file.out")
	require.NoError(t, beforeErr)

	_, afterErr := os.ReadFile(afterOnlyPath + "/file.out")
	require.NoError(t, afterErr)
}

func TestTFTerragruntHookWorkingDir(t *testing.T) {
	t.Parallel()

	fixturePath := "fixtures/hooks/working_dir"
	helpers.CleanupTerraformFolder(t, fixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, fixturePath)
	rootPath := filepath.Join(tmpEnvPath, fixturePath)

	helpers.RunTerragrunt(t, "terragrunt validate --non-interactive --working-dir "+rootPath)
}

func TestTFTerragruntAfterHook(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksAfterOnlyPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksAfterOnlyPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksAfterOnlyPath)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	_, exception := os.ReadFile(rootPath + "/file.out")

	require.NoError(t, exception)
}

func TestTFTerragruntBeforeAndAfterHook(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksBeforeAndAfterPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksBeforeAndAfterPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)

	_, beforeException := os.ReadFile(rootPath + "/before.out")
	_, afterException := os.ReadFile(rootPath + "/after.out")

	output := stdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(
		t,
		0,
		strings.Count(output, "BEFORE_TERRAGRUNT_READ_CONFIG"),
		"terragrunt-read-config before_hook should not be triggered",
	)
	t.Logf("output: %s", output)

	assert.Equal(
		t,
		1,
		strings.Count(output, "AFTER_TERRAGRUNT_READ_CONFIG"),
		"Hooks on terragrunt-read-config command executed more than once",
	)

	expectedHookOutput := fmt.Sprintf(
		"TF_PATH=%s COMMAND=terragrunt-read-config HOOK_NAME=after_hook_3",
		wrappedBinary(t.Context()),
	)
	assert.Equal(t, 1, strings.Count(output, expectedHookOutput))

	require.NoError(t, beforeException)
	require.NoError(t, afterException)
}

func TestTFTerragruntSkipOnError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksSkipOnErrorPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksSkipOnErrorPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksSkipOnErrorPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)

	require.Error(t, err)

	output := stdout.String()

	assert.Contains(t, output, "BEFORE_SHOULD_DISPLAY")
	assert.NotContains(t, output, "BEFORE_NODISPLAY")

	assert.Contains(t, output, "AFTER_SHOULD_DISPLAY")
	assert.NotContains(t, output, "AFTER_NODISPLAY")

	assert.Contains(t, output, "ERROR_HOOK_EXECUTED")
	assert.NotContains(t, output, "NOT_MATCHING_ERROR_HOOK")
	assert.Contains(t, output, "PATTERN_MATCHING_ERROR_HOOK")
}

func TestTFTerragruntCatchErrorsInTerraformExecution(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureErrorHooksPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureErrorHooksPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureErrorHooksPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)

	require.Error(t, err)

	output := stderr.String()

	assert.Contains(t, output, "pattern_matching_hook")
	assert.Contains(t, output, "catch_all_matching_hook")
	assert.NotContains(t, output, "not_matching_hook")
}

func TestTFTerragruntCatchErrorsFromStdout(t *testing.T) {
	t.Parallel()

	if helpers.IsTerragruntProviderCacheEnabled(t) {
		t.Skip()
	}

	helpers.CleanupTerraformFolder(t, testFixtureErrorHooksPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureErrorHooksPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureErrorHooksPath)
	tfPath := filepath.Join(rootPath, "tf.sh")

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath+" --tf-path "+tfPath,
		&stdout,
		&stderr,
	)

	require.Error(t, err)

	output := stderr.String()

	assert.Contains(t, output, "pattern_matching_hook")
	assert.Contains(t, output, "catch_all_matching_hook")
	assert.NotContains(t, output, "not_matching_hook")
}

func TestTFTerragruntErrorHookTriggeredOnSourceDownloadFail(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureErrorHooksSourceDownloadFail)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureErrorHooksSourceDownloadFail)
	rootPath := filepath.Join(tmpEnvPath, testFixtureErrorHooksSourceDownloadFail)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --non-interactive --working-dir "+rootPath+
			" -- apply -auto-approve",
	)
	require.Error(t, err)

	// Hook output goes to stdout, check both stdout and stderr
	output := stdout + stderr
	// Verify error hook for init-from-module is triggered when source download fails
	assert.Contains(t, output, "ERROR_HOOK_TRIGGERED_ON_INIT_FROM_MODULE",
		"Error hook for 'init-from-module' should be triggered when source download fails")
}

func TestTFTerragruntBeforeOneArgAction(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksOneArgActionPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksOneArgActionPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksOneArgActionPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --working-dir %s --log-level trace",
			rootPath,
		),
		&stdout,
		&stderr,
	)
	output := stderr.String()

	if err != nil {
		t.Error("Expected successful execution of terragrunt with 1 before hook execution.")
	} else {
		assert.Contains(t, output, "Running command: date")
	}
}

func TestTFTerragruntEmptyStringCommandHook(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksEmptyStringCommandPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksEmptyStringCommandPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksEmptyStringCommandPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	if err != nil {
		assert.Contains(t, err.Error(), "Need at least one non-empty argument in 'execute'.")
	} else {
		t.Error("Expected an Error with message: 'Need at least one argument'")
	}
}

func TestTFTerragruntEmptyCommandListHook(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksEmptyCommandListPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksEmptyCommandListPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksEmptyCommandListPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	if err != nil {
		assert.Contains(t, err.Error(), "Need at least one non-empty argument in 'execute'.")
	} else {
		t.Error("Expected an Error with message: 'Need at least one argument'")
	}
}

func TestTFTerragruntHookInterpolation(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksInterpolationsPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksInterpolationsPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksInterpolationsPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	output := stdout.String()

	homePath := os.Getenv("HOME")
	if homePath == "" {
		homePath = "HelloWorld"
	}

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Contains(t, output, homePath)
}

func TestTFTerragruntHookPreservesAbsolutePaths(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksPathPreservation)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksPathPreservation)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksPathPreservation)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	require.Error(t, err)

	// The absolute path should be preserved exactly as the hook output it
	// NOT converted to a relative path like "../../../.terraform.d/plugin-cache"
	assert.Contains(t, stderr, "/home/testuser/.terraform.d/plugin-cache")
	assert.NotContains(t, stderr, "../")
}

func TestTFTerragruntHookContextEnvExperimentEnabled(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksContextEnv)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksContextEnv)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksContextEnv)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --experiment hook-context-env --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	beforeOut, readErr := os.ReadFile(filepath.Join(rootPath, "before.out"))
	require.NoError(t, readErr)
	afterOut, readErr := os.ReadFile(filepath.Join(rootPath, "after.out"))
	require.NoError(t, readErr)
	errorOut, readErr := os.ReadFile(filepath.Join(rootPath, "error.out"))
	require.NoError(t, readErr)

	wantSource := filepath.Join(rootPath, "modules", "foo")

	assert.Contains(t, string(beforeOut), "TG_CTX_HOOK_TYPE=before_hook")
	assert.Contains(t, string(beforeOut), "TG_CTX_HOOK_NAME=shared_name_hook")
	assert.Contains(t, string(beforeOut), "TG_CTX_TERRAGRUNT_DIR="+rootPath)
	assert.Contains(t, string(beforeOut), "TG_CTX_SOURCE="+wantSource)

	assert.Contains(t, string(afterOut), "TG_CTX_HOOK_TYPE=after_hook")
	assert.Contains(t, string(afterOut), "TG_CTX_HOOK_NAME=shared_name_hook")
	assert.Contains(t, string(afterOut), "TG_CTX_TERRAGRUNT_DIR="+rootPath)
	assert.Contains(t, string(afterOut), "TG_CTX_SOURCE="+wantSource)

	assert.Contains(t, string(errorOut), "TG_CTX_HOOK_TYPE=error_hook")
	assert.Contains(t, string(errorOut), "TG_CTX_HOOK_NAME=error_hook_1")
	assert.Contains(t, string(errorOut), "TG_CTX_TERRAGRUNT_DIR="+rootPath)
	assert.Contains(t, string(errorOut), "TG_CTX_SOURCE="+wantSource)
}

func TestTFTerragruntHookContextEnvExperimentDisabled(t *testing.T) {
	t.Parallel()

	if helpers.IsExperimentMode(t) {
		t.Skip()
	}

	helpers.CleanupTerraformFolder(t, testFixtureHooksContextEnv)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksContextEnv)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksContextEnv)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	beforeOut, readErr := os.ReadFile(filepath.Join(rootPath, "before.out"))
	require.NoError(t, readErr)
	afterOut, readErr := os.ReadFile(filepath.Join(rootPath, "after.out"))
	require.NoError(t, readErr)
	errorOut, readErr := os.ReadFile(filepath.Join(rootPath, "error.out"))
	require.NoError(t, readErr)

	// The existing context env vars are still set unconditionally.
	assert.Contains(t, string(beforeOut), "TG_CTX_HOOK_NAME=shared_name_hook")

	// The new env vars must NOT be set when the experiment is disabled.
	for _, out := range [][]byte{beforeOut, afterOut, errorOut} {
		assert.Contains(t, string(out), "TG_CTX_HOOK_TYPE=<unset>")
		assert.Contains(t, string(out), "TG_CTX_SOURCE=<unset>")
		assert.Contains(t, string(out), "TG_CTX_TERRAGRUNT_DIR=<unset>")
	}
}

func TestTFTerragruntHookExitCodeError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksExitCodeError)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksExitCodeError)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksExitCodeError)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	output := stderr.String()
	// Error message should show exit code and the actual hook output
	assert.Contains(t, output, `exited with non-zero exit code 2`)
	assert.Contains(t, output, "lint warning: something is wrong")
}
