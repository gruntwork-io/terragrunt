package test_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"

	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureDestroyOrder           = "fixtures/destroy-order"
	testFixturePreventDestroyOverride = "fixtures/prevent-destroy-override/child"
	testFixturePreventDestroyNotSet   = "fixtures/prevent-destroy-not-set/child"
	testFixtureDestroyWarning         = "fixtures/destroy-warning"
	testFixtureDestroyDependentModule = "fixtures/destroy-dependent-module"
)

func TestTerragruntDestroyOrder(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDestroyOrder)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDestroyOrder)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDestroyOrder, "app")

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all destroy --non-interactive --tf-forward-stdout --working-dir "+rootPath)
	require.NoError(t, err)
	assert.Regexp(t, `(?smi)(?:(Module E|Module D|Module B).*){3}(?:(Module A|Module C).*){2}`, stdout)
}

func TestTerragruntApplyDestroyOrder(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDestroyOrder)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDestroyOrder)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDestroyOrder, "app")

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --tf-forward-stdout --working-dir "+rootPath+" -- apply -destroy")
	require.NoError(t, err)
	assert.Regexp(t, `(?smi)(?:(Module E|Module D|Module B).*){3}(?:(Module A|Module C).*){2}`, stdout)
}

func TestPreventDestroyOverride(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixturePreventDestroyOverride)

	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --working-dir "+testFixturePreventDestroyOverride, os.Stdout, os.Stderr))
	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt destroy -auto-approve --working-dir "+testFixturePreventDestroyOverride, os.Stdout, os.Stderr))
}

func TestPreventDestroyNotSet(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixturePreventDestroyNotSet)

	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --working-dir "+testFixturePreventDestroyNotSet, os.Stdout, os.Stderr))
	err := helpers.RunTerragruntCommand(t, "terragrunt destroy -auto-approve --working-dir "+testFixturePreventDestroyNotSet, os.Stdout, os.Stderr)

	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, run.ModuleIsProtected{}, underlying)
	}
}

func TestDestroyDependentModule(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDestroyDependentModule)
	tmpEnvPath, _ := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureDestroyDependentModule))
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDestroyDependentModule)

	helpers.CreateGitRepo(t, rootPath)
	// apply each module in order
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+util.JoinPath(rootPath, "a"))
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+util.JoinPath(rootPath, "b"))
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+util.JoinPath(rootPath, "c"))

	config.ClearOutputCache()

	// destroy module which have outputs from other modules
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	workingDir := util.JoinPath(rootPath, "c")
	err := helpers.RunTerragruntCommand(t, "terragrunt destroy -auto-approve --non-interactive --log-level trace --working-dir "+workingDir, &stdout, &stderr)
	require.NoError(t, err)

	output := stderr.String()

	for _, path := range []string{
		util.JoinPath(rootPath, "b", "terragrunt.hcl"),
		util.JoinPath(rootPath, "a", "terragrunt.hcl"),
	} {
		relPath, err := filepath.Rel(workingDir, path)
		require.NoError(t, err)
		assert.Contains(t, output, relPath, output)
	}

	assert.Contains(t, output, "\"value\": \"module-b.txt\"", output)
	assert.Contains(t, output, "\"value\": \"module-a.txt\"", output)
}

func TestShowWarningWithDependentModulesBeforeDestroy(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixtureDestroyWarning)

	rootPath = util.JoinPath(rootPath, testFixtureDestroyWarning)
	vpcPath := util.JoinPath(rootPath, "vpc")
	appV1Path := util.JoinPath(rootPath, "app-v1")
	appV2Path := util.JoinPath(rootPath, "app-v2")

	helpers.CleanupTerraformFolder(t, rootPath)
	helpers.CleanupTerraformFolder(t, vpcPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt run --all init --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)
	err = helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)

	// try to destroy vpc module and check if warning is printed in output
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = helpers.RunTerragruntCommand(t, "terragrunt destroy --non-interactive --working-dir "+vpcPath, &stdout, &stderr)
	require.NoError(t, err)

	output := stderr.String()
	assert.Equal(t, 1, strings.Count(output, appV1Path))
	assert.Equal(t, 1, strings.Count(output, appV2Path))
}

func TestNoShowWarningWithDependentModulesBeforeDestroy(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixtureDestroyWarning)

	rootPath = util.JoinPath(rootPath, testFixtureDestroyWarning)
	vpcPath := util.JoinPath(rootPath, "vpc")
	appV1Path := util.JoinPath(rootPath, "app-v1")
	appV2Path := util.JoinPath(rootPath, "app-v2")

	cleanupTerraformFolder(t, rootPath)
	cleanupTerraformFolder(t, vpcPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt run --all init --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)
	err = helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)

	// try to destroy vpc module and check if warning is not printed in output
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = helpers.RunTerragruntCommand(t, "terragrunt destroy --non-interactive --no-destroy-dependencies-check --working-dir "+vpcPath, &stdout, &stderr)
	require.NoError(t, err)

	output := stderr.String()
	assert.Equal(t, 0, strings.Count(output, appV1Path))
	assert.Equal(t, 0, strings.Count(output, appV2Path))
}

func TestPreventDestroyDependenciesIncludedConfig(t *testing.T) {
	t.Parallel()

	// Populate module paths.
	moduleNames := []string{
		"module-a",
		"module-b",
		"module-c",
	}
	modulePaths := make(map[string]string, len(moduleNames))
	for _, moduleName := range moduleNames {
		modulePaths[moduleName] = util.JoinPath(testFixtureLocalIncludePreventDestroyDependencies, moduleName)
	}

	// Cleanup all modules directories.
	helpers.CleanupTerraformFolder(t, testFixtureLocalIncludePreventDestroyDependencies)
	for _, modulePath := range modulePaths {
		helpers.CleanupTerraformFolder(t, modulePath)
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	// Apply and destroy all modules.
	err := helpers.RunTerragruntCommand(t, "terragrunt apply-all --non-interactive --working-dir "+testFixtureLocalIncludePreventDestroyDependencies, &applyAllStdout, &applyAllStderr)
	helpers.LogBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
	helpers.LogBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")

	if err != nil {
		t.Fatalf("apply-all in TestPreventDestroyDependenciesIncludedConfig failed with error: %v. Full std", err)
	}

	var (
		destroyAllStdout bytes.Buffer
		destroyAllStderr bytes.Buffer
	)

	err = helpers.RunTerragruntCommand(t, "terragrunt destroy-all --non-interactive --working-dir "+testFixtureLocalIncludePreventDestroyDependencies, &destroyAllStdout, &destroyAllStderr)
	helpers.LogBufferContentsLineByLine(t, destroyAllStdout, "destroy-all stdout")
	helpers.LogBufferContentsLineByLine(t, destroyAllStderr, "destroy-all stderr")

	require.NoError(t, err)

	// Check that modules C, D and E were deleted and modules A and B weren't.
	for moduleName, modulePath := range modulePaths {
		var (
			showStdout bytes.Buffer
			showStderr bytes.Buffer
		)

		err = helpers.RunTerragruntCommand(t, "terragrunt show --non-interactive --tf-forward-stdout --working-dir "+modulePath, &showStdout, &showStderr)
		helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout for "+modulePath)
		helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr for "+modulePath)

		require.NoError(t, err)
		output := showStdout.String()
		switch moduleName {
		case "module-a":
			assert.Contains(t, output, "Hello, Module A")
		case "module-b":
			assert.Contains(t, output, "Hello, Module B")
		case "module-c":
			assert.NotContains(t, output, "Hello, Module C")
		}
	}
}

func TestTerragruntSkipConfirmExternalDependencies(t *testing.T) {
	// This test cannot be run using Terragrunt Provider Cache because it causes the flock files to be locked forever, which in turn blocks other TGs (processes).
	// We use flock files to prevent multiple TGs from caching the same provider in parallel in a shared cache, which causes to conflicts.
	if helpers.IsTerragruntProviderCacheEnabled(t) {
		t.Skip()
	}

	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExternalDependency)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureExternalDependency)

	helpers.CreateGitRepo(t, tmpEnvPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	r, w, _ := os.Pipe()
	oldStdout := os.Stderr
	os.Stderr = w

	tmp := t.TempDir()

	err := helpers.RunTerragruntCommand(
		t,
		fmt.Sprintf(
			"terragrunt destroy --feature dep=%s --working-dir %s",
			tmp,
			testPath,
		),
		&stdout,
		&stderr,
	)
	os.Stderr = oldStdout
	require.NoError(t, w.Close())

	capturedOutput := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, e := io.Copy(&buf, r)
		assert.NoError(t, e)
		capturedOutput <- buf.String()
	}()

	captured := <-capturedOutput

	require.NoError(t, err)
	assert.NotContains(t, captured, "Should Terragrunt apply the external dependency?")
	assert.NotContains(t, captured, tmp)
}

func TestStorePlanFilesRunAllDestroy(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

	dependencyPath := util.JoinPath(tmpEnvPath, testFixtureOutDir, "dependency")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --log-level trace --working-dir %s --out-dir %s", dependencyPath, tmpDir))

	// plan and apply
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all plan --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all apply --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	// remove all tfstate files from temp directory to prepare destroy
	list, err := findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	// prepare destroy plan
	_, output, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all --non-interactive --log-level trace --working-dir %s --out-dir %s -- plan -destroy", testPath, tmpDir))
	require.NoError(t, err)

	assert.Contains(t, output, "Using output file "+getPathRelativeTo(t, tmpDir, testPath))
	// verify that tfplan files are created in the tmpDir, 2 files
	list, err = findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all apply --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, tmpDir))
	require.NoError(t, err)
}

func TestStorePlanFilesShortcutAllDestroy(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

	dependencyPath := util.JoinPath(tmpEnvPath, testFixtureOutDir, "dependency")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --log-level trace --working-dir %s --out-dir %s", dependencyPath, tmpDir))

	// plan and apply
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt plan --all --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply --all --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	// remove all tfstate files from temp directory to prepare destroy
	list, err := findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	// prepare destroy plan
	_, output, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt plan --all -destroy --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	assert.Contains(t, output, "Using output file "+getPathRelativeTo(t, tmpDir, testPath))
	// verify that tfplan files are created in the tmpDir, 2 files
	list, err = findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply --all --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, tmpDir))
	require.NoError(t, err)
}
