package test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/config"
	tfsource "github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureLocalDownloadPath          = "fixture-download/local"
	testFixtureCustomLockFile             = "fixture-download/custom-lock-file"
	testFixtureRemoteDownloadPath         = "fixture-download/remote"
	testFixtureInvalidRemoteDownloadPath  = "fixture-download/remote-invalid"
	testFixtureOverrideDonwloadPath       = "fixture-download/override"
	testFixtureLocalRelativeDownloadPath  = "fixture-download/local-relative"
	testFixtureRemoteRelativeDownloadPath = "fixture-download/remote-relative"
	testFixtureLocalWithBackend           = "fixture-download/local-with-backend"
	testFixtureLocalWithExcludeDir        = "fixture-download/local-with-exclude-dir"
	testFixtureLocalWithIncludeDir        = "fixture-download/local-with-include-dir"
	testFixtureRemoteWithBackend          = "fixture-download/remote-with-backend"
	testFixtureRemoteModuleInRoot         = "fixture-download/remote-module-in-root"
	testFixtureLocalMissingBackend        = "fixture-download/local-with-missing-backend"
	testFixtureLocalWithHiddenFolder      = "fixture-download/local-with-hidden-folder"
	testFixtureLocalWithAllowedHidden     = "fixture-download/local-with-allowed-hidden"
)

func TestLocalDownload(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLocalDownloadPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", testFixtureLocalDownloadPath))

	// As of Terraform 0.14.0 we should be copying the lock file from .terragrunt-cache to the working directory
	assert.FileExists(t, util.JoinPath(testFixtureLocalDownloadPath, util.TerraformLockFile))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", testFixtureLocalDownloadPath))
}

func TestLocalDownloadWithHiddenFolder(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLocalWithHiddenFolder)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", testFixtureLocalWithHiddenFolder))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", testFixtureLocalWithHiddenFolder))
}

func TestLocalDownloadWithAllowedHiddenFiles(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLocalWithAllowedHidden)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s/live", testFixtureLocalWithAllowedHidden))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s/live", testFixtureLocalWithAllowedHidden))

	// Validate that the hidden file was copied
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -raw text --terragrunt-non-interactive --terragrunt-working-dir %s/live", testFixtureLocalWithAllowedHidden), &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "output stdout")
	logBufferContentsLineByLine(t, stderr, "output stderr")
	require.NoError(t, err)
	assert.Equal(t, "Hello world", stdout.String())
}

func TestLocalDownloadWithRelativePath(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLocalRelativeDownloadPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", testFixtureLocalRelativeDownloadPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", testFixtureLocalRelativeDownloadPath))
}

func TestLocalWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-lock-table-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpEnvPath := copyEnvironment(t, "fixture-download")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLocalWithBackend)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
}

func TestLocalWithMissingBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-lock-table-%s", strings.ToLower(uniqueId()))

	tmpEnvPath := copyEnvironment(t, "fixture-download")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLocalMissingBackend)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), os.Stdout, os.Stderr)
	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, terraform.BackendNotDefined{}, underlying)
	}
}

func TestRemoteDownload(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureRemoteDownloadPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", testFixtureRemoteDownloadPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", testFixtureRemoteDownloadPath))
}

func TestInvalidRemoteDownload(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureInvalidRemoteDownloadPath)
	applyStdout := bytes.Buffer{}
	applyStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", testFixtureInvalidRemoteDownloadPath), &applyStdout, &applyStderr)

	logBufferContentsLineByLine(t, applyStdout, "apply stdout")
	logBufferContentsLineByLine(t, applyStderr, "apply stderr")

	assert.Error(t, err)
	errMessage := "downloading source url"
	assert.Containsf(t, err.Error(), errMessage, "expected error containing %q, got %s", errMessage, err)

}

func TestRemoteDownloadWithRelativePath(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureRemoteRelativeDownloadPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", testFixtureRemoteRelativeDownloadPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", testFixtureRemoteRelativeDownloadPath))
}

func TestRemoteDownloadOverride(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureOverrideDonwloadPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", testFixtureOverrideDonwloadPath, "../hello-world"))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", testFixtureOverrideDonwloadPath, "../hello-world"))
}

func TestRemoteWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-lock-table-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpEnvPath := copyEnvironment(t, testFixtureRemoteWithBackend)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRemoteWithBackend)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
}

func TestRemoteWithModuleInRoot(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureRemoteModuleInRoot)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRemoteModuleInRoot)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
}

// As of Terraform 0.14.0, if there's already a lock file in the working directory, we should be copying it into
// .terragrunt-cache
func TestCustomLockFile(t *testing.T) {
	t.Parallel()

	path := fmt.Sprintf("%s-%s", testFixtureCustomLockFile, wrappedBinary())
	tmpEnvPath := copyEnvironment(t, filepath.Dir(testFixtureCustomLockFile))
	rootPath := util.JoinPath(tmpEnvPath, path)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", rootPath))

	source := "../custom-lock-file-module"
	downloadDir := util.JoinPath(rootPath, TERRAGRUNT_CACHE)
	result, err := tfsource.NewSource(source, downloadDir, rootPath, util.CreateLogEntry("", util.GetDefaultLogLevel()))
	require.NoError(t, err)

	lockFilePath := util.JoinPath(result.WorkingDir, util.TerraformLockFile)
	require.FileExists(t, lockFilePath)

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
		cleanupTerragruntFolder(t, testFixtureLocalWithExcludeDir)
		for _, modulePath := range modulePaths {
			cleanupTerragruntFolder(t, modulePath)
		}

		// Apply modules according to test cases
		err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s %s", testCase.workingDir, testCase.excludeArgs), &applyAllStdout, &applyAllStderr)

		logBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
		logBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")

		if err != nil {
			t.Fatalf("apply-all in TestExcludeDirs failed with error: %v. Full std", err)
		}

		// Check that the excluded module output is not present
		for _, modulePath := range modulePaths {
			showStdout := bytes.Buffer{}
			showStderr := bytes.Buffer{}

			err = runTerragruntCommand(t, fmt.Sprintf("terragrunt show --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", modulePath), &showStdout, &showStderr)
			logBufferContentsLineByLine(t, showStdout, fmt.Sprintf("show stdout for %s", modulePath))
			logBufferContentsLineByLine(t, showStderr, fmt.Sprintf("show stderr for %s", modulePath))

			assert.NoError(t, err)
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
		cleanupTerragruntFolder(t, testFixtureLocalWithIncludeDir)
		for _, modulePath := range modulePaths {
			cleanupTerragruntFolder(t, modulePath)
		}

		// Apply modules according to test cases
		err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive  --terragrunt-log-level debug --terragrunt-working-dir %s %s", testCase.workingDir, testCase.includeArgs), &applyAllStdout, &applyAllStderr)

		logBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
		logBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")

		if err != nil {
			t.Fatalf("apply-all in TestExcludeDirs failed with error: %v. Full std", err)
		}

		// Check that the included module output is present
		for _, modulePath := range modulePaths {
			showStdout := bytes.Buffer{}
			showStderr := bytes.Buffer{}

			err = runTerragruntCommand(t, fmt.Sprintf("terragrunt show --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", modulePath), &showStdout, &showStderr)
			logBufferContentsLineByLine(t, showStdout, fmt.Sprintf("show stdout for %s", modulePath))
			logBufferContentsLineByLine(t, showStderr, fmt.Sprintf("show stderr for %s", modulePath))

			assert.NoError(t, err)
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

	tmpPath, _ := filepath.EvalSymlinks(copyEnvironment(t, TEST_FIXTURE_REGRESSIONS))
	testPath := filepath.Join(tmpPath, TEST_FIXTURE_REGRESSIONS, "exclude-dependency")
	for _, modulePath := range modulePaths {
		cleanupTerragruntFolder(t, filepath.Join(testPath, modulePath))
	}

	includedModulesWithNone := runValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{}, false)
	assert.Greater(t, len(includedModulesWithNone), 0)

	includedModulesWithAmzApp := runValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{"amazing-app/k8s"}, false)
	assert.Equal(t, []string{"amazing-app/k8s", "clusters/eks"}, includedModulesWithAmzApp)

	includedModulesWithTestApp := runValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{"testapp/k8s"}, false)
	assert.Equal(t, []string{"clusters/eks", "testapp/k8s"}, includedModulesWithTestApp)
}

func TestIncludeDirsStrict(t *testing.T) {
	t.Parallel()

	modulePaths := []string{
		"amazing-app/k8s",
		"clusters/eks",
		"testapp/k8s",
	}

	tmpPath, _ := filepath.EvalSymlinks(copyEnvironment(t, TEST_FIXTURE_REGRESSIONS))
	testPath := filepath.Join(tmpPath, TEST_FIXTURE_REGRESSIONS, "exclude-dependency")
	cleanupTerragruntFolder(t, testPath)
	for _, modulePath := range modulePaths {
		cleanupTerragruntFolder(t, filepath.Join(testPath, modulePath))
	}

	includedModulesWithNone := runValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{}, true)
	assert.Equal(t, []string{}, includedModulesWithNone)

	includedModulesWithAmzApp := runValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{"amazing-app/k8s"}, true)
	assert.Equal(t, []string{"amazing-app/k8s"}, includedModulesWithAmzApp)

	includedModulesWithTestApp := runValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{"testapp/k8s"}, true)
	assert.Equal(t, []string{"testapp/k8s"}, includedModulesWithTestApp)
}

func TestTerragruntExternalDependencies(t *testing.T) {
	t.Parallel()

	modules := []string{
		"module-a",
		"module-b",
	}

	cleanupTerraformFolder(t, TEST_FIXTURE_EXTERNAL_DEPENDENCE)
	for _, module := range modules {
		cleanupTerraformFolder(t, util.JoinPath(TEST_FIXTURE_EXTERNAL_DEPENDENCE, module))
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	rootPath := copyEnvironment(t, TEST_FIXTURE_EXTERNAL_DEPENDENCE)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_EXTERNAL_DEPENDENCE, "module-b")

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-include-external-dependencies --terragrunt-working-dir %s", modulePath), &applyAllStdout, &applyAllStderr)
	logBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
	logBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")
	applyAllStdoutString := applyAllStdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	for _, module := range modules {
		assert.Contains(t, applyAllStdoutString, fmt.Sprintf("Hello World, %s", module))
	}
}
