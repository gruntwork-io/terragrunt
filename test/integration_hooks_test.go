package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureHooksBeforeOnlyPath                                = "fixtures/hooks/before-only"
	testFixtureHooksAllPath                                       = "fixtures/hooks/all"
	testFixtureHooksAfterOnlyPath                                 = "fixtures/hooks/after-only"
	testFixtureHooksBeforeAndAfterPath                            = "fixtures/hooks/before-and-after"
	testFixtureHooksBeforeAfterAndErrorMergePath                  = "fixtures/hooks/before-after-and-error-merge"
	testFixtureHooksSkipOnErrorPath                               = "fixtures/hooks/skip-on-error"
	testFixtureErrorHooksPath                                     = "fixtures/hooks/error-hooks"
	testFixtureHooksOneArgActionPath                              = "fixtures/hooks/one-arg-action"
	testFixtureHooksEmptyStringCommandPath                        = "fixtures/hooks/bad-arg-action/empty-string-command"
	testFixtureHooksEmptyCommandListPath                          = "fixtures/hooks/bad-arg-action/empty-command-list"
	testFixtureHooksInterpolationsPath                            = "fixtures/hooks/interpolations"
	testFixtureHooksInitOnceNoSourceNoBackend                     = "fixtures/hooks/init-once/no-source-no-backend"
	testFixtureHooksInitOnceNoSourceWithBackend                   = "fixtures/hooks/init-once/no-source-with-backend"
	testFixtureHooksInitOnceWithSourceNoBackend                   = "fixtures/hooks/init-once/with-source-no-backend"
	testFixtureHooksInitOnceWithSourceNoBackendSuppressHookStdout = "fixtures/hooks/init-once/with-source-no-backend-suppress-hook-stdout"
	testFixtureHooksInitOnceWithSourceWithBackend                 = "fixtures/hooks/init-once/with-source-with-backend"
)

func TestTerragruntBeforeHook(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksBeforeOnlyPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksBeforeOnlyPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksBeforeOnlyPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	_, exception := os.ReadFile(rootPath + "/file.out")

	require.NoError(t, exception)
}

func TestTerragruntInitHookNoSourceNoBackend(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksInitOnceNoSourceNoBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInitOnceNoSourceNoBackend)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
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

	helpers.CleanupTerraformFolder(t, testFixtureHooksInitOnceWithSourceNoBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInitOnceWithSourceNoBackend)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level trace", rootPath), &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")
	output := stdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_ONLY_ONCE\n"), "Hooks on init command executed more than once")
	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE\n"), "Hooks on init-from-module command executed more than once")
}

func TestTerragruntHookRunAllApply(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksAllPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksAllPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksAllPath)
	beforeOnlyPath := util.JoinPath(rootPath, "before-only")
	afterOnlyPath := util.JoinPath(rootPath, "after-only")

	helpers.RunTerragrunt(t, "terragrunt run-all apply -auto-approve --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	_, beforeErr := os.ReadFile(beforeOnlyPath + "/file.out")
	require.NoError(t, beforeErr)
	_, afterErr := os.ReadFile(afterOnlyPath + "/file.out")
	require.NoError(t, afterErr)
}

func TestTerragruntHookApplyAll(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksAllPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksAllPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksAllPath)
	beforeOnlyPath := util.JoinPath(rootPath, "before-only")
	afterOnlyPath := util.JoinPath(rootPath, "after-only")

	helpers.RunTerragrunt(t, "terragrunt apply-all -auto-approve --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	_, beforeErr := os.ReadFile(beforeOnlyPath + "/file.out")
	require.NoError(t, beforeErr)
	_, afterErr := os.ReadFile(afterOnlyPath + "/file.out")
	require.NoError(t, afterErr)
}

func TestTerragruntHookWorkingDir(t *testing.T) {
	t.Parallel()

	fixturePath := "fixtures/hooks/working_dir"
	helpers.CleanupTerraformFolder(t, fixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, fixturePath)
	rootPath := util.JoinPath(tmpEnvPath, fixturePath)

	helpers.RunTerragrunt(t, "terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
}

func TestTerragruntAfterHook(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksAfterOnlyPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksAfterOnlyPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksAfterOnlyPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	_, exception := os.ReadFile(rootPath + "/file.out")

	require.NoError(t, exception)
}

func TestTerragruntBeforeAndAfterHook(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksBeforeAndAfterPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksBeforeAndAfterPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)

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

	helpers.CleanupTerraformFolder(t, testFixtureHooksSkipOnErrorPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksSkipOnErrorPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksSkipOnErrorPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)

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

	helpers.CleanupTerraformFolder(t, testFixtureErrorHooksPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureErrorHooksPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureErrorHooksPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)

	require.Error(t, err)

	output := stderr.String()

	assert.Contains(t, output, "pattern_matching_hook")
	assert.Contains(t, output, "catch_all_matching_hook")
	assert.NotContains(t, output, "not_matching_hook")

}

func TestTerragruntCatchErrorsFromStdout(t *testing.T) {
	t.Parallel()

	if os.Getenv("TERRAGRUNT_PROVIDER_CACHE") == "1" {
		t.Skip()
	}

	helpers.CleanupTerraformFolder(t, testFixtureErrorHooksPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureErrorHooksPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureErrorHooksPath)
	tfPath := filepath.Join(rootPath, "tf.sh")

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath+" --terragrunt-tfpath "+tfPath, &stdout, &stderr)

	require.Error(t, err)

	output := stderr.String()

	assert.Contains(t, output, "pattern_matching_hook")
	assert.Contains(t, output, "catch_all_matching_hook")
	assert.NotContains(t, output, "not_matching_hook")
}

func TestTerragruntBeforeOneArgAction(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksOneArgActionPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksOneArgActionPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksOneArgActionPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level trace", rootPath), &stdout, &stderr)
	output := stderr.String()

	if err != nil {
		t.Error("Expected successful execution of terragrunt with 1 before hook execution.")
	} else {
		assert.Contains(t, output, "Running command: date")
	}
}

func TestTerragruntEmptyStringCommandHook(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksEmptyStringCommandPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksEmptyStringCommandPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksEmptyStringCommandPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)

	if err != nil {
		assert.Contains(t, err.Error(), "Need at least one non-empty argument in 'execute'.")
	} else {
		t.Error("Expected an Error with message: 'Need at least one argument'")
	}
}

func TestTerragruntEmptyCommandListHook(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksEmptyCommandListPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksEmptyCommandListPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksEmptyCommandListPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)

	if err != nil {
		assert.Contains(t, err.Error(), "Need at least one non-empty argument in 'execute'.")
	} else {
		t.Error("Expected an Error with message: 'Need at least one argument'")
	}
}

func TestTerragruntHookInterpolation(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksInterpolationsPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksInterpolationsPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInterpolationsPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
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

	helpers.CleanupTerraformFolder(t, testFixtureHooksInitOnceWithSourceNoBackendSuppressHookStdout)
	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInitOnceWithSourceNoBackendSuppressHookStdout)

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt terragrunt-info --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &showStdout, &showStderr)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")

	var dat terragruntinfo.TerragruntInfoGroup
	errUnmarshal := json.Unmarshal(showStdout.Bytes(), &dat)
	require.NoError(t, errUnmarshal)

	assert.Equal(t, fmt.Sprintf("%s/%s", rootPath, helpers.TerragruntCache), dat.DownloadDir)
	assert.Equal(t, wrappedBinary(), dat.TerraformBinary)
	assert.Empty(t, dat.IamRole)
}
