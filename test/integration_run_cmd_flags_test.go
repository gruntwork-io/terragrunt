package test_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	testFixtureRunCmdFlags            = "fixtures/run-cmd-flags"
	testFixtureRunCmdModuleQuiet      = "fixtures/run-cmd-flags/module-quiet"
	testFixtureRunCmdModuleGlobalA    = "fixtures/run-cmd-flags/module-global-cache-a"
	testFixtureRunCmdModuleGlobalB    = "fixtures/run-cmd-flags/module-global-cache-b"
	testFixtureRunCmdModuleNoCache    = "fixtures/run-cmd-flags/module-no-cache"
	testFixtureRunCmdModuleConflict   = "fixtures/run-cmd-flags/module-conflict"
	runCmdSecretValue                 = "TOP_SECRET_TOKEN"
	expectedGlobalCachedValue         = "global-value-1"
	unexpectedGlobalCachedSecondValue = "global-value-2"
	expectedNoCacheFirstValue         = "no-cache-value-1"
)

type runCmdFixtureResult struct {
	rootPath string
	stdout   string
	stderr   string
}

func runCmdFlagsFixture(t *testing.T) runCmdFixtureResult {
	t.Helper()

	for _, modulePath := range []string{
		testFixtureRunCmdModuleQuiet,
		testFixtureRunCmdModuleGlobalA,
		testFixtureRunCmdModuleGlobalB,
		testFixtureRunCmdModuleNoCache,
		testFixtureRunCmdModuleConflict,
	} {
		helpers.CleanupTerraformFolder(t, modulePath)
	}

	// Clean up counter files from previous test runs in the fixture directory
	scriptsPath := filepath.Join(testFixtureRunCmdFlags, "scripts")
	_ = os.Remove(filepath.Join(scriptsPath, "global_counter.txt"))
	_ = os.Remove(filepath.Join(scriptsPath, "no_cache_counter.txt"))

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRunCmdFlags)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRunCmdFlags)

	// Remove the conflicting module so the happy-path tests can run `terragrunt run --all` without errors.
	conflictDir := filepath.Join(rootPath, "module-conflict")
	require.NoError(t, os.RemoveAll(conflictDir))

	cmd := "terragrunt run --all plan --non-interactive --log-level debug --working-dir " + rootPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	// Clean up counter files after test execution
	t.Cleanup(func() {
		scriptsPath := filepath.Join(testFixtureRunCmdFlags, "scripts")
		_ = os.Remove(filepath.Join(scriptsPath, "global_counter.txt"))
		_ = os.Remove(filepath.Join(scriptsPath, "no_cache_counter.txt"))
	})

	return runCmdFixtureResult{
		rootPath: rootPath,
		stdout:   stdout,
		stderr:   stderr,
	}
}

func TestRunCmdQuietRedactsOutput(t *testing.T) {
	t.Parallel()

	result := runCmdFlagsFixture(t)

	assert.Contains(t, result.stderr, "run_cmd output: [REDACTED]")
	assert.NotContains(t, result.stderr, runCmdSecretValue)
}

func TestRunCmdGlobalCacheSharesResultAcrossModules(t *testing.T) {
	t.Parallel()

	result := runCmdFlagsFixture(t)

	combinedOutput := strings.Join([]string{result.stdout, result.stderr}, "\n")

	globalCounterPath := filepath.Join(result.rootPath, "scripts", "global_counter.txt")
	globalCounterBytes, readErr := os.ReadFile(globalCounterPath)
	require.NoError(t, readErr)

	assert.Equal(t, "1", strings.TrimSpace(string(globalCounterBytes)))
	assert.Contains(t, combinedOutput, expectedGlobalCachedValue)
	assert.NotContains(t, combinedOutput, unexpectedGlobalCachedSecondValue)
}

func TestRunCmdNoCacheSkipsCachedValue(t *testing.T) {
	t.Parallel()

	result := runCmdFlagsFixture(t)

	assert.Contains(t, result.stderr, "run_cmd output: ["+expectedNoCacheFirstValue+"]")
	assert.NotContains(t, result.stderr, "run_cmd, cached output: ["+expectedNoCacheFirstValue+"]")
}

func TestRunCmdConflictingCacheOptionsFails(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRunCmdModuleConflict)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRunCmdFlags)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRunCmdFlags)

	cmd := "terragrunt run --all plan --non-interactive --log-level debug --working-dir " + rootPath

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.Error(t, err)
	assert.Contains(t, stderr, "--terragrunt-global-cache and --terragrunt-no-cache options cannot be used together")
}
