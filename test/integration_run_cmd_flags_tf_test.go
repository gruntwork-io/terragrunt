//go:build tf

package test_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureRunCmdFlags            = "fixtures/run-cmd-flags"
	testFixtureRunCmdModuleQuiet      = "fixtures/run-cmd-flags/module-quiet"
	testFixtureRunCmdModuleGlobalA    = "fixtures/run-cmd-flags/module-global-cache-a"
	testFixtureRunCmdModuleGlobalB    = "fixtures/run-cmd-flags/module-global-cache-b"
	testFixtureRunCmdModuleNoCache    = "fixtures/run-cmd-flags/module-no-cache"
	testFixtureRunCmdModuleConflict   = "fixtures/run-cmd-flags/module-conflict"
	testFixtureRunCmdSharedFile       = "fixtures/run-cmd-shared-file"
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
	rootPath := filepath.Join(tmpEnvPath, testFixtureRunCmdFlags)

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

func TestTFRunCmdQuietRedactsOutput(t *testing.T) {
	t.Parallel()

	result := runCmdFlagsFixture(t)

	assert.Contains(t, result.stderr, "run_cmd output: [REDACTED]")
	assert.NotContains(t, result.stderr, runCmdSecretValue)
}

func TestTFRunCmdGlobalCacheSharesResultAcrossModules(t *testing.T) {
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

func TestTFRunCmdNoCacheSkipsCachedValue(t *testing.T) {
	t.Parallel()

	result := runCmdFlagsFixture(t)

	assert.Contains(t, result.stderr, "run_cmd output: ["+expectedNoCacheFirstValue+"]")
	assert.NotContains(t, result.stderr, "run_cmd, cached output: ["+expectedNoCacheFirstValue+"]")
}

// TestTFRunCmdSharedFileKeysOnUnitEnvAndArgv pins the run_cmd cache key across
// a `run --all`. Two units read one shared file, so its run_cmd calls evaluate
// under one directory, while --auth-provider-cmd hands each unit its own
// credentials. Each unit must see the value its own environment produced, and
// two commands whose argv differ only in element boundaries must not share one
// cache entry.
func TestTFRunCmdSharedFileKeysOnUnitEnvAndArgv(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRunCmdSharedFile)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRunCmdSharedFile)
	rootPath := filepath.Join(tmpEnvPath, testFixtureRunCmdSharedFile)
	mockAuthCmd := filepath.Join(rootPath, "mock-auth-cmd.sh")

	helpers.ValidateAuthProviderScript(t, venv.OSVenv(), rootPath, mockAuthCmd)

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --all --non-interactive --working-dir %s --auth-provider-cmd %s -- apply -auto-approve",
			rootPath,
			mockAuthCmd,
		),
	)

	for _, unit := range []string{"unit-a", "unit-b"} {
		stdout, _, err := helpers.RunTerragruntCommandWithOutput(
			t,
			fmt.Sprintf(
				"terragrunt run --non-interactive --working-dir %s --auth-provider-cmd %s -- output -json",
				filepath.Join(rootPath, unit),
				mockAuthCmd,
			),
		)
		require.NoError(t, err)

		outputs := map[string]helpers.TerraformOutput{}
		require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

		assert.Equal(t, "acct-"+unit, outputs["account"].Value, "%s must get the value its own credentials produced", unit)
		assert.Equal(t, "x y|", outputs["joined"].Value)
		assert.Equal(t, "x|y|", outputs["split"].Value, "argv element boundaries must be part of the cache key")
	}
}
