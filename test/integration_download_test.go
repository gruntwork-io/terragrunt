package test_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureLocalDownloadPath                      = "fixtures/download/local"
	testFixtureCustomLockFile                         = "fixtures/download/custom-lock-file"
	testFixtureRemoteDownloadPath                     = "fixtures/download/remote"
	testFixtureInvalidRemoteDownloadPath              = "fixtures/download/remote-invalid"
	testFixtureInvalidRemoteDownloadPathWithRetries   = "fixtures/download/remote-invalid-with-retries"
	testFixtureOverrideDownloadPath                   = "fixtures/download/override"
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

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureLocalDownloadPath)

	// As of Terraform 0.14.0 we should be copying the lock file from .terragrunt-cache to the working directory
	assert.FileExists(t, util.JoinPath(testFixtureLocalDownloadPath, util.TerraformLockFile))

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureLocalDownloadPath)
}

func TestLocalDownloadDisableCopyTerraformLockFile(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDisableCopyLockFilePath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureDisableCopyLockFilePath)

	// The terraform lock file should not be copied if `copy_terraform_lock_file = false`
	assert.NoFileExists(t, util.JoinPath(testFixtureDisableCopyLockFilePath, util.TerraformLockFile))

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureDisableCopyLockFilePath)
}

func TestLocalIncludeDisableCopyTerraformLockFile(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureIncludeDisableCopyLockFilePath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureIncludeDisableCopyLockFilePath)

	// The terraform lock file should not be copied if `copy_terraform_lock_file = false`
	assert.NoFileExists(t, util.JoinPath(testFixtureIncludeDisableCopyLockFilePath, util.TerraformLockFile))

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureIncludeDisableCopyLockFilePath)
}

func TestLocalDownloadWithHiddenFolder(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalWithHiddenFolder)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureLocalWithHiddenFolder)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureLocalWithHiddenFolder)
}

func TestLocalDownloadWithAllowedHiddenFiles(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalWithAllowedHidden)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --working-dir %s/live", testFixtureLocalWithAllowedHidden))

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --working-dir %s/live", testFixtureLocalWithAllowedHidden))

	// Validate that the hidden file was copied
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt output -raw text --non-interactive --working-dir %s/live", testFixtureLocalWithAllowedHidden), &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "output stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "output stderr")
	require.NoError(t, err)
	assert.Equal(t, "Hello world", stdout.String())
}

func TestLocalDownloadWithRelativePath(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalRelativeDownloadPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureLocalRelativeDownloadPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureLocalRelativeDownloadPath)
}

func TestLocalWithMissingBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-lock-table-" + strings.ToLower(helpers.UniqueID())

	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/download")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLocalMissingBackend)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath, os.Stdout, os.Stderr)
	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, run.BackendNotDefined{}, underlying)
	}
}

func TestRemoteDownload(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRemoteDownloadPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureRemoteDownloadPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureRemoteDownloadPath)
}

func TestInvalidRemoteDownload(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInvalidRemoteDownloadPath)

	applyStdout := bytes.Buffer{}
	applyStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureInvalidRemoteDownloadPath, &applyStdout, &applyStderr)

	helpers.LogBufferContentsLineByLine(t, applyStdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, applyStderr, "apply stderr")

	require.Error(t, err)

	errMessage := "downloading source url"
	assert.Containsf(t, err.Error(), errMessage, "expected error containing %q, got %s", errMessage, err)
}

func TestInvalidRemoteDownloadWithRetries(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInvalidRemoteDownloadPathWithRetries)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureInvalidRemoteDownloadPathWithRetries)

	require.Error(t, err)

	errMessage := "max retry attempts (2) reached for error"
	assert.Containsf(t, err.Error(), errMessage, "expected error containing %q, got %s", errMessage, err)
}

func TestRemoteDownloadWithRelativePath(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRemoteRelativeDownloadPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureRemoteRelativeDownloadPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureRemoteRelativeDownloadPath)
}

func TestRemoteDownloadWithRelativePathAndSlashInBranch(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRemoteRelativeDownloadPathWithSlash)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureRemoteRelativeDownloadPathWithSlash)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureRemoteRelativeDownloadPathWithSlash)
}

func TestRemoteDownloadOverride(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureOverrideDownloadPath)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --working-dir %s --source %s", testFixtureOverrideDownloadPath, "../hello-world"))

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --working-dir %s --source %s", testFixtureOverrideDownloadPath, "../hello-world"))
}

func TestRemoteWithModuleInRoot(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRemoteModuleInRoot)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRemoteModuleInRoot)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)
}

// As of Terraform 0.14.0, if there's already a lock file in the working directory, we should be copying it into
// .terragrunt-cache
func TestCustomLockFile(t *testing.T) {
	t.Parallel()

	path := fmt.Sprintf("%s-%s", testFixtureCustomLockFile, wrappedBinary())
	tmpEnvPath := helpers.CopyEnvironment(t, filepath.Dir(testFixtureCustomLockFile))
	rootPath := util.JoinPath(tmpEnvPath, path)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --log-level trace --working-dir "+rootPath)

	source := "../custom-lock-file-module"
	downloadDir := util.JoinPath(rootPath, helpers.TerragruntCache)
	result, err := tf.NewSource(createLogger(), source, downloadDir, rootPath, false)
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
		name                  string
		excludeArgs           string
		excludedModuleOutputs []string
		enableDoubleStar      bool
	}{
		{"exclude gce modules with triple star", "--queue-exclude-dir **/gce/**/*", []string{"Module GCE B", "Module GCE C", "Module GCE E"}, false},
		{"exclude production env and gce c modules with triple star", "--queue-exclude-dir production-env/**/* --queue-exclude-dir **/module-gce-c", []string{"Module GCE C", "Module AWS D", "Module GCE E"}, false},
		{"exclude integration env gce b and c modules and aws modules with triple star", "--queue-exclude-dir integration-env/gce/module-gce-b --queue-exclude-dir integration-env/gce/module-gce-c --queue-exclude-dir **/module-aws*", []string{"Module AWS A", "Module GCE B", "Module GCE C", "Module AWS D"}, false},
		{"exclude gce modules with double star", "--queue-exclude-dir **/gce/**", []string{"Module GCE B", "Module GCE C", "Module GCE E"}, true},
		{"exclude production env and gce c modules with double star", "--queue-exclude-dir production-env/**/* --queue-exclude-dir **/module-gce-c", []string{"Module GCE C", "Module AWS D", "Module GCE E"}, true},
		{"exclude integration env gce b and c modules and aws modules with double star", "--queue-exclude-dir integration-env/gce/module-gce-b --queue-exclude-dir integration-env/gce/module-gce-c --queue-exclude-dir **/module-aws*", []string{"Module AWS A", "Module GCE B", "Module GCE C", "Module AWS D"}, true},
	}

	for _, tt := range testCases {
		opts, err := options.NewTerragruntOptionsForTest("running_module_test")
		require.NoError(t, err)

		doubleStarDefaultEnabled := opts.StrictControls.FilterByNames("double-star").Evaluate(t.Context()) != nil
		if doubleStarDefaultEnabled && !tt.enableDoubleStar {
			t.Skip("Skipping test because double-star is already enabled by default")
		}

		tmpDir := helpers.CopyEnvironment(t, "fixtures/download")
		workingDir := util.JoinPath(tmpDir, testFixtureLocalWithExcludeDir)
		workingDir, err = filepath.EvalSymlinks(workingDir)
		require.NoError(t, err)

		modulePaths := make(map[string]string, len(moduleNames))
		for _, moduleName := range moduleNames {
			modulePaths[moduleName] = util.JoinPath(workingDir, moduleName)
		}

		applyAllStdout := bytes.Buffer{}
		applyAllStderr := bytes.Buffer{}

		// Apply modules according to test cases
		strictControl := ""
		if !doubleStarDefaultEnabled && tt.enableDoubleStar {
			strictControl = "--strict-control double-star"
		}

		err = helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt run --all apply --non-interactive --log-level trace --working-dir %s %s %s", workingDir, tt.excludeArgs, strictControl), &applyAllStdout, &applyAllStderr)
		require.NoError(t, err)

		helpers.LogBufferContentsLineByLine(t, applyAllStdout, "run --all apply stdout")
		helpers.LogBufferContentsLineByLine(t, applyAllStderr, "run --all apply stderr")

		// Check that the excluded module output is not present
		for _, modulePath := range modulePaths {
			showStdout := bytes.Buffer{}
			showStderr := bytes.Buffer{}

			err = helpers.RunTerragruntCommand(t, "terragrunt show --non-interactive --log-level trace --working-dir "+modulePath, &showStdout, &showStderr)
			helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout for "+modulePath)
			helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr for "+modulePath)

			require.NoError(t, err)

			output := showStdout.String()
			for _, excludedModuleOutput := range tt.excludedModuleOutputs {
				assert.NotContains(t, output, excludedModuleOutput)
			}
		}
	}
}

func TestExcludeDirsWithFilter(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	// Populate module paths.
	moduleNames := []string{
		"integration-env/aws/module-aws-a",
		"integration-env/gce/module-gce-b",
		"integration-env/gce/module-gce-c",
		"production-env/aws/module-aws-d",
		"production-env/gce/module-gce-e",
	}

	testCases := []struct {
		name                  string
		excludeArgs           string
		excludedModuleOutputs []string
	}{
		{
			name:                  "exclude gce modules",
			excludeArgs:           "--filter '!./**/gce/**'",
			excludedModuleOutputs: []string{"Module GCE B", "Module GCE C", "Module GCE E"},
		},
		{
			name:                  "exclude production env and gce c modules",
			excludeArgs:           "--filter '!./production-env/**' --filter '!./**/module-gce-c'",
			excludedModuleOutputs: []string{"Module GCE C", "Module AWS D", "Module GCE E"},
		},
		{
			name:                  "exclude integration env gce b and c modules and aws modules",
			excludeArgs:           "--filter '!./integration-env/gce/module-gce-b' --filter '!./integration-env/gce/module-gce-c' --filter '!./**/module-aws*'",
			excludedModuleOutputs: []string{"Module AWS A", "Module GCE B", "Module GCE C", "Module AWS D"},
		},
		{
			name:                  "exclude gce modules",
			excludeArgs:           "--filter '!./**/gce/**'",
			excludedModuleOutputs: []string{"Module GCE B", "Module GCE C", "Module GCE E"},
		},
		{
			name:                  "exclude production env and gce c modules",
			excludeArgs:           "--filter '!./production-env/**' --filter '!./**/module-gce-c'",
			excludedModuleOutputs: []string{"Module GCE C", "Module AWS D", "Module GCE E"},
		},
		{
			name:                  "exclude integration env gce b and c modules and aws modules",
			excludeArgs:           "--filter '!./integration-env/gce/module-gce-b' --filter '!./integration-env/gce/module-gce-c' --filter '!./**/module-aws*'",
			excludedModuleOutputs: []string{"Module AWS A", "Module GCE B", "Module GCE C", "Module AWS D"},
		},
	}

	for _, tt := range testCases {
		tmpDir := helpers.CopyEnvironment(t, "fixtures/download")
		workingDir := util.JoinPath(tmpDir, testFixtureLocalWithExcludeDir)
		workingDir, err := filepath.EvalSymlinks(workingDir)
		require.NoError(t, err)

		modulePaths := make(map[string]string, len(moduleNames))
		for _, moduleName := range moduleNames {
			modulePaths[moduleName] = util.JoinPath(workingDir, moduleName)
		}

		// Apply modules according to test cases
		_, _, err = helpers.RunTerragruntCommandWithOutput(
			t,
			fmt.Sprintf(
				"terragrunt run --all apply --non-interactive --log-level trace --working-dir %s %s",
				workingDir,
				tt.excludeArgs,
			),
		)
		require.NoError(t, err)

		// Check that the excluded module output is not present
		for _, modulePath := range modulePaths {
			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt show --non-interactive --log-level trace --working-dir "+modulePath)

			require.NoError(t, err)

			output := stdout
			for _, excludedModuleOutput := range tt.excludedModuleOutputs {
				assert.NotContains(t, output, excludedModuleOutput)
			}
		}
	}
}

/*
	TestIncludeDirs tests that the --queue-include-dir flag works as expected.

MAINTAINER NOTE: Why is this test _so slow_? It took 2 mins on my machine...

We really need to start reporting on test durations and decide on a budget for each test.
I'm not sure we're getting good value from the time taken on tests like this.
*/
func TestIncludeDirs(t *testing.T) {
	t.Parallel()

	// Populate module paths.
	unitNames := []string{
		"integration-env/aws/module-aws-a",
		"integration-env/gce/module-gce-b",
		"integration-env/gce/module-gce-c",
		"production-env/aws/module-aws-d",
		"production-env/gce/module-gce-e",
	}

	testCases := []struct {
		name                string
		includeArgs         string
		includedUnitOutputs []string
	}{
		{
			name:                "no-match",
			includeArgs:         "--queue-include-dir xyz",
			includedUnitOutputs: []string{},
		},
		{
			name:                "wildcard-aws",
			includeArgs:         "--queue-include-dir */aws",
			includedUnitOutputs: []string{"Module GCE B", "Module GCE C", "Module GCE E"},
		},
		{
			name:                "production-and-gce-c",
			includeArgs:         "--queue-include-dir production-env --queue-include-dir **/module-gce-c",
			includedUnitOutputs: []string{"Module GCE B", "Module AWS A"},
		},
		{
			name:                "specific-modules",
			includeArgs:         "--queue-include-dir integration-env/gce/module-gce-b --queue-include-dir integration-env/gce/module-gce-c --queue-include-dir **/module-aws*",
			includedUnitOutputs: []string{"Module GCE E"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.CopyEnvironment(t, "fixtures/download")
			workingDir := util.JoinPath(tmpDir, testFixtureLocalWithIncludeDir)
			workingDir, err := filepath.EvalSymlinks(workingDir)
			require.NoError(t, err)

			unitPaths := make(map[string]string, len(unitNames))
			for _, unitName := range unitNames {
				unitPaths[unitName] = util.JoinPath(workingDir, unitName)
			}

			applyAllStdout := bytes.Buffer{}
			applyAllStderr := bytes.Buffer{}

			// Apply modules according to test case
			err = helpers.RunTerragruntCommand(
				t,
				fmt.Sprintf(
					"terragrunt run --all apply --non-interactive  --log-level trace --working-dir %s %s",
					workingDir, tc.includeArgs,
				),
				&applyAllStdout,
				&applyAllStderr,
			)
			require.NoError(t, err)

			helpers.LogBufferContentsLineByLine(t, applyAllStdout, "run --all apply stdout")
			helpers.LogBufferContentsLineByLine(t, applyAllStderr, "run --all apply stderr")

			// Check that the included module output is present
			for _, modulePath := range unitPaths {
				showStdout := bytes.Buffer{}
				showStderr := bytes.Buffer{}

				err = helpers.RunTerragruntCommand(
					t,
					"terragrunt show --non-interactive --log-level trace --working-dir "+modulePath,
					&showStdout,
					&showStderr,
				)
				helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout for "+modulePath)
				helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr for "+modulePath)

				require.NoError(t, err)

				output := showStdout.String()
				for _, includedUnitOutput := range tc.includedUnitOutputs {
					assert.NotContains(t, output, includedUnitOutput)
				}
			}
		})
	}
}

/*
	TestIncludeDirsWithFilter tests that the --filter flag works as expected, just like in TestIncludeDirs.

MAINTAINER NOTE: Why is this test _so slow_? It took 2 mins on my machine...

We really need to start reporting on test durations and decide on a budget for each test.
I'm not sure we're getting good value from the time taken on tests like this.
*/
func TestIncludeDirsWithFilter(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	// Copy the entire download fixture directory to ensure all referenced sources are available
	tmpDir := helpers.CopyEnvironment(t, "fixtures/download")
	workingDir := util.JoinPath(tmpDir, testFixtureLocalWithIncludeDir)
	workingDir, err := filepath.EvalSymlinks(workingDir)
	require.NoError(t, err)

	// Populate paths.
	unitNames := []string{
		"integration-env/aws/module-aws-a",
		"integration-env/gce/module-gce-b",
		"integration-env/gce/module-gce-c",
		"production-env/aws/module-aws-d",
		"production-env/gce/module-gce-e",
	}

	testCases := []struct {
		includeArgs         string
		includedUnitOutputs []string
	}{
		{
			includeArgs:         "--filter xyz",
			includedUnitOutputs: []string{},
		},
		{
			includeArgs:         "--filter ./*/aws/*",
			includedUnitOutputs: []string{"Module GCE B", "Module GCE C", "Module GCE E"},
		},
		{
			includeArgs:         "--filter production-env --filter ./**/module-gce-c",
			includedUnitOutputs: []string{"Module GCE B", "Module AWS A"},
		},
		{
			includeArgs:         "--filter ./integration-env/gce/module-gce-b --filter ./integration-env/gce/module-gce-c --filter ./**/module-aws**",
			includedUnitOutputs: []string{"Module GCE E"},
		},
	}

	unitPaths := make(map[string]string, len(unitNames))
	for _, unitName := range unitNames {
		unitPaths[unitName] = util.JoinPath(workingDir, unitName)
	}

	for _, tc := range testCases {
		applyAllStdout := bytes.Buffer{}
		applyAllStderr := bytes.Buffer{}

		// Cleanup all modules directories.
		helpers.CleanupTerragruntFolder(t, workingDir)

		for _, unitPath := range unitPaths {
			helpers.CleanupTerragruntFolder(t, unitPath)
		}

		// Apply modules according to test cases
		err := helpers.RunTerragruntCommand(
			t,
			fmt.Sprintf(
				"terragrunt run --all apply --non-interactive  --log-level trace --working-dir %s %s",
				workingDir, tc.includeArgs,
			),
			&applyAllStdout,
			&applyAllStderr,
		)
		require.NoError(t, err)

		helpers.LogBufferContentsLineByLine(t, applyAllStdout, "run --all apply stdout")
		helpers.LogBufferContentsLineByLine(t, applyAllStderr, "run --all apply stderr")

		// Check that the included module output is present
		for _, unitPath := range unitPaths {
			showStdout := bytes.Buffer{}
			showStderr := bytes.Buffer{}

			err = helpers.RunTerragruntCommand(
				t,
				"terragrunt show --non-interactive --log-level trace --working-dir "+unitPath,
				&showStdout,
				&showStderr,
			)
			helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout for "+unitPath)
			helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr for "+unitPath)

			require.NoError(t, err)

			output := showStdout.String()
			for _, includedUnitOutput := range tc.includedUnitOutputs {
				assert.NotContains(t, output, includedUnitOutput)
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

	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --queue-include-external --tf-forward-stdout --working-dir "+modulePath, &applyAllStdout, &applyAllStderr)
	helpers.LogBufferContentsLineByLine(t, applyAllStdout, "run --all apply stdout")
	helpers.LogBufferContentsLineByLine(t, applyAllStderr, "run --all apply stderr")

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

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+fixtureRoot)

	err := helpers.RunTerragruntCommand(t, "terragrunt destroy -auto-approve --non-interactive --working-dir "+fixtureRoot, os.Stdout, os.Stderr)

	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, run.ModuleIsProtected{}, underlying)
	}
}

func TestPreventDestroyApply(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/download")

	fixtureRoot := util.JoinPath(tmpEnvPath, testFixtureLocalPreventDestroy)
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+fixtureRoot)

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -destroy -auto-approve --non-interactive --working-dir "+fixtureRoot, os.Stdout, os.Stderr)

	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, run.ModuleIsProtected{}, underlying)
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
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+testFixtureLocalPreventDestroyDependencies,
		&applyAllStdout,
		&applyAllStderr,
	)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, applyAllStdout, "run --all apply stdout")
	helpers.LogBufferContentsLineByLine(t, applyAllStderr, "run --all apply stderr")

	var (
		destroyAllStdout bytes.Buffer
		destroyAllStderr bytes.Buffer
	)

	err = helpers.RunTerragruntCommand(t, "terragrunt run --all destroy --non-interactive --working-dir "+testFixtureLocalPreventDestroyDependencies, &destroyAllStdout, &destroyAllStderr)
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
		case "module-d":
			assert.NotContains(t, output, "Hello, Module D")
		case "module-e":
			assert.NotContains(t, output, "Hello, Module E")
		}
	}
}

func TestDownloadWithCASEnabled(t *testing.T) {
	t.Parallel()

	fixturePath := "fixtures/download/remote"

	tmpEnvPath := helpers.CopyEnvironment(t, fixturePath)
	testPath := util.JoinPath(tmpEnvPath, fixturePath)
	helpers.CleanupTerraformFolder(t, testPath)

	// Run with CAS experiment enabled
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	cmd := "terragrunt apply --auto-approve --non-interactive --experiment cas --log-level debug --working-dir " + testPath
	err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "Downloading Terraform configurations")
}

func TestCASStorageDirectory(t *testing.T) {
	t.Parallel()

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	expectedCASDir := filepath.Join(homeDir, ".cache", "terragrunt", "cas")

	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/download")
	testPath := util.JoinPath(tmpEnvPath, "fixtures/download/local")

	helpers.CleanupTerraformFolder(t, testPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	cmd := "terragrunt plan --experiment cas --working-dir " + testPath
	_ = helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)

	_, err = os.Stat(expectedCASDir)
	require.NoError(t, err)

	storeDir := filepath.Join(expectedCASDir, "store")
	_, err = os.Stat(storeDir)
	require.NoError(t, err)
}
