//go:build tflint
// +build tflint

//nolint:paralleltest
package test_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureTflintNoIssuesFound  = "fixtures/tflint/no-issues-found"
	testFixtureTflintIssuesFound    = "fixtures/tflint/issues-found"
	testFixtureTflintNoConfigFile   = "fixtures/tflint/no-config-file"
	testFixtureTflintModuleFound    = "fixtures/tflint/module-found"
	testFixtureTflintNoTfSourcePath = "fixtures/tflint/no-tf-source"
	testFixtureTflintExternalTflint = "fixtures/tflint/external-tflint"
	testFixtureTflintTfvarPassing   = "fixtures/tflint/tfvar-passing"
	testFixtureTflintArgs           = "fixtures/tflint/tflint-args"
	testFixtureTflintCustomConfig   = "fixtures/tflint/custom-tflint-config"

	// Number of init samples to detect tflint race conditions
	tflintInitSamples = 25
)

func TestTflintFindsNoIssuesWithValidCode(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := CopyEnvironmentWithTflint(t, testFixtureTflintNoIssuesFound)
	modulePath := filepath.Join(rootPath, testFixtureTflintNoIssuesFound)
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --log-level trace --working-dir "+modulePath, out, errOut)
	require.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")

	found, err := regexp.MatchString("--config ./.terragrunt-cache/.*/.tflint.hcl", errOut.String())
	require.NoError(t, err)
	assert.True(t, found)
}

func TestTflintFindsModule(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := CopyEnvironmentWithTflint(t, testFixtureTflintModuleFound)
	modulePath := filepath.Join(rootPath, testFixtureTflintModuleFound)
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --working-dir "+modulePath, out, errOut)
	require.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")
}

func TestTflintFindsIssuesWithInvalidInput(t *testing.T) {
	errOut := new(bytes.Buffer)
	rootPath := CopyEnvironmentWithTflint(t, testFixtureTflintIssuesFound)
	modulePath := filepath.Join(rootPath, testFixtureTflintIssuesFound)
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --working-dir "+modulePath, os.Stdout, errOut)
	assert.Error(t, err, "Tflint found issues in the project. Check for the tflint logs")
}

func TestTflintWithoutConfigFile(t *testing.T) {
	errOut := new(bytes.Buffer)
	rootPath := CopyEnvironmentWithTflint(t, testFixtureTflintNoConfigFile)
	modulePath := filepath.Join(rootPath, testFixtureTflintNoConfigFile)
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --working-dir "+modulePath, io.Discard, errOut)
	assert.Error(t, err, "Could not find .tflint.hcl config file in the parent folders:")
}

func TestTflintFindsConfigInCurrentPath(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := CopyEnvironmentWithTflint(t, testFixtureTflintNoTfSourcePath)
	modulePath := filepath.Join(rootPath, testFixtureTflintNoTfSourcePath)
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --log-level trace --working-dir "+modulePath, out, errOut)
	require.NoError(t, err)

	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
	assert.Contains(t, errOut.String(), "--config ./.tflint.hcl")
}

func TestTflintInitSameModule(t *testing.T) {
	rootPath := CopyEnvironmentWithTflint(t, testFixtureParallelRun)
	t.Cleanup(func() {
		helpers.RemoveFolder(t, rootPath)
	})
	modulePath := filepath.Join(rootPath, testFixtureParallelRun)
	runPath := filepath.Join(rootPath, testFixtureParallelRun, "dev")
	appTemplate := filepath.Join(rootPath, testFixtureParallelRun, "dev", "app")
	// generate multiple "app" modules that will be initialized in parallel
	for i := 0; i < tflintInitSamples; i++ {
		appPath := filepath.Join(modulePath, "dev", fmt.Sprintf("app-%d", i))
		err := util.CopyFolderContents(createLogger(), appTemplate, appPath, ".terragrunt-test", []string{}, []string{})
		require.NoError(t, err)
	}
	helpers.RunTerragrunt(t, "terragrunt run --all init --log-level trace --non-interactive --working-dir "+runPath)
}

func TestTflintFindsNoIssuesWithValidCodeDifferentDownloadDir(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	downloadDir := helpers.TmpDirWOSymlinks(t)

	rootPath := CopyEnvironmentWithTflint(t, testFixtureTflintNoIssuesFound)
	t.Cleanup(func() {
		helpers.RemoveFolder(t, rootPath)
	})
	modulePath := filepath.Join(rootPath, testFixtureTflintNoIssuesFound)
	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt plan --log-level trace --working-dir %s --download-dir %s", modulePath, downloadDir), out, errOut)
	require.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")

	relPath, err := filepath.Rel(modulePath, downloadDir)
	require.NoError(t, err)

	found, err := regexp.MatchString(fmt.Sprintf("--config %s/.*/.tflint.hcl", relPath), errOut.String())
	require.NoError(t, err)
	assert.True(t, found)
}

func TestTflintExternalTflint(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	rootPath := CopyEnvironmentWithTflint(t, testFixtureTflintExternalTflint)
	t.Cleanup(func() {
		helpers.RemoveFolder(t, rootPath)
	})
	runPath := filepath.Join(rootPath, testFixtureTflintExternalTflint)
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --log-level trace --working-dir "+runPath, out, errOut)
	require.NoError(t, err)

	assert.Contains(t, errOut.String(), "Running external tflint with args")
	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
}

func TestTflintTfvarsArePassedToTflint(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	rootPath := CopyEnvironmentWithTflint(t, testFixtureTflintTfvarPassing)
	t.Cleanup(func() {
		helpers.RemoveFolder(t, rootPath)
	})
	runPath := filepath.Join(rootPath, testFixtureTflintTfvarPassing)
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --log-level trace --working-dir "+runPath, out, errOut)
	require.NoError(t, err)

	assert.Contains(t, errOut.String(), "--var-file=extra.tfvars")
	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
}

func TestTflintArgumentsPassedIn(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	rootPath := CopyEnvironmentWithTflint(t, testFixtureTflintArgs)
	t.Cleanup(func() {
		helpers.RemoveFolder(t, rootPath)
	})
	runPath := filepath.Join(rootPath, testFixtureTflintArgs)
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --log-level trace --working-dir "+runPath, out, errOut)
	require.NoError(t, err)

	assert.Contains(t, errOut.String(), "--minimum-failure-severity=error")
	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
}

func TestTflintCustomConfig(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)

	rootPath := CopyEnvironmentWithTflint(t, testFixtureTflintCustomConfig)
	t.Cleanup(func() {
		helpers.RemoveFolder(t, rootPath)
	})
	runPath := filepath.Join(rootPath, testFixtureTflintCustomConfig)
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --log-level trace --working-dir "+runPath, out, errOut)
	require.NoError(t, err)

	assert.Contains(t, errOut.String(), "--config custom.tflint.hcl")
	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
}

func CopyEnvironmentWithTflint(t *testing.T, environmentPath string) string {
	t.Helper()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	t.Logf("Copying %s to %s", environmentPath, tmpDir)

	require.NoError(t, util.CopyFolderContents(createLogger(), environmentPath, filepath.Join(tmpDir, environmentPath), ".terragrunt-test", []string{".tflint.hcl"}, []string{}))

	return tmpDir
}
