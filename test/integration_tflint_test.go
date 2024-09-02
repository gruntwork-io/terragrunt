//go:build tflint
// +build tflint

package test_test

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

const (
	testFixtureTflintNoIssuesFound  = "fixtures/fixture-tflint/no-issues-found"
	testFixtureTflintIssuesFound    = "fixtures/fixture-tflint/issues-found"
	testFixtureTflintNoConfigFile   = "fixtures/fixture-tflint/no-config-file"
	testFixtureTflintModuleFound    = "fixtures/fixture-tflint/module-found"
	testFixtureTflintNoTfSourcePath = "fixtures/fixture-tflint/no-tf-source"
	testFixtureTflintExternalTflint = "fixtures/fixture-tflint/external-tflint"
	testFixtureTflintTfvarPassing   = "fixtures/fixture-tflint/tfvar-passing"
	testFixtureTflintArgs           = "fixtures/fixture-tflint/tflint-args"
	testFixtureTflintCustomConfig   = "fixtures/fixture-tflint/custom-tflint-config"
)

func TestTflintFindsNoIssuesWithValidCode(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, testFixtureTflintNoIssuesFound)
	modulePath := util.JoinPath(rootPath, testFixtureTflintNoIssuesFound)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir %s", modulePath), out, errOut)
	assert.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")

	found, err := regexp.MatchString(fmt.Sprintf("--config %s/.terragrunt-cache/.*/.tflint.hcl", modulePath), errOut.String())
	assert.NoError(t, err)
	assert.True(t, found)
}

func TestTflintFindsModule(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, testFixtureTflintModuleFound)
	modulePath := util.JoinPath(rootPath, testFixtureTflintModuleFound)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-working-dir %s", modulePath), out, errOut)
	assert.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")
}

func TestTflintFindsIssuesWithInvalidInput(t *testing.T) {
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, testFixtureTflintIssuesFound)
	modulePath := util.JoinPath(rootPath, testFixtureTflintIssuesFound)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-working-dir %s", modulePath), os.Stdout, errOut)
	assert.Error(t, err, "Tflint found issues in the project. Check for the tflint logs")
}

func TestTflintWithoutConfigFile(t *testing.T) {
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, testFixtureTflintNoConfigFile)
	modulePath := util.JoinPath(rootPath, testFixtureTflintNoConfigFile)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-working-dir %s", modulePath), io.Discard, errOut)
	assert.Error(t, err, "Could not find .tflint.hcl config file in the parent folders:")
}

func TestTflintFindsConfigInCurrentPath(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, testFixtureTflintNoTfSourcePath)
	modulePath := util.JoinPath(rootPath, testFixtureTflintNoTfSourcePath)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir %s", modulePath), out, errOut)
	assert.NoError(t, err)

	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
	assert.Contains(t, errOut.String(), fmt.Sprintf("--config %s/.tflint.hcl", modulePath))
}

func TestTflintInitSameModule(t *testing.T) {
	rootPath := copyEnvironmentWithTflint(t, testFixtureParallelRun)
	t.Cleanup(func() {
		removeFolder(t, rootPath)
	})
	modulePath := util.JoinPath(rootPath, testFixtureParallelRun)
	runPath := util.JoinPath(rootPath, testFixtureParallelRun, "dev")
	appTemplate := util.JoinPath(rootPath, testFixtureParallelRun, "dev", "app")
	// generate multiple "app" modules that will be initialized in parallel
	for i := 0; i < 50; i++ {
		appPath := util.JoinPath(modulePath, "dev", fmt.Sprintf("app-%d", i))
		err := util.CopyFolderContents(appTemplate, appPath, ".terragrunt-test", []string{})
		assert.NoError(t, err)
	}
	runTerragrunt(t, fmt.Sprintf("terragrunt run-all init --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", runPath))
}

func TestTflintFindsNoIssuesWithValidCodeDifferentDownloadDir(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	downloadDir, err := os.MkdirTemp("", "download-dir")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	rootPath := copyEnvironmentWithTflint(t, testFixtureTflintNoIssuesFound)
	modulePath := util.JoinPath(rootPath, testFixtureTflintNoIssuesFound)
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-download-dir %s", modulePath, downloadDir), out, errOut)
	assert.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")
	found, err := regexp.MatchString(fmt.Sprintf("--config %s/.*/.tflint.hcl", downloadDir), errOut.String())
	assert.NoError(t, err)
	assert.True(t, found)
}

func TestExternalTflint(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	rootPath := copyEnvironmentWithTflint(t, testFixtureTflintExternalTflint)
	t.Cleanup(func() {
		removeFolder(t, rootPath)
	})
	runPath := util.JoinPath(rootPath, testFixtureTflintExternalTflint)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir %s", runPath), out, errOut)
	assert.NoError(t, err)

	assert.Contains(t, errOut.String(), "Running external tflint with args")
	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
}

func TestTfvarsArePassedToTflint(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	rootPath := copyEnvironmentWithTflint(t, testFixtureTflintTfvarPassing)
	t.Cleanup(func() {
		removeFolder(t, rootPath)
	})
	runPath := util.JoinPath(rootPath, testFixtureTflintTfvarPassing)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir %s", runPath), out, errOut)
	assert.NoError(t, err)

	assert.Contains(t, errOut.String(), "--var-file=extra.tfvars")
	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
}

func TestTflintArgumentsPassedIn(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	rootPath := copyEnvironmentWithTflint(t, testFixtureTflintArgs)
	t.Cleanup(func() {
		removeFolder(t, rootPath)
	})
	runPath := util.JoinPath(rootPath, testFixtureTflintArgs)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir %s", runPath), out, errOut)
	assert.NoError(t, err)

	assert.Contains(t, errOut.String(), "--minimum-failure-severity=error")
	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
}

func TestTflintCustomConfig(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	rootPath := copyEnvironmentWithTflint(t, testFixtureTflintCustomConfig)
	t.Cleanup(func() {
		removeFolder(t, rootPath)
	})
	runPath := util.JoinPath(rootPath, testFixtureTflintCustomConfig)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir %s", runPath), out, errOut)
	assert.NoError(t, err)

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
