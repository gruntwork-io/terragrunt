package test_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	tfsource "github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureLocalDownloadPath                      = "fixtures/download/local"
	testFixtureCustomLockFile                         = "fixtures/download/custom-lock-file"
	testFixtureRemoteDownloadPath                     = "fixtures/download/remote"
	testFixtureInvalidRemoteDownloadPath              = "fixtures/download/remote-invalid"
	testFixtureOverrideDonwloadPath                   = "fixtures/download/override"
	testFixtureLocalRelativeDownloadPath              = "fixtures/download/local-relative"
	testFixtureRemoteRelativeDownloadPath             = "fixtures/download/remote-relative"
	testFixtureRemoteRelativeDownloadPathWithSlash    = "fixtures/download/remote-relative-with-slash"
	testFixtureLocalWithBackend                       = "fixtures/download/local-with-backend"
	testFixtureLocalWithExcludeDir                    = "fixtures/download/local-with-exclude-dir"
	testFixtureLocalWithIncludeDir                    = "fixtures/download/local-with-include-dir"
	testFixtureRemoteWithBackend                      = "fixtures/download/remote-with-backend"
	testFixtureRemoteModuleInRoot                     = "fixtures/download/remote-module-in-root"
	testFixtureLocalMissingBackend                    = "fixtures/download/local-with-missing-backend"
	testFixtureLocalWithHiddenFolder                  = "fixtures/download/local-with-hidden-folder"
	testFixtureLocalWithAllowedHidden                 = "fixtures/download/local-with-allowed-hidden"
	testFixtureLocalPreventDestroy                    = "fixtures/download/local-with-prevent-destroy"
	testFixtureLocalPreventDestroyDependencies        = "fixtures/download/local-with-prevent-destroy-dependencies"
	testFixtureLocalIncludePreventDestroyDependencies = "fixtures/download/local-include-with-prevent-destroy-dependencies"
	testFixtureNotExistingSource                      = "fixtures/download/invalid-path"
	testFixtureDisableCopyLockFilePath                = "fixtures/download/local-disable-copy-terraform-lock-file"
	testFixtureIncludeDisableCopyLockFilePath         = "fixtures/download/local-include-disable-copy-lock-file/module-b"
)

func TestLocalDownload(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalDownloadPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalDownloadPath)

	// As of Terraform 0.14.0 we should be copying the lock file from .terragrunt-cache to the working directory
	assert.FileExists(t, util.JoinPath(testFixtureLocalDownloadPath, util.TerraformLockFile))

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalDownloadPath)
}

func TestLocalDownloadDisableCopyTerraformLockFile(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDisableCopyLockFilePath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureDisableCopyLockFilePath)

	// The terraform lock file should not be copied if `copy_terraform_lock_file = false`
	assert.NoFileExists(t, util.JoinPath(testFixtureDisableCopyLockFilePath, util.TerraformLockFile))

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureDisableCopyLockFilePath)
}

func TestLocalIncludeDisableCopyTerraformLockFile(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureIncludeDisableCopyLockFilePath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureIncludeDisableCopyLockFilePath)

	// The terraform lock file should not be copied if `copy_terraform_lock_file = false`
	assert.NoFileExists(t, util.JoinPath(testFixtureIncludeDisableCopyLockFilePath, util.TerraformLockFile))

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureIncludeDisableCopyLockFilePath)
}

func TestLocalDownloadWithHiddenFolder(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalWithHiddenFolder)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalWithHiddenFolder)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalWithHiddenFolder)
}

func TestLocalDownloadWithAllowedHiddenFiles(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalWithAllowedHidden)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s/live", testFixtureLocalWithAllowedHidden))

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s/live", testFixtureLocalWithAllowedHidden))

	// Validate that the hidden file was copied
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt output -raw text --terragrunt-non-interactive --terragrunt-working-dir %s/live", testFixtureLocalWithAllowedHidden), &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "output stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "output stderr")
	require.NoError(t, err)
	assert.Equal(t, "Hello world", stdout.String())
}

func TestLocalDownloadWithRelativePath(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalRelativeDownloadPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalRelativeDownloadPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalRelativeDownloadPath)
}

func TestLocalWithMissingBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-lock-table-" + strings.ToLower(helpers.UniqueID())

	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/download")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLocalMissingBackend)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, os.Stdout, os.Stderr)
	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, terraform.BackendNotDefined{}, underlying)
	}
}

func TestRemoteDownload(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRemoteDownloadPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureRemoteDownloadPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureRemoteDownloadPath)
}

func TestInvalidRemoteDownload(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInvalidRemoteDownloadPath)
	applyStdout := bytes.Buffer{}
	applyStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureInvalidRemoteDownloadPath, &applyStdout, &applyStderr)

	helpers.LogBufferContentsLineByLine(t, applyStdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, applyStderr, "apply stderr")

	require.Error(t, err)
	errMessage := "downloading source url"
	assert.Containsf(t, err.Error(), errMessage, "expected error containing %q, got %s", errMessage, err)

}

func TestRemoteDownloadWithRelativePath(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRemoteRelativeDownloadPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureRemoteRelativeDownloadPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureRemoteRelativeDownloadPath)
}

func TestRemoteDownloadWithRelativePathAndSlashInBranch(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRemoteRelativeDownloadPathWithSlash)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureRemoteRelativeDownloadPathWithSlash)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureRemoteRelativeDownloadPathWithSlash)
}

func TestRemoteDownloadOverride(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureOverrideDonwloadPath)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", testFixtureOverrideDonwloadPath, "../hello-world"))

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", testFixtureOverrideDonwloadPath, "../hello-world"))
}

func TestRemoteWithModuleInRoot(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRemoteModuleInRoot)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRemoteModuleInRoot)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
}

// As of Terraform 0.14.0, if there's already a lock file in the working directory, we should be copying it into
// .terragrunt-cache
func TestCustomLockFile(t *testing.T) {
	t.Parallel()

	path := fmt.Sprintf("%s-%s", testFixtureCustomLockFile, wrappedBinary())
	tmpEnvPath := helpers.CopyEnvironment(t, filepath.Dir(testFixtureCustomLockFile))
	rootPath := util.JoinPath(tmpEnvPath, path)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)

	source := "../custom-lock-file-module"
	downloadDir := util.JoinPath(rootPath, helpers.TerragruntCache)
	result, err := tfsource.NewSource(source, downloadDir, rootPath, createLogger(), false)
	require.NoError(t, err)

	lockFilePath := util.JoinPath(result.WorkingDir, util.TerraformLockFile)
	assert.FileExists(t, lockFilePath)

	readFile, err := os.ReadFile(lockFilePath)
	require.NoError(t, err)

	// In our lock file, we intentionally have hashes for an older version of the AWS provider. If the lock file
	// copying works, then Terraform will stick with this older version. If there is a bug, Terraform will end up
	// installing a newer version (since the version is not pinned in the .tf code, only in the lock file).
	assert.Contains(t, string(readFile), `version     = "5.23.0"`)
}

func TestExcludeDirs(t *testing.T) {
	t.Parallel()

	// Populate module paths.
	moduleNames := []string{
		"integration-env/aws/module-aws-a",
		"integration-env/gce/module-gce-b",
		"integration-env/gce/module-gce-c",
		"production-env/aws/module-aws-d",
		"production-env/gce/module-gce-e",
	}

	testCases := []struct {
		workingDir            string
		excludeArgs           string
		excludedModuleOutputs []string
	}{
		{testFixtureLocalWithExcludeDir, "--terragrunt-exclude-dir **/gce/**/*", []string{"Module GCE B", "Module GCE C", "Module GCE E"}},
		{testFixtureLocalWithExcludeDir, "--terragrunt-exclude-dir production-env/**/* --terragrunt-exclude-dir **/module-gce-c", []string{"Module GCE C", "Module AWS D", "Module GCE E"}},
		{testFixtureLocalWithExcludeDir, "--terragrunt-exclude-dir integration-env/gce/module-gce-b --terragrunt-exclude-dir integration-env/gce/module-gce-c --terragrunt-exclude-dir **/module-aws*", []string{"Module AWS A", "Module GCE B", "Module GCE C", "Module AWS D"}},
	}

	modulePaths := make(map[string]string, len(moduleNames))
	for _, moduleName := range moduleNames {
		modulePaths[moduleName] = util.JoinPath(testFixtureLocalWithExcludeDir, moduleName)
	}

	for _, testCase := range testCases {
		applyAllStdout := bytes.Buffer{}
		applyAllStderr := bytes.Buffer{}

		// Cleanup all modules directories.
		helpers.CleanupTerragruntFolder(t, testFixtureLocalWithExcludeDir)
		for _, modulePath := range modulePaths {
			helpers.CleanupTerragruntFolder(t, modulePath)
		}

		// Apply modules according to test cases
		err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir %s %s", testCase.workingDir, testCase.excludeArgs), &applyAllStdout, &applyAllStderr)

		helpers.LogBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
		helpers.LogBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")

		if err != nil {
			t.Fatalf("apply-all in TestExcludeDirs failed with error: %v. Full std", err)
		}

		// Check that the excluded module output is not present
		for _, modulePath := range modulePaths {
			showStdout := bytes.Buffer{}
			showStderr := bytes.Buffer{}

			err = helpers.RunTerragruntCommand(t, "terragrunt show --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+modulePath, &showStdout, &showStderr)
			helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout for "+modulePath)
			helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr for "+modulePath)

			require.NoError(t, err)
			output := showStdout.String()
			for _, excludedModuleOutput := range testCase.excludedModuleOutputs {
				assert.NotContains(t, output, excludedModuleOutput)
			}

		}
	}
}

func TestIncludeDirs(t *testing.T) {
	t.Parallel()

	// Populate module paths.
	moduleNames := []string{
		"integration-env/aws/module-aws-a",
		"integration-env/gce/module-gce-b",
		"integration-env/gce/module-gce-c",
		"production-env/aws/module-aws-d",
		"production-env/gce/module-gce-e",
	}

	testCases := []struct {
		workingDir            string
		includeArgs           string
		includedModuleOutputs []string
	}{
		{testFixtureLocalWithIncludeDir, "--terragrunt-include-dir xyz", []string{}},
		{testFixtureLocalWithIncludeDir, "--terragrunt-include-dir */aws", []string{"Module GCE B", "Module GCE C", "Module GCE E"}},
		{testFixtureLocalWithIncludeDir, "--terragrunt-include-dir production-env --terragrunt-include-dir **/module-gce-c", []string{"Module GCE B", "Module AWS A"}},
		{testFixtureLocalWithIncludeDir, "--terragrunt-include-dir integration-env/gce/module-gce-b --terragrunt-include-dir integration-env/gce/module-gce-c --terragrunt-include-dir **/module-aws*", []string{"Module GCE E"}},
	}

	modulePaths := make(map[string]string, len(moduleNames))
	for _, moduleName := range moduleNames {
		modulePaths[moduleName] = util.JoinPath(testFixtureLocalWithIncludeDir, moduleName)
	}

	for _, testCase := range testCases {
		applyAllStdout := bytes.Buffer{}
		applyAllStderr := bytes.Buffer{}

		// Cleanup all modules directories.
		helpers.CleanupTerragruntFolder(t, testFixtureLocalWithIncludeDir)
		for _, modulePath := range modulePaths {
			helpers.CleanupTerragruntFolder(t, modulePath)
		}

		// Apply modules according to test cases
		err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive  --terragrunt-log-level trace --terragrunt-working-dir %s %s", testCase.workingDir, testCase.includeArgs), &applyAllStdout, &applyAllStderr)

		helpers.LogBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
		helpers.LogBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")

		if err != nil {
			t.Fatalf("apply-all in TestExcludeDirs failed with error: %v. Full std", err)
		}

		// Check that the included module output is present
		for _, modulePath := range modulePaths {
			showStdout := bytes.Buffer{}
			showStderr := bytes.Buffer{}

			err = helpers.RunTerragruntCommand(t, "terragrunt show --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+modulePath, &showStdout, &showStderr)
			helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout for "+modulePath)
			helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr for "+modulePath)

			require.NoError(t, err)
			output := showStdout.String()
			for _, includedModuleOutput := range testCase.includedModuleOutputs {
				assert.NotContains(t, output, includedModuleOutput)
			}

		}
	}
}

func TestIncludeDirsDependencyConsistencyRegression(t *testing.T) {
	t.Parallel()

	modulePaths := []string{
		"amazing-app/k8s",
		"clusters/eks",
		"testapp/k8s",
	}

	tmpPath, _ := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureRegressions))
	testPath := filepath.Join(tmpPath, testFixtureRegressions, "exclude-dependency")
	for _, modulePath := range modulePaths {
		helpers.CleanupTerragruntFolder(t, filepath.Join(testPath, modulePath))
	}

	includedModulesWithNone := helpers.RunValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{}, false)
	assert.NotEmpty(t, includedModulesWithNone)

	includedModulesWithAmzApp := helpers.RunValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{"amazing-app/k8s"}, false)
	assert.Equal(t, getPathsRelativeTo(t, testPath, []string{"amazing-app/k8s", "clusters/eks"}), includedModulesWithAmzApp)

	includedModulesWithTestApp := helpers.RunValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{"testapp/k8s"}, false)
	assert.Equal(t, getPathsRelativeTo(t, testPath, []string{"clusters/eks", "testapp/k8s"}), includedModulesWithTestApp)
}

func TestIncludeDirsStrict(t *testing.T) {
	t.Parallel()

	modulePaths := []string{
		"amazing-app/k8s",
		"clusters/eks",
		"testapp/k8s",
	}

	tmpPath, _ := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureRegressions))
	testPath := filepath.Join(tmpPath, testFixtureRegressions, "exclude-dependency")
	helpers.CleanupTerragruntFolder(t, testPath)
	for _, modulePath := range modulePaths {
		helpers.CleanupTerragruntFolder(t, filepath.Join(testPath, modulePath))
	}

	includedModulesWithNone := helpers.RunValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{}, true)
	assert.Equal(t, []string{}, includedModulesWithNone)

	includedModulesWithAmzApp := helpers.RunValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{"amazing-app/k8s"}, true)
	assert.Equal(t, getPathsRelativeTo(t, testPath, []string{"amazing-app/k8s"}), includedModulesWithAmzApp)

	includedModulesWithTestApp := helpers.RunValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{"testapp/k8s"}, true)
	assert.Equal(t, getPathsRelativeTo(t, testPath, []string{"testapp/k8s"}), includedModulesWithTestApp)
}

func TestTerragruntExternalDependencies(t *testing.T) {
	t.Parallel()

	modules := []string{
		"module-a",
		"module-b",
	}

	helpers.CleanupTerraformFolder(t, testFixtureExternalDependence)
	for _, module := range modules {
		helpers.CleanupTerraformFolder(t, util.JoinPath(testFixtureExternalDependence, module))
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	rootPath := helpers.CopyEnvironment(t, testFixtureExternalDependence)
	modulePath := util.JoinPath(rootPath, testFixtureExternalDependence, "module-b")

	err := helpers.RunTerragruntCommand(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-include-external-dependencies --terragrunt-forward-tf-stdout --terragrunt-working-dir "+modulePath, &applyAllStdout, &applyAllStderr)
	helpers.LogBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
	helpers.LogBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")
	applyAllStdoutString := applyAllStdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	for _, module := range modules {
		assert.Contains(t, applyAllStdoutString, "Hello World, "+module)
	}
}

func TestPreventDestroy(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/download")
	fixtureRoot := util.JoinPath(tmpEnvPath, testFixtureLocalPreventDestroy)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+fixtureRoot)

	err := helpers.RunTerragruntCommand(t, "terragrunt destroy -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+fixtureRoot, os.Stdout, os.Stderr)

	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, terraform.ModuleIsProtected{}, underlying)
	}
}

func TestPreventDestroyApply(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/download")

	fixtureRoot := util.JoinPath(tmpEnvPath, testFixtureLocalPreventDestroy)
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+fixtureRoot)

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -destroy -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+fixtureRoot, os.Stdout, os.Stderr)

	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, terraform.ModuleIsProtected{}, underlying)
	}
}

func TestPreventDestroyDependencies(t *testing.T) {
	t.Parallel()

	// Populate module paths.
	moduleNames := []string{
		"module-a",
		"module-b",
		"module-c",
		"module-d",
		"module-e",
	}
	modulePaths := make(map[string]string, len(moduleNames))
	for _, moduleName := range moduleNames {
		modulePaths[moduleName] = util.JoinPath(testFixtureLocalPreventDestroyDependencies, moduleName)
	}

	// Cleanup all modules directories.
	helpers.CleanupTerraformFolder(t, testFixtureLocalPreventDestroyDependencies)
	for _, modulePath := range modulePaths {
		helpers.CleanupTerraformFolder(t, modulePath)
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	// Apply and destroy all modules.
	err := helpers.RunTerragruntCommand(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalPreventDestroyDependencies, &applyAllStdout, &applyAllStderr)
	helpers.LogBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
	helpers.LogBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")

	if err != nil {
		t.Fatalf("apply-all in TestPreventDestroyDependencies failed with error: %v. Full std", err)
	}

	var (
		destroyAllStdout bytes.Buffer
		destroyAllStderr bytes.Buffer
	)

	err = helpers.RunTerragruntCommand(t, "terragrunt destroy-all --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalPreventDestroyDependencies, &destroyAllStdout, &destroyAllStderr)
	helpers.LogBufferContentsLineByLine(t, destroyAllStdout, "destroy-all stdout")
	helpers.LogBufferContentsLineByLine(t, destroyAllStderr, "destroy-all stderr")

	require.Error(t, err)

	var multiErrors *errors.MultiError

	if ok := errors.As(err, &multiErrors); ok {
		err = multiErrors
	}

	assert.IsType(t, &errors.MultiError{}, err)

	// Check that modules C, D and E were deleted and modules A and B weren't.
	for moduleName, modulePath := range modulePaths {
		var (
			showStdout bytes.Buffer
			showStderr bytes.Buffer
		)

		err = helpers.RunTerragruntCommand(t, "terragrunt show --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+modulePath, &showStdout, &showStderr)
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
		case "module-d":
			assert.NotContains(t, output, "Hello, Module D")
		case "module-e":
			assert.NotContains(t, output, "Hello, Module E")
		}
	}
}
