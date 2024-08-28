package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureHooksBeforeOnlyPath                                = "fixture-hooks/before-only"
	testFixtureHooksAllPath                                       = "fixture-hooks/all"
	testFixtureHooksAfterOnlyPath                                 = "fixture-hooks/after-only"
	testFixtureHooksBeforeAndAfterPath                            = "fixture-hooks/before-and-after"
	testFixtureHooksBeforeAfterAndErrorMergePath                  = "fixture-hooks/before-after-and-error-merge"
	testFixtureHooksSkipOnErrorPath                               = "fixture-hooks/skip-on-error"
	testFixtureErrorHooksPath                                     = "fixture-hooks/error-hooks"
	testFixtureHooksOneArgActionPath                              = "fixture-hooks/one-arg-action"
	testFixtureHooksEmptyStringCommandPath                        = "fixture-hooks/bad-arg-action/empty-string-command"
	testFixtureHooksEmptyCommandListPath                          = "fixture-hooks/bad-arg-action/empty-command-list"
	testFixtureHooksInterpolationsPath                            = "fixture-hooks/interpolations"
	testFixtureHooksInitOnceNoSourceNoBackend                     = "fixture-hooks/init-once/no-source-no-backend"
	testFixtureHooksInitOnceNoSourceWithBackend                   = "fixture-hooks/init-once/no-source-with-backend"
	testFixtureHooksInitOnceWithSourceNoBackend                   = "fixture-hooks/init-once/with-source-no-backend"
	testFixtureHooksInitOnceWithSourceNoBackendSuppressHookStdout = "fixture-hooks/init-once/with-source-no-backend-suppress-hook-stdout"
	testFixtureHooksInitOnceWithSourceWithBackend                 = "fixture-hooks/init-once/with-source-with-backend"
)

func TestTerragruntBeforeHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksBeforeOnlyPath)
	tmpEnvPath := copyEnvironment(t, testFixtureHooksBeforeOnlyPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksBeforeOnlyPath)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	_, exception := os.ReadFile(rootPath + "/file.out")

	require.NoError(t, exception)
}

func TestTerragruntInitHookNoSourceNoBackend(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksInitOnceNoSourceNoBackend)
	tmpEnvPath := copyEnvironment(t, "fixture-hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInitOnceNoSourceNoBackend)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
	output := stdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_ONLY_ONCE"), "Hooks on init command executed more than once")
	// With no source, `init-from-module` should not execute
	assert.NotContains(t, output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE", "Hooks on init-from-module command executed when no source was specified")
}

func TestTerragruntInitHookWithSourceNoBackend(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksInitOnceWithSourceNoBackend)
	tmpEnvPath := copyEnvironment(t, "fixture-hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInitOnceWithSourceNoBackend)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level debug", rootPath), &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")
	output := stdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_ONLY_ONCE\n"), "Hooks on init command executed more than once")
	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE\n"), "Hooks on init-from-module command executed more than once")
}

func TestTerragruntHookRunAllApply(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksAllPath)
	tmpEnvPath := copyEnvironment(t, testFixtureHooksAllPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksAllPath)
	beforeOnlyPath := util.JoinPath(rootPath, "before-only")
	afterOnlyPath := util.JoinPath(rootPath, "after-only")

	runTerragrunt(t, "terragrunt run-all apply -auto-approve --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	_, beforeErr := os.ReadFile(beforeOnlyPath + "/file.out")
	require.NoError(t, beforeErr)
	_, afterErr := os.ReadFile(afterOnlyPath + "/file.out")
	require.NoError(t, afterErr)
}

func TestTerragruntHookApplyAll(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksAllPath)
	tmpEnvPath := copyEnvironment(t, testFixtureHooksAllPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksAllPath)
	beforeOnlyPath := util.JoinPath(rootPath, "before-only")
	afterOnlyPath := util.JoinPath(rootPath, "after-only")

	runTerragrunt(t, "terragrunt apply-all -auto-approve --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	_, beforeErr := os.ReadFile(beforeOnlyPath + "/file.out")
	require.NoError(t, beforeErr)
	_, afterErr := os.ReadFile(afterOnlyPath + "/file.out")
	require.NoError(t, afterErr)
}

func TestTerragruntHookWorkingDir(t *testing.T) {
	t.Parallel()

	fixturePath := "fixture-hooks/working_dir"
	cleanupTerraformFolder(t, fixturePath)
	tmpEnvPath := copyEnvironment(t, fixturePath)
	rootPath := util.JoinPath(tmpEnvPath, fixturePath)

	runTerragrunt(t, "terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
}

func TestTerragruntAfterHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksAfterOnlyPath)
	tmpEnvPath := copyEnvironment(t, testFixtureHooksAfterOnlyPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksAfterOnlyPath)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	_, exception := os.ReadFile(rootPath + "/file.out")

	require.NoError(t, exception)
}

func TestTerragruntBeforeAndAfterHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := copyEnvironment(t, testFixtureHooksBeforeAndAfterPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksBeforeAndAfterPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)

	_, beforeException := os.ReadFile(rootPath + "/before.out")
	_, afterException := os.ReadFile(rootPath + "/after.out")

	output := stdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(t, 0, strings.Count(output, "BEFORE_TERRAGRUNT_READ_CONFIG"), "terragrunt-read-config before_hook should not be triggered")
	t.Logf("output: %s", output)

	assert.Equal(t, 1, strings.Count(output, "AFTER_TERRAGRUNT_READ_CONFIG"), "Hooks on terragrunt-read-config command executed more than once")

	expectedHookOutput := fmt.Sprintf("TF_PATH=%s COMMAND=terragrunt-read-config HOOK_NAME=after_hook_3", wrappedBinary())
	assert.Equal(t, 1, strings.Count(output, expectedHookOutput))

	require.NoError(t, beforeException)
	require.NoError(t, afterException)
}

func TestTerragruntSkipOnError(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksSkipOnErrorPath)
	tmpEnvPath := copyEnvironment(t, testFixtureHooksSkipOnErrorPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksSkipOnErrorPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)

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

func TestTerragruntCatchErrorsInTerraformExecution(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureErrorHooksPath)
	tmpEnvPath := copyEnvironment(t, testFixtureErrorHooksPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureErrorHooksPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)

	require.Error(t, err)

	output := stderr.String()

	assert.Contains(t, output, "pattern_matching_hook")
	assert.Contains(t, output, "catch_all_matching_hook")
	assert.NotContains(t, output, "not_matching_hook")

}

func TestTerragruntBeforeOneArgAction(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksOneArgActionPath)
	tmpEnvPath := copyEnvironment(t, testFixtureHooksOneArgActionPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksOneArgActionPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level debug", rootPath), &stdout, &stderr)
	output := stderr.String()

	if err != nil {
		t.Error("Expected successful execution of terragrunt with 1 before hook execution.")
	} else {
		assert.Contains(t, output, "Running command: date")
	}
}

func TestTerragruntEmptyStringCommandHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksEmptyStringCommandPath)
	tmpEnvPath := copyEnvironment(t, testFixtureHooksEmptyStringCommandPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksEmptyStringCommandPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)

	if err != nil {
		assert.Contains(t, err.Error(), "Need at least one non-empty argument in 'execute'.")
	} else {
		t.Error("Expected an Error with message: 'Need at least one argument'")
	}
}

func TestTerragruntEmptyCommandListHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksEmptyCommandListPath)
	tmpEnvPath := copyEnvironment(t, testFixtureHooksEmptyCommandListPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksEmptyCommandListPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)

	if err != nil {
		assert.Contains(t, err.Error(), "Need at least one non-empty argument in 'execute'.")
	} else {
		t.Error("Expected an Error with message: 'Need at least one argument'")
	}
}

func TestTerragruntHookInterpolation(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksInterpolationsPath)
	tmpEnvPath := copyEnvironment(t, testFixtureHooksInterpolationsPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInterpolationsPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
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

func TestTerragruntInfo(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHooksInitOnceWithSourceNoBackendSuppressHookStdout)
	tmpEnvPath := copyEnvironment(t, "fixture-hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInitOnceWithSourceNoBackendSuppressHookStdout)

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt terragrunt-info --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &showStdout, &showStderr)
	require.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")

	var dat terragruntinfo.TerragruntInfoGroup
	errUnmarshal := json.Unmarshal(showStdout.Bytes(), &dat)
	require.NoError(t, errUnmarshal)

	assert.Equal(t, fmt.Sprintf("%s/%s", rootPath, terragruntCache), dat.DownloadDir)
	assert.Equal(t, wrappedBinary(), dat.TerraformBinary)
	assert.Empty(t, dat.IamRole)
}
