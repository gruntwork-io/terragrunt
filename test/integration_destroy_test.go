package test_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureDestroyOrder                 = "fixtures/destroy-order"
	testFixturePreventDestroyOverride       = "fixtures/prevent-destroy-override/child"
	testFixturePreventDestroyNotSet         = "fixtures/prevent-destroy-not-set/child"
	testFixtureDestroyWarning               = "fixtures/destroy-warning"
	testFixtureDestroyDependentModule       = "fixtures/destroy-dependent-module"
	testFixtureDestroyDependentModuleErrors = "fixtures/destroy-dependent-module-errors"
)

func TestTerragruntDestroyOrder(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDestroyOrder)
	rootPath := filepath.Join(tmpEnvPath, testFixtureDestroyOrder, "app")
	// Resolve symlinks to avoid path mismatches on macOS where /var -> /private/var
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	// Run destroy with report file to capture run order
	reportFile := filepath.Join(rootPath, "report.json")
	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf("terragrunt run --all destroy --non-interactive --working-dir %s --report-file %s", rootPath, reportFile),
	)
	require.NoError(t, err)

	// Parse the report file to verify dependency order
	runs, err := report.ParseJSONRunsFromFile(reportFile)
	require.NoError(t, err)

	// Verify all modules are in the report
	runA := runs.FindByName("module-a")
	runB := runs.FindByName("module-b")
	runC := runs.FindByName("module-c")
	runD := runs.FindByName("module-d")
	runE := runs.FindByName("module-e")

	require.NotNil(t, runA, "module-a should be in report")
	require.NotNil(t, runB, "module-b should be in report")
	require.NotNil(t, runC, "module-c should be in report")
	require.NotNil(t, runD, "module-d should be in report")
	require.NotNil(t, runE, "module-e should be in report")

	// Module B depends on A, so B must be destroyed (start) before A
	assert.True(t, runB.Started.Before(runA.Started), "Module B should start before Module A (B depends on A)")

	// Module D depends on C, so D must be destroyed (start) before C
	assert.True(t, runD.Started.Before(runC.Started), "Module D should start before Module C (D depends on C)")
}

func TestTerragruntApplyDestroyOrder(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDestroyOrder)
	rootPath := filepath.Join(tmpEnvPath, testFixtureDestroyOrder, "app")
	// Resolve symlinks to avoid path mismatches on macOS where /var -> /private/var
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	// Run destroy with report file to capture run order
	reportFile := filepath.Join(rootPath, "report.json")
	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --no-color --all --non-interactive --tf-forward-stdout --working-dir %s --report-file %s -- apply -destroy",
			rootPath,
			reportFile,
		),
	)
	require.NoError(t, err)

	// Parse the report file to verify dependency order
	runs, err := report.ParseJSONRunsFromFile(reportFile)
	require.NoError(t, err)

	runA := runs.FindByName("module-a")
	runB := runs.FindByName("module-b")
	runC := runs.FindByName("module-c")
	runD := runs.FindByName("module-d")
	runE := runs.FindByName("module-e")

	require.NotNil(t, runA, "module-a should be in report")
	require.NotNil(t, runB, "module-b should be in report")
	require.NotNil(t, runC, "module-c should be in report")
	require.NotNil(t, runD, "module-d should be in report")
	require.NotNil(t, runE, "module-e should be in report")

	// Module B depends on A, so B must be destroyed (start) before A
	assert.True(t, runB.Started.Before(runA.Started), "Module B should be destroyed before Module A (B depends on A)")

	// Module D depends on C, so D must be destroyed (start) before C
	assert.True(t, runD.Started.Before(runC.Started), "Module D should be destroyed before Module C (D depends on C)")
}

// TestTerragruntDestroyOrderWithQueueIgnoreErrors tests that --queue-ignore-errors still respects dependency order.
// This is a regression test for issue #4947.
// Note: This test verifies the behavior is the same with and without --queue-ignore-errors for successful runs.
// The unit tests in internal/queue/queue_test.go provide comprehensive coverage of the ordering logic.
func TestTerragruntDestroyOrderWithQueueIgnoreErrors(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDestroyOrder)
	rootPath := filepath.Join(tmpEnvPath, testFixtureDestroyOrder, "app")
	// Resolve symlinks to avoid path mismatches on macOS where /var -> /private/var
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	// Run destroy with --queue-ignore-errors flag and capture results in a report file
	// The main difference with --queue-ignore-errors is that it allows progress even if dependencies fail,
	// but it should still respect the dependency order when dependencies are in terminal states.
	reportFile := filepath.Join(rootPath, "report.json")
	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf("terragrunt run --all destroy --non-interactive --queue-ignore-errors --working-dir %s --report-file %s", rootPath, reportFile),
	)
	require.NoError(t, err)

	// Parse the report file to verify dependency order
	runs, err := report.ParseJSONRunsFromFile(reportFile)
	require.NoError(t, err)

	// Verify all modules are in the report
	runA := runs.FindByName("module-a")
	runB := runs.FindByName("module-b")
	runC := runs.FindByName("module-c")
	runD := runs.FindByName("module-d")
	runE := runs.FindByName("module-e")

	require.NotNil(t, runA, "module-a should be in report")
	require.NotNil(t, runB, "module-b should be in report")
	require.NotNil(t, runC, "module-c should be in report")
	require.NotNil(t, runD, "module-d should be in report")
	require.NotNil(t, runE, "module-e should be in report")

	// Module B depends on A, so B must be destroyed (start) before A
	assert.True(t, runB.Started.Before(runA.Started), "Module B should start before Module A (B depends on A)")

	// Module D depends on C, so D must be destroyed (start) before C
	assert.True(t, runD.Started.Before(runC.Started), "Module D should start before Module C (D depends on C)")
}

func TestPreventDestroyOverride(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePreventDestroyOverride)
	rootPath := filepath.Join(tmpEnvPath, testFixturePreventDestroyOverride)

	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --working-dir "+rootPath, os.Stdout, os.Stderr))
	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt destroy -auto-approve --working-dir "+rootPath, os.Stdout, os.Stderr))
}

func TestPreventDestroyNotSet(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePreventDestroyNotSet)
	rootPath := filepath.Join(tmpEnvPath, testFixturePreventDestroyNotSet)

	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --working-dir "+rootPath, os.Stdout, os.Stderr))
	err := helpers.RunTerragruntCommand(t, "terragrunt destroy -auto-approve --working-dir "+rootPath, os.Stdout, os.Stderr)

	if assert.Error(t, err) {
		var target run.ModuleIsProtected
		assert.ErrorAs(t, err, &target)
	}
}

func TestDestroyDependentModule(t *testing.T) {
	t.Parallel()

	tmpEnvPath, _ := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureDestroyDependentModule))
	rootPath := filepath.Join(tmpEnvPath, testFixtureDestroyDependentModule)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(rootPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	// apply each module in order
	helpers.RunTerragrunt(
		t,
		"terragrunt run --working-dir "+filepath.Join(rootPath, "a")+" -- apply -auto-approve",
	)
	helpers.RunTerragrunt(
		t,
		"terragrunt run --working-dir "+filepath.Join(rootPath, "b")+" -- apply -auto-approve",
	)
	helpers.RunTerragrunt(
		t,
		"terragrunt run --working-dir "+filepath.Join(rootPath, "c")+" -- apply -auto-approve",
	)

	// destroy module which have outputs from other modules
	workingDir := filepath.Join(rootPath, "c")
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --log-level debug --working-dir "+workingDir+" -- destroy -auto-approve",
	)
	require.NoError(t, err)

	for _, path := range []string{
		filepath.Join(rootPath, "b", "terragrunt.hcl"),
		filepath.Join(rootPath, "a", "terragrunt.hcl"),
	} {
		relPath, err := filepath.Rel(workingDir, path)
		require.NoError(t, err)
		assert.Contains(t, stderr, relPath, stderr)
	}

	assert.Contains(t, stderr, "\"value\": \"module-b.txt\"", stderr)
	assert.Contains(t, stderr, "\"value\": \"module-a.txt\"", stderr)
}

func TestShowWarningWithDependentModulesBeforeDestroy(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixtureDestroyWarning)

	rootPath = filepath.Join(rootPath, testFixtureDestroyWarning)
	vpcPath := filepath.Join(rootPath, "vpc")
	appV1Path := filepath.Join(rootPath, "app-v1")
	appV2Path := filepath.Join(rootPath, "app-v2")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt run --all init --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)
	err = helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)

	// try to destroy vpc module and check if warning is printed in output
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = helpers.RunTerragruntCommand(t, "terragrunt destroy --non-interactive --destroy-dependencies-check --working-dir "+vpcPath, &stdout, &stderr)
	require.NoError(t, err)

	output := stderr.String()
	assert.Equal(t, 1, strings.Count(output, appV1Path))
	assert.Equal(t, 1, strings.Count(output, appV2Path))
}

func TestNoShowWarningWithDependentModulesBeforeDestroy(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixtureDestroyWarning)

	rootPath = filepath.Join(rootPath, testFixtureDestroyWarning)
	vpcPath := filepath.Join(rootPath, "vpc")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt run --all init --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)
	err = helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)

	// try to destroy vpc module and check if warning is not printed in output (default behavior - checks disabled)
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = helpers.RunTerragruntCommand(t, "terragrunt destroy --non-interactive --working-dir "+vpcPath, &stdout, &stderr)
	require.NoError(t, err)

	output := stderr.String()
	appV1Path := filepath.Join(rootPath, "app-v1")
	appV2Path := filepath.Join(rootPath, "app-v2")
	assert.Equal(t, 0, strings.Count(output, appV1Path))
	assert.Equal(t, 0, strings.Count(output, appV2Path))
}

func TestPreventDestroyDependenciesIncludedConfig(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLocalIncludePreventDestroyDependencies)
	rootPath := filepath.Join(tmpEnvPath, testFixtureLocalIncludePreventDestroyDependencies)

	// Populate module paths.
	moduleNames := []string{
		"module-a",
		"module-b",
		"module-c",
	}

	modulePaths := make(map[string]string, len(moduleNames))
	for _, moduleName := range moduleNames {
		modulePaths[moduleName] = filepath.Join(rootPath, moduleName)
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	// Apply and destroy all modules.
	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath, &applyAllStdout, &applyAllStderr)
	helpers.LogBufferContentsLineByLine(t, applyAllStdout, "run --all apply stdout")
	helpers.LogBufferContentsLineByLine(t, applyAllStderr, "run --all apply stderr")

	if err != nil {
		t.Fatalf("run --all apply in TestPreventDestroyDependenciesIncludedConfig failed with error: %v. Full std", err)
	}

	var (
		destroyAllStdout bytes.Buffer
		destroyAllStderr bytes.Buffer
	)

	err = helpers.RunTerragruntCommand(t, "terragrunt run --all destroy --non-interactive --working-dir "+rootPath, &destroyAllStdout, &destroyAllStderr)
	helpers.LogBufferContentsLineByLine(t, destroyAllStdout, "run --all destroy stdout")
	helpers.LogBufferContentsLineByLine(t, destroyAllStderr, "run --all destroy stderr")

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
	testPath := filepath.Join(tmpEnvPath, testFixtureExternalDependency)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpEnvPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	r, w, _ := os.Pipe()
	oldStdout := os.Stderr
	os.Stderr = w

	tmp := helpers.TmpDirWOSymlinks(t)

	err = helpers.RunTerragruntCommand(
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

	tmpDir := helpers.TmpDirWOSymlinks(t)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	testPath := filepath.Join(tmpEnvPath, testFixtureOutDir)

	dependencyPath := filepath.Join(tmpEnvPath, testFixtureOutDir, "dependency")
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --working-dir %s --out-dir %s",
			dependencyPath,
			tmpDir,
		),
	)

	// plan and apply
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all plan --non-interactive --working-dir %s --out-dir %s",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all apply --non-interactive --working-dir %s --out-dir %s",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)

	// remove all tfstate files from temp directory to prepare destroy
	list, err := findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	// prepare destroy plan
	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all --non-interactive --working-dir %s --out-dir %s -- plan -destroy",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)

	// verify that tfplan files are created in the tmpDir, 2 files
	list, err = findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all apply --non-interactive --working-dir %s --out-dir %s",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)
}

func TestStorePlanFilesShortcutAllDestroy(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	testPath := filepath.Join(tmpEnvPath, testFixtureOutDir)

	dependencyPath := filepath.Join(tmpEnvPath, testFixtureOutDir, "dependency")
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --working-dir %s --out-dir %s",
			dependencyPath,
			tmpDir,
		),
	)

	// plan and apply
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt plan --all --non-interactive --working-dir %s --out-dir %s",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt apply --all --non-interactive --working-dir %s --out-dir %s",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)

	// remove all tfstate files from temp directory to prepare destroy
	list, err := findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	// prepare destroy plan
	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt plan --all -destroy --non-interactive --working-dir %s --out-dir %s",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)

	// verify that tfplan files are created in the tmpDir, 2 files
	list, err = findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt apply --all --non-interactive --working-dir %s --out-dir %s",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)
}

func TestDestroyDependentModuleParseErrors(t *testing.T) {
	t.Parallel()

	tmpEnvPath, _ := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureDestroyDependentModuleErrors))
	rootPath := filepath.Join(tmpEnvPath, testFixtureDestroyDependentModuleErrors)

	helpers.CreateGitRepo(t, rootPath)

	// apply dev
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run -all apply --non-interactive --working-dir "+filepath.Join(rootPath, "dev"),
	)
	require.NoError(t, err)

	// try to destroy app1 to trigger dependent units scanning
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt destroy -auto-approve --non-interactive --working-dir "+filepath.Join(rootPath, "dev", "app1"),
	)
	require.NoError(t, err)

	// shouldn't contain SOPS errors which are printed during dependent units discovery
	assert.NotContains(t, stderr, "sops metadata not found")
}
