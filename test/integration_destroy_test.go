package test_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/hashicorp/go-multierror"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureDestroyOrder           = "fixtures/fixture-destroy-order"
	testFixturePreventDestroyOverride = "fixtures/fixture-prevent-destroy-override/child"
	testFixturePreventDestroyNotSet   = "fixtures/fixture-prevent-destroy-not-set/child"
	testFixtureDestroyWarning         = "fixtures/fixture-destroy-warning"
	testFixtureDestroyDependentModule = "fixtures/fixture-destroy-dependent-module"
)

func TestTerragruntDestroyOrder(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureDestroyOrder)
	tmpEnvPath := copyEnvironment(t, testFixtureDestroyOrder)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDestroyOrder, "app")

	runTerragrunt(t, "terragrunt run-all apply --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt run-all destroy --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`(?smi)(?:(Module E|Module D|Module B).*){3}(?:(Module A|Module C).*){2}`), stdout)
}

func TestTerragruntApplyDestroyOrder(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureDestroyOrder)
	tmpEnvPath := copyEnvironment(t, testFixtureDestroyOrder)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDestroyOrder, "app")

	runTerragrunt(t, "terragrunt run-all apply --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt run-all apply -destroy --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`(?smi)(?:(Module E|Module D|Module B).*){3}(?:(Module A|Module C).*){2}`), stdout)
}

func TestPreventDestroyOverride(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixturePreventDestroyOverride)

	require.NoError(t, runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-working-dir "+testFixturePreventDestroyOverride, os.Stdout, os.Stderr))
	require.NoError(t, runTerragruntCommand(t, "terragrunt destroy -auto-approve --terragrunt-working-dir "+testFixturePreventDestroyOverride, os.Stdout, os.Stderr))
}

func TestPreventDestroyNotSet(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixturePreventDestroyNotSet)

	require.NoError(t, runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-working-dir "+testFixturePreventDestroyNotSet, os.Stdout, os.Stderr))
	err := runTerragruntCommand(t, "terragrunt destroy -auto-approve --terragrunt-working-dir "+testFixturePreventDestroyNotSet, os.Stdout, os.Stderr)

	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, terraform.ModuleIsProtected{}, underlying)
	}
}

func TestDestroyDependentModule(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureDestroyDependentModule)
	tmpEnvPath, _ := filepath.EvalSymlinks(copyEnvironment(t, testFixtureDestroyDependentModule))
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDestroyDependentModule)

	commandOutput, err := exec.Command("git", "init", rootPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(commandOutput))
	}
	// apply each module in order
	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+util.JoinPath(rootPath, "a"))
	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+util.JoinPath(rootPath, "b"))
	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+util.JoinPath(rootPath, "c"))

	config.ClearOutputCache()

	// destroy module which have outputs from other modules
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	workingDir := util.JoinPath(rootPath, "c")
	err = runTerragruntCommand(t, "terragrunt destroy -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir "+workingDir, &stdout, &stderr)
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

	rootPath := copyEnvironment(t, testFixtureDestroyWarning)

	rootPath = util.JoinPath(rootPath, testFixtureDestroyWarning)
	vpcPath := util.JoinPath(rootPath, "vpc")
	appV1Path := util.JoinPath(rootPath, "app-v1")
	appV2Path := util.JoinPath(rootPath, "app-v2")

	cleanupTerraformFolder(t, rootPath)
	cleanupTerraformFolder(t, vpcPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt run-all init --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)
	err = runTerragruntCommand(t, "terragrunt run-all apply --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)

	// try to destroy vpc module and check if warning is printed in output
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = runTerragruntCommand(t, "terragrunt destroy --terragrunt-non-interactive --terragrunt-working-dir "+vpcPath, &stdout, &stderr)
	require.NoError(t, err)

	output := stderr.String()
	assert.Equal(t, 1, strings.Count(output, appV1Path))
	assert.Equal(t, 1, strings.Count(output, appV2Path))
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
	cleanupTerraformFolder(t, testFixtureLocalIncludePreventDestroyDependencies)
	for _, modulePath := range modulePaths {
		cleanupTerraformFolder(t, modulePath)
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	// Apply and destroy all modules.
	err := runTerragruntCommand(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalIncludePreventDestroyDependencies, &applyAllStdout, &applyAllStderr)
	logBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
	logBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")

	if err != nil {
		t.Fatalf("apply-all in TestPreventDestroyDependenciesIncludedConfig failed with error: %v. Full std", err)
	}

	var (
		destroyAllStdout bytes.Buffer
		destroyAllStderr bytes.Buffer
	)

	err = runTerragruntCommand(t, "terragrunt destroy-all --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalIncludePreventDestroyDependencies, &destroyAllStdout, &destroyAllStderr)
	logBufferContentsLineByLine(t, destroyAllStdout, "destroy-all stdout")
	logBufferContentsLineByLine(t, destroyAllStderr, "destroy-all stderr")

	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, &multierror.Error{}, underlying)
	}

	// Check that modules C, D and E were deleted and modules A and B weren't.
	for moduleName, modulePath := range modulePaths {
		var (
			showStdout bytes.Buffer
			showStderr bytes.Buffer
		)

		err = runTerragruntCommand(t, "terragrunt show --terragrunt-non-interactive --terragrunt-working-dir "+modulePath, &showStdout, &showStderr)
		logBufferContentsLineByLine(t, showStdout, "show stdout for "+modulePath)
		logBufferContentsLineByLine(t, showStderr, "show stderr for "+modulePath)

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
	if envProviderCache := os.Getenv(commands.TerragruntProviderCacheEnvName); envProviderCache != "" {
		providerCache, err := strconv.ParseBool(envProviderCache)
		require.NoError(t, err)
		if providerCache {
			return
		}
	}

	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureExternalDependency)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureExternalDependency)

	t.Cleanup(func() {
		os.RemoveAll(filepath.ToSlash("/tmp/external-46521694"))
	})
	require.NoError(t, os.Mkdir(filepath.ToSlash("/tmp/external-46521694"), 0755))

	output, err := exec.Command("git", "init", tmpEnvPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(output))
	}

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	r, w, _ := os.Pipe()
	oldStdout := os.Stderr
	os.Stderr = w

	err = runTerragruntCommand(t, "terragrunt destroy --terragrunt-working-dir "+testPath, &stdout, &stderr)
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
	assert.NotContains(t, captured, "/tmp/external1")
}

func TestStorePlanFilesRunAllDestroy(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpEnvPath := copyEnvironment(t, testFixtureOutDir)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

	// plan and apply
	_, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	_, _, err = runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all apply --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	// remove all tfstate files from temp directory to prepare destroy
	list, err := findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	// prepare destroy plan
	_, output, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all plan -destroy --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	assert.Contains(t, output, "Using output file "+tmpDir)
	// verify that tfplan files are created in the tmpDir, 2 files
	list, err = findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	_, _, err = runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all apply --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-out-dir %s", testPath, tmpDir))
	require.NoError(t, err)
}
