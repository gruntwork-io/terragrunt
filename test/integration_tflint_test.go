//go:build tflint
// +build tflint

//nolint:paralleltest
package integration_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: These tests intentionally don't use t.Parallel() because they are testing the behavior of tflint, which
// doesn't support parallel runs.

func TestTflintFindsNoIssuesWithValidCode(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, TestFixtureTflintNoIssuesFound)
	modulePath := util.JoinPath(rootPath, TestFixtureTflintNoIssuesFound)
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir "+modulePath, out, errOut)
	require.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")

	found, err := regexp.MatchString(fmt.Sprintf("--config %s/.terragrunt-cache/.*/.tflint.hcl", modulePath), errOut.String())
	require.NoError(t, err)
	assert.True(t, found)
}

func TestTflintFindsModule(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, TestFixtureTflintModuleFound)
	modulePath := util.JoinPath(rootPath, TestFixtureTflintModuleFound)
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-working-dir "+modulePath, out, errOut)
	require.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")
}

func TestTflintFindsIssuesWithInvalidInput(t *testing.T) {
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, TestFixtureTflintIssuesFound)
	modulePath := util.JoinPath(rootPath, TestFixtureTflintIssuesFound)
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-working-dir "+modulePath, os.Stdout, errOut)
	require.Error(t, err, "Tflint found issues in the project. Check for the tflint logs")
}

func TestTflintWithoutConfigFile(t *testing.T) {
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, TestFixtureTflintNoConfigFile)
	modulePath := util.JoinPath(rootPath, TestFixtureTflintNoConfigFile)
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-working-dir "+modulePath, io.Discard, errOut)
	require.Error(t, err, "Could not find .tflint.hcl config file in the parent folders:")
}

func TestTflintFindsConfigInCurrentPath(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, TestFixtureTflintNoTfSourcePath)
	modulePath := util.JoinPath(rootPath, TestFixtureTflintNoTfSourcePath)
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir "+modulePath, out, errOut)
	require.NoError(t, err)

	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
	assert.Contains(t, errOut.String(), fmt.Sprintf("--config %s/.tflint.hcl", modulePath))
}

func TestTflintInitSameModule(t *testing.T) {
	rootPath := copyEnvironmentWithTflint(t, TestFixtureParallelRun)
	t.Cleanup(func() {
		removeFolder(t, rootPath)
	})
	modulePath := util.JoinPath(rootPath, TestFixtureParallelRun)
	runPath := util.JoinPath(rootPath, TestFixtureParallelRun, "dev")
	appTemplate := util.JoinPath(rootPath, TestFixtureParallelRun, "dev", "app")
	// generate multiple "app" modules that will be initialized in parallel
	for i := 0; i < 50; i++ {
		appPath := util.JoinPath(modulePath, "dev", fmt.Sprintf("app-%d", i))
		err := util.CopyFolderContents(appTemplate, appPath, ".terragrunt-test", []string{})
		require.NoError(t, err)
	}
	runTerragrunt(t, "terragrunt run-all init --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir "+runPath)
}

func TestTflintFindsNoIssuesWithValidCodeDifferentDownloadDir(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	downloadDir, err := os.MkdirTemp("", "download-dir")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	rootPath := copyEnvironmentWithTflint(t, TestFixtureTflintNoIssuesFound)
	modulePath := util.JoinPath(rootPath, TestFixtureTflintNoIssuesFound)
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-download-dir %s", modulePath, downloadDir), out, errOut)
	require.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")
	found, err := regexp.MatchString(fmt.Sprintf("--config %s/.*/.tflint.hcl", downloadDir), errOut.String())
	require.NoError(t, err)
	assert.True(t, found)
}

func TestExternalTflint(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	rootPath := copyEnvironmentWithTflint(t, TestFixtureTflintExternalTflint)
	t.Cleanup(func() {
		removeFolder(t, rootPath)
	})
	runPath := util.JoinPath(rootPath, TestFixtureTflintExternalTflint)
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir "+runPath, out, errOut)
	require.NoError(t, err)

	assert.Contains(t, errOut.String(), "Running external tflint with args")
	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
}

func TestTfvarsArePassedToTflint(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	rootPath := copyEnvironmentWithTflint(t, TestFixtureTflintTfvarPassing)
	t.Cleanup(func() {
		removeFolder(t, rootPath)
	})
	runPath := util.JoinPath(rootPath, TestFixtureTflintTfvarPassing)
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir "+runPath, out, errOut)
	require.NoError(t, err)

	assert.Contains(t, errOut.String(), "--var-file=extra.tfvars")
	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
}

func TestTflintArgumentsPassedIn(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	rootPath := copyEnvironmentWithTflint(t, TestFixtureTflintArgs)
	t.Cleanup(func() {
		removeFolder(t, rootPath)
	})
	runPath := util.JoinPath(rootPath, TestFixtureTflintArgs)
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir "+runPath, out, errOut)
	require.NoError(t, err)

	assert.Contains(t, errOut.String(), "--minimum-failure-severity=error")
	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
}

func TestTflintCustomConfig(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	rootPath := copyEnvironmentWithTflint(t, TestFixtureTflintCustomConfig)
	t.Cleanup(func() {
		removeFolder(t, rootPath)
	})
	runPath := util.JoinPath(rootPath, TestFixtureTflintCustomConfig)
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir "+runPath, out, errOut)
	require.NoError(t, err)

	assert.Contains(t, errOut.String(), "--config custom.tflint.hcl")
	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
}

func copyEnvironmentWithTflint(t *testing.T, environmentPath string) string {
	tmpDir, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	t.Logf("Copying %s to %s", environmentPath, tmpDir)

	require.NoError(t, util.CopyFolderContents(environmentPath, util.JoinPath(tmpDir, environmentPath), ".terragrunt-test", []string{".tflint.hcl"}))

	return tmpDir
}
