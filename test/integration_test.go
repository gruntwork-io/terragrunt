package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"

	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/configstack"
	terragruntDynamoDb "github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"
)

// hard-code this to match the test fixture for now
const (
	TERRAFORM_REMOTE_STATE_S3_REGION                        = "us-west-2"
	TERRAFORM_REMOTE_STATE_GCP_REGION                       = "eu"
	TEST_FIXTURE_PATH                                       = "fixture/"
	TEST_FIXTURE_GCS_PATH                                   = "fixture-gcs/"
	TEST_FIXTURE_GCS_BYO_BUCKET_PATH                        = "fixture-gcs-byo-bucket/"
	TEST_FIXTURE_INCLUDE_PATH                               = "fixture-include/"
	TEST_FIXTURE_INCLUDE_CHILD_REL_PATH                     = "qa/my-app"
	TEST_FIXTURE_STACK                                      = "fixture-stack/"
	TEST_FIXTURE_OUTPUT_ALL                                 = "fixture-output-all"
	TEST_FIXTURE_STDOUT                                     = "fixture-download/stdout-test"
	TEST_FIXTURE_EXTRA_ARGS_PATH                            = "fixture-extra-args/"
	TEST_FIXTURE_ENV_VARS_BLOCK_PATH                        = "fixture-env-vars-block/"
	TEST_FIXTURE_SKIP                                       = "fixture-skip/"
	TEST_FIXTURE_LOCAL_DOWNLOAD_PATH                        = "fixture-download/local"
	TEST_FIXTURE_REMOTE_DOWNLOAD_PATH                       = "fixture-download/remote"
	TEST_FIXTURE_OVERRIDE_DOWNLOAD_PATH                     = "fixture-download/override"
	TEST_FIXTURE_LOCAL_RELATIVE_DOWNLOAD_PATH               = "fixture-download/local-relative"
	TEST_FIXTURE_REMOTE_RELATIVE_DOWNLOAD_PATH              = "fixture-download/remote-relative"
	TEST_FIXTURE_LOCAL_WITH_BACKEND                         = "fixture-download/local-with-backend"
	TEST_FIXTURE_LOCAL_WITH_EXCLUDE_DIR                     = "fixture-download/local-with-exclude-dir"
	TEST_FIXTURE_LOCAL_WITH_INCLUDE_DIR                     = "fixture-download/local-with-include-dir"
	TEST_FIXTURE_REMOTE_WITH_BACKEND                        = "fixture-download/remote-with-backend"
	TEST_FIXTURE_REMOTE_MODULE_IN_ROOT                      = "fixture-download/remote-module-in-root"
	TEST_FIXTURE_LOCAL_MISSING_BACKEND                      = "fixture-download/local-with-missing-backend"
	TEST_FIXTURE_LOCAL_WITH_HIDDEN_FOLDER                   = "fixture-download/local-with-hidden-folder"
	TEST_FIXTURE_LOCAL_PREVENT_DESTROY                      = "fixture-download/local-with-prevent-destroy"
	TEST_FIXTURE_LOCAL_PREVENT_DESTROY_DEPENDENCIES         = "fixture-download/local-with-prevent-destroy-dependencies"
	TEST_FIXTURE_LOCAL_INCLUDE_PREVENT_DESTROY_DEPENDENCIES = "fixture-download/local-include-with-prevent-destroy-dependencies"
	TEST_FIXTURE_EXTERNAL_DEPENDENCIE                       = "fixture-external-dependencies"
	TEST_FIXTURE_HOOKS_BEFORE_ONLY_PATH                     = "fixture-hooks/before-only"
	TEST_FIXTURE_HOOKS_AFTER_ONLY_PATH                      = "fixture-hooks/after-only"
	TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_PATH                = "fixture-hooks/before-and-after"
	TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_MERGE_PATH          = "fixture-hooks/before-and-after-merge"
	TEST_FIXTURE_HOOKS_SKIP_ON_ERROR_PATH                   = "fixture-hooks/skip-on-error"
	TEST_FIXTURE_HOOKS_ONE_ARG_ACTION_PATH                  = "fixture-hooks/one-arg-action"
	TEST_FIXTURE_HOOKS_EMPTY_STRING_COMMAND_PATH            = "fixture-hooks/bad-arg-action/empty-string-command"
	TEST_FIXTURE_HOOKS_EMPTY_COMMAND_LIST_PATH              = "fixture-hooks/bad-arg-action/empty-command-list"
	TEST_FIXTURE_HOOKS_INTERPOLATIONS_PATH                  = "fixture-hooks/interpolations"
	TEST_FIXTURE_HOOKS_INIT_ONCE_NO_SOURCE_NO_BACKEND       = "fixture-hooks/init-once/no-source-no-backend"
	TEST_FIXTURE_HOOKS_INIT_ONCE_NO_SOURCE_WITH_BACKEND     = "fixture-hooks/init-once/no-source-with-backend"
	TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND     = "fixture-hooks/init-once/with-source-no-backend"
	TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_WITH_BACKEND   = "fixture-hooks/init-once/with-source-with-backend"
	TEST_FIXTURE_FAILED_TERRAFORM                           = "fixture-failure"
	TEST_FIXTURE_EXIT_CODE                                  = "fixture-exit-code"
	TEST_FIXTURE_AUTO_RETRY_RERUN                           = "fixture-auto-retry/re-run"
	TEST_FIXTURE_AUTO_RETRY_EXHAUST                         = "fixture-auto-retry/exhaust"
	TEST_FIXTURE_AUTO_RETRY_APPLY_ALL_RETRIES               = "fixture-auto-retry/apply-all"
	TEST_FIXTURE_INPUTS                                     = "fixture-inputs"
	TERRAFORM_BINARY                                        = "terraform"
	TERRAFORM_FOLDER                                        = ".terraform"
	TERRAFORM_STATE                                         = "terraform.tfstate"
	TERRAFORM_STATE_BACKUP                                  = "terraform.tfstate.backup"
	TERRAGRUNT_CACHE                                        = ".terragrunt-cache"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func TestTerragruntInitHookNoSourceNoBackend(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_INIT_ONCE_NO_SOURCE_NO_BACKEND)
	tmpEnvPath := copyEnvironment(t, "fixture-hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_INIT_ONCE_NO_SOURCE_NO_BACKEND)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	output := stderr.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	// `init` hook should execute only once (2 occurrences due to the echo and its output)
	assert.Equal(t, 2, strings.Count(output, "AFTER_INIT_ONLY_ONCE"), "Hooks on init command executed more than once")
	// With no source, `init-from-module` should not execute
	assert.NotContains(t, output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE", "Hooks on init-from-module command executed when no source was specified")
}

func TestTerragruntInitHookNoSourceWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_INIT_ONCE_NO_SOURCE_WITH_BACKEND)
	tmpEnvPath := copyEnvironment(t, "fixture-hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_INIT_ONCE_NO_SOURCE_WITH_BACKEND)

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", TERRAFORM_REMOTE_STATE_S3_REGION)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	output := stderr.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	// `init` hook should execute only once (2 occurrences due to the echo and its output)
	assert.Equal(t, 2, strings.Count(output, "AFTER_INIT_ONLY_ONCE"), "Hooks on init command executed more than once")
	// With no source, `init-from-module` should not execute
	assert.NotContains(t, output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE", "Hooks on init-from-module command executed when no source was specified")
}

func TestTerragruntInitHookWithSourceNoBackend(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND)
	tmpEnvPath := copyEnvironment(t, "fixture-hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	output := stderr.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	// `init` hook should execute only once (2 occurrences due to the echo and its output)
	assert.Equal(t, 2, strings.Count(output, "AFTER_INIT_ONLY_ONCE"), "Hooks on init command executed more than once")
	// `init-from-module` hook should execute only once (2 occurrences due to the echo and its output)
	assert.Equal(t, 2, strings.Count(output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE"), "Hooks on init-from-module command executed more than once")
}

func TestTerragruntInitHookWithSourceWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_WITH_BACKEND)
	tmpEnvPath := copyEnvironment(t, "fixture-hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_WITH_BACKEND)

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", TERRAFORM_REMOTE_STATE_S3_REGION)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	output := stderr.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	// `init` hook should execute only once (2 occurrences due to the echo and its output)
	assert.Equal(t, 2, strings.Count(output, "AFTER_INIT_ONLY_ONCE"), "Hooks on init command executed more than once")
	// `init-from-module` hook should execute only once (2 occurrences due to the echo and its output)
	assert.Equal(t, 2, strings.Count(output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE"), "Hooks on init-from-module command executed more than once")
}

func TestTerragruntBeforeHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_BEFORE_ONLY_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_BEFORE_ONLY_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_BEFORE_ONLY_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	_, exception := ioutil.ReadFile(rootPath + "/file.out")

	assert.NoError(t, exception)
}

func TestTerragruntAfterHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_AFTER_ONLY_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_AFTER_ONLY_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_AFTER_ONLY_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	_, exception := ioutil.ReadFile(rootPath + "/file.out")

	assert.NoError(t, exception)
}

func TestTerragruntBeforeAndAfterHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	_, beforeException := ioutil.ReadFile(rootPath + "/before.out")
	_, afterException := ioutil.ReadFile(rootPath + "/after.out")

	assert.NoError(t, beforeException)
	assert.NoError(t, afterException)
}

func TestTerragruntBeforeAndAfterMergeHook(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_MERGE_PATH, TEST_FIXTURE_INCLUDE_CHILD_REL_PATH)
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	t.Logf("bucketName: %s", s3BucketName)
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_MERGE_PATH, TEST_FIXTURE_INCLUDE_CHILD_REL_PATH, s3BucketName, config.DefaultTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))

	_, beforeException := ioutil.ReadFile(childPath + "/before.out")
	_, beforeChildException := ioutil.ReadFile(childPath + "/before-child.out")
	_, beforeOverriddenParentException := ioutil.ReadFile(childPath + "/before-parent.out")
	_, afterException := ioutil.ReadFile(childPath + "/after.out")
	_, afterParentException := ioutil.ReadFile(childPath + "/after-parent.out")

	assert.NoError(t, beforeException)
	assert.NoError(t, beforeChildException)
	assert.NoError(t, afterException)
	assert.NoError(t, afterParentException)

	// PathError because no file found
	assert.Error(t, beforeOverriddenParentException)
}

func TestTerragruntSkipOnError(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_SKIP_ON_ERROR_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_SKIP_ON_ERROR_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_SKIP_ON_ERROR_PATH)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)

	output := stderr.String()
	if err != nil {
		assert.Contains(t, output, "BEFORE_SHOULD_DISPLAY")
		assert.NotContains(t, output, "BEFORE_NODISPLAY")

		assert.Contains(t, output, "AFTER_SHOULD_DISPLAY")
		assert.NotContains(t, output, "AFTER_NODISPLAY")
	} else {
		t.Error("Expected NO terragrunt execution due to previous errors but it did run.")
	}
}

func TestTerragruntBeforeOneArgAction(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_ONE_ARG_ACTION_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_ONE_ARG_ACTION_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_ONE_ARG_ACTION_PATH)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	output := stderr.String()

	if err != nil {
		t.Error("Expected successful execution of terragrunt with 1 before hook execution.")
	} else {
		assert.Contains(t, output, "Running command: date")
	}
}

func TestTerragruntEmptyStringCommandHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_EMPTY_STRING_COMMAND_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_EMPTY_STRING_COMMAND_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_EMPTY_STRING_COMMAND_PATH)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)

	if err != nil {
		assert.Contains(t, err.Error(), "Need at least one non-empty argument in 'execute'.")
	} else {
		t.Error("Expected an Error with message: 'Need at least one argument'")
	}
}

func TestTerragruntEmptyCommandListHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_EMPTY_COMMAND_LIST_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_EMPTY_COMMAND_LIST_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_EMPTY_COMMAND_LIST_PATH)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)

	if err != nil {
		assert.Contains(t, err.Error(), "Need at least one non-empty argument in 'execute'.")
	} else {
		t.Error("Expected an Error with message: 'Need at least one argument'")
	}
}

func TestTerragruntHookInterpolation(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_INTERPOLATIONS_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_INTERPOLATIONS_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_INTERPOLATIONS_PATH)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	erroutput := stderr.String()

	homePath := os.Getenv("HOME")
	if homePath == "" {
		homePath = "HelloWorld"
	}

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Contains(t, erroutput, homePath)

}

func TestTerragruntWorksWithLocalTerraformVersion(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_PATH)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, TEST_FIXTURE_PATH, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, TEST_FIXTURE_PATH))

	var expectedS3Tags = map[string]string{
		"owner": "terragrunt integration test",
		"name":  "Terraform state storage"}
	validateS3BucketExistsAndIsTagged(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName, expectedS3Tags)

	var expectedDynamoDBTableTags = map[string]string{
		"owner": "terragrunt integration test",
		"name":  "Terraform lock table"}
	validateDynamoDBTableExistsAndIsTagged(t, TERRAFORM_REMOTE_STATE_S3_REGION, lockTableName, expectedDynamoDBTableTags)
}

func TestTerragruntWorksWithGCSBackend(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GCS_PATH)

	// We need a project to create the bucket in, so we pull one from the recommended environment variable.
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	defer deleteGCSBucket(t, gcsBucketName)

	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(t, TEST_FIXTURE_GCS_PATH, project, TERRAFORM_REMOTE_STATE_GCP_REGION, gcsBucketName, config.DefaultTerragruntConfigPath)
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntGCSConfigPath, TEST_FIXTURE_GCS_PATH))

	var expectedGCSLabels = map[string]string{
		"owner": "terragrunt_test",
		"name":  "terraform_state_storage"}
	validateGCSBucketExistsAndIsLabeled(t, TERRAFORM_REMOTE_STATE_GCP_REGION, gcsBucketName, expectedGCSLabels)
}

func TestTerragruntWorksWithExistingGCSBucket(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GCS_BYO_BUCKET_PATH)

	// We need a project to create the bucket in, so we pull one from the recommended environment variable.
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	defer deleteGCSBucket(t, gcsBucketName)

	// manually create the GCS bucket outside the US (default) to test Terragrunt works correctly with an existing bucket.
	location := TERRAFORM_REMOTE_STATE_GCP_REGION
	createGCSBucket(t, project, location, gcsBucketName)

	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(t, TEST_FIXTURE_GCS_BYO_BUCKET_PATH, project, TERRAFORM_REMOTE_STATE_GCP_REGION, gcsBucketName, config.DefaultTerragruntConfigPath)
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntGCSConfigPath, TEST_FIXTURE_GCS_BYO_BUCKET_PATH))

	validateGCSBucketExistsAndIsLabeled(t, location, gcsBucketName, nil)
}

func TestTerragruntWorksWithIncludes(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(TEST_FIXTURE_INCLUDE_PATH, TEST_FIXTURE_INCLUDE_CHILD_REL_PATH)
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, TEST_FIXTURE_INCLUDE_PATH, TEST_FIXTURE_INCLUDE_CHILD_REL_PATH, s3BucketName, config.DefaultTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
}

func TestTerragruntReportsTerraformErrorsWithPlanAll(t *testing.T) {

	cleanupTerraformFolder(t, TEST_FIXTURE_FAILED_TERRAFORM)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_FAILED_TERRAFORM)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, "fixture-failure")

	cmd := fmt.Sprintf("terragrunt plan-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootTerragruntConfigPath)
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	// Call runTerragruntCommand directly because this command contains failures (which causes runTerragruntRedirectOutput to abort) but we don't care.
	if err := runTerragruntCommand(t, cmd, &stdout, &stderr); err == nil {
		t.Fatalf("Failed to properly fail command: %v. The terraform should be bad", cmd)
	}
	output := stdout.String()
	errOutput := stderr.String()
	fmt.Printf("STDERR is %s.\n STDOUT is %s", errOutput, output)
	assert.True(t, strings.Contains(errOutput, "missingvar1") || strings.Contains(output, "missingvar1"))
	assert.True(t, strings.Contains(errOutput, "missingvar2") || strings.Contains(output, "missingvar2"))
}

func TestTerragruntOutputAllCommand(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_OUTPUT_ALL)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", environmentPath))

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt output-all --terragrunt-non-interactive --terragrunt-working-dir %s", environmentPath), &stdout, &stderr)
	output := stdout.String()

	assert.True(t, strings.Contains(output, "app1 output"))
	assert.True(t, strings.Contains(output, "app2 output"))
	assert.True(t, strings.Contains(output, "app3 output"))

	assert.True(t, (strings.Index(output, "app3 output") < strings.Index(output, "app1 output")) && (strings.Index(output, "app1 output") < strings.Index(output, "app2 output")))
}

func TestTerragruntValidateAllCommand(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_OUTPUT_ALL)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL)

	runTerragrunt(t, fmt.Sprintf("terragrunt validate-all --terragrunt-non-interactive --terragrunt-working-dir %s", environmentPath))
}

// Check that Terragrunt does not pollute stdout with anything
func TestTerragruntStdOut(t *testing.T) {
	t.Parallel()

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_STDOUT))
	runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt output foo --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_STDOUT), &stdout, &stderr)

	output := stdout.String()
	assert.Equal(t, "foo\n", output)
}

func TestTerragruntOutputAllCommandSpecificVariableIgnoreDependencyErrors(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_OUTPUT_ALL)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", environmentPath))

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	// Call runTerragruntCommand directly because this command contains failures (which causes runTerragruntRedirectOutput to abort) but we don't care.
	runTerragruntCommand(t, fmt.Sprintf("terragrunt output-all app2_text --terragrunt-ignore-dependency-errors --terragrunt-non-interactive --terragrunt-working-dir %s", environmentPath), &stdout, &stderr)
	output := stdout.String()

	// Without --terragrunt-ignore-dependency-errors, app2 never runs because its dependencies have "errors" since they don't have the output "app2_text".
	assert.True(t, strings.Contains(output, "app2 output"))
}

func TestTerragruntStackCommands(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_STACK)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, "fixture-stack", config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	mgmtEnvironmentPath := fmt.Sprintf("%s/fixture-stack/mgmt", tmpEnvPath)
	stageEnvironmentPath := fmt.Sprintf("%s/fixture-stack/stage", tmpEnvPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", mgmtEnvironmentPath))
	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", stageEnvironmentPath))

	runTerragrunt(t, fmt.Sprintf("terragrunt output-all --terragrunt-non-interactive --terragrunt-working-dir %s", mgmtEnvironmentPath))
	runTerragrunt(t, fmt.Sprintf("terragrunt output-all --terragrunt-non-interactive --terragrunt-working-dir %s", stageEnvironmentPath))

	runTerragrunt(t, fmt.Sprintf("terragrunt destroy-all --terragrunt-non-interactive --terragrunt-working-dir %s", stageEnvironmentPath))
	runTerragrunt(t, fmt.Sprintf("terragrunt destroy-all --terragrunt-non-interactive --terragrunt-working-dir %s", mgmtEnvironmentPath))
}

func TestLocalDownload(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_DOWNLOAD_PATH))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_DOWNLOAD_PATH))
}

func TestLocalDownloadWithHiddenFolder(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_WITH_HIDDEN_FOLDER)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_WITH_HIDDEN_FOLDER))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_WITH_HIDDEN_FOLDER))
}

func TestLocalDownloadWithRelativePath(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_RELATIVE_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_RELATIVE_DOWNLOAD_PATH))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_RELATIVE_DOWNLOAD_PATH))
}

func TestRemoteDownload(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_REMOTE_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_REMOTE_DOWNLOAD_PATH))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_REMOTE_DOWNLOAD_PATH))
}

func TestRemoteDownloadWithRelativePath(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_REMOTE_RELATIVE_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_REMOTE_RELATIVE_DOWNLOAD_PATH))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_REMOTE_RELATIVE_DOWNLOAD_PATH))
}

func TestRemoteDownloadOverride(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_OVERRIDE_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", TEST_FIXTURE_OVERRIDE_DOWNLOAD_PATH, "../hello-world"))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", TEST_FIXTURE_OVERRIDE_DOWNLOAD_PATH, "../hello-world"))
}

func TestLocalWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-lock-table-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpEnvPath := copyEnvironment(t, "fixture-download")
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_LOCAL_WITH_BACKEND)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
}

func TestLocalWithMissingBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-lock-table-%s", strings.ToLower(uniqueId()))

	tmpEnvPath := copyEnvironment(t, "fixture-download")
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_LOCAL_MISSING_BACKEND)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), os.Stdout, os.Stderr)
	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, cli.BackendNotDefined{}, underlying)
	}
}

func TestRemoteWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-lock-table-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_REMOTE_WITH_BACKEND)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_REMOTE_WITH_BACKEND)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
}

func TestRemoteWithModuleInRoot(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_REMOTE_MODULE_IN_ROOT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_REMOTE_MODULE_IN_ROOT)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
}

// Run terragrunt plan -detailed-exitcode on a folder with some uncreated resources and make sure that you get an exit
// code of "2", which means there are changes to apply.
func TestExitCode(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, TEST_FIXTURE_EXIT_CODE)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_EXIT_CODE)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan -detailed-exitcode --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), os.Stdout, os.Stderr)

	exitCode, exitCodeErr := shell.GetExitCode(err)
	assert.Nil(t, exitCodeErr)
	assert.Equal(t, 2, exitCode)
}

func TestExtraArguments(t *testing.T) {
	// Do not use t.Parallel() on this test, it will infers with the other TestExtraArguments.* tests
	out := new(bytes.Buffer)
	runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_EXTRA_ARGS_PATH), out, os.Stderr)
	t.Log(out.String())
	assert.Contains(t, out.String(), "Hello, World from dev!")
}

func TestExtraArgumentsWithEnv(t *testing.T) {
	// Do not use t.Parallel() on this test, it will infers with the other TestExtraArguments.* tests
	out := new(bytes.Buffer)
	os.Setenv("TF_VAR_env", "prod")
	defer os.Unsetenv("TF_VAR_env")
	runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_EXTRA_ARGS_PATH), out, os.Stderr)
	t.Log(out.String())
	assert.Contains(t, out.String(), "Hello, World!")
}

func TestExtraArgumentsWithEnvVarBlock(t *testing.T) {
	out := new(bytes.Buffer)
	runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_ENV_VARS_BLOCK_PATH), out, os.Stderr)
	t.Log(out.String())
	assert.Contains(t, out.String(), "I'm set in extra_arguments env_vars")
}

func TestExtraArgumentsWithRegion(t *testing.T) {
	// Do not use t.Parallel() on this test, it will infers with the other TestExtraArguments.* tests
	out := new(bytes.Buffer)
	os.Setenv("TF_VAR_region", "us-west-2")
	defer os.Unsetenv("TF_VAR_region")
	runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_EXTRA_ARGS_PATH), out, os.Stderr)
	t.Log(out.String())
	assert.Contains(t, out.String(), "Hello, World from Oregon!")
}

func TestPriorityOrderOfArgument(t *testing.T) {
	// Do not use t.Parallel() on this test, it will infers with the other TestExtraArguments.* tests
	out := new(bytes.Buffer)
	injectedValue := "Injected-directly-by-argument"
	runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt apply -var extra_var=%s --terragrunt-non-interactive --terragrunt-working-dir %s", injectedValue, TEST_FIXTURE_EXTRA_ARGS_PATH), out, os.Stderr)
	t.Log(out.String())
	// And the result value for test should be the injected variable since the injected arguments are injected before the suplied parameters,
	// so our override of extra_var should be the last argument.
	assert.Contains(t, out.String(), fmt.Sprintf("test = %s", injectedValue))
}

func TestAutoRetryBasicRerun(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_RERUN)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_RERUN)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.Nil(t, err)
	assert.Contains(t, out.String(), "Apply complete!")
}

func TestAutoRetrySkip(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_RERUN)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_RERUN)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --auto-approve --terragrunt-no-auto-retry --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.NotNil(t, err)
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryExhaustRetries(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_EXHAUST)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_EXHAUST)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.NotNil(t, err)
	assert.Contains(t, out.String(), "Failed to load backend")
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryFlagWithRecoverableError(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_RERUN)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_RERUN)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --auto-approve --terragrunt-no-auto-retry --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.NotNil(t, err)
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryEnvVarWithRecoverableError(t *testing.T) {
	os.Setenv("TERRAGRUNT_AUTO_RETRY", "false")
	defer os.Unsetenv("TERRAGRUNT_AUTO_RETRY")
	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_RERUN)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_RERUN)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.NotNil(t, err)
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryApplyAllDependentModuleRetries(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_APPLY_ALL_RETRIES)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_APPLY_ALL_RETRIES)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.Nil(t, err)
	s := out.String()
	assert.Contains(t, s, "app1 output")
	assert.Contains(t, s, "app2 output")
	assert.Contains(t, s, "app3 output")
	assert.Contains(t, s, "Apply complete!")

}

// This tests terragrunt properly passes through terraform commands and any number of specified args
func TestTerraformCommandCliArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		command  []string
		expected string
	}{
		{
			[]string{"version"},
			"terraform version",
		},
		{
			[]string{"version", "foo"},
			"terraform version foo",
		},
		{
			[]string{"version", "foo", "bar", "baz"},
			"terraform version foo bar baz",
		},
		{
			[]string{"version", "foo", "bar", "baz", "foobar"},
			"terraform version foo bar baz foobar",
		},
	}

	for _, testCase := range testCases {
		cmd := fmt.Sprintf("terragrunt %s --terragrunt-non-interactive --terragrunt-working-dir %s", strings.Join(testCase.command, " "), TEST_FIXTURE_EXTRA_ARGS_PATH)

		var (
			stdout bytes.Buffer
			stderr bytes.Buffer
		)

		runTerragruntRedirectOutput(t, cmd, &stdout, &stderr)
		output := stdout.String()
		errOutput := stderr.String()
		assert.True(t, strings.Contains(errOutput, testCase.expected) || strings.Contains(output, testCase.expected))
	}
}

// This tests terragrunt properly passes through terraform commands with sub commands
// and any number of specified args
func TestTerraformSubcommandCliArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		command  []string
		expected string
	}{
		{
			[]string{"force-unlock"},
			"terraform force-unlock",
		},
		{
			[]string{"force-unlock", "foo"},
			"terraform force-unlock foo",
		},
		{
			[]string{"force-unlock", "foo", "bar", "baz"},
			"terraform force-unlock foo bar baz",
		},
		{
			[]string{"force-unlock", "foo", "bar", "baz", "foobar"},
			"terraform force-unlock foo bar baz foobar",
		},
	}

	for _, testCase := range testCases {
		cmd := fmt.Sprintf("terragrunt %s --terragrunt-non-interactive --terragrunt-working-dir %s", strings.Join(testCase.command, " "), TEST_FIXTURE_EXTRA_ARGS_PATH)

		var (
			stdout bytes.Buffer
			stderr bytes.Buffer
		)
		// Call runTerragruntCommand directly because this command contains failures (which causes runTerragruntRedirectOutput to abort) but we don't care.
		if err := runTerragruntCommand(t, cmd, &stdout, &stderr); err == nil {
			t.Fatalf("Failed to properly fail command: %v.", cmd)
		}
		output := stdout.String()
		errOutput := stderr.String()
		assert.True(t, strings.Contains(errOutput, testCase.expected) || strings.Contains(output, testCase.expected))
	}
}

func TestPreventDestroy(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_PREVENT_DESTROY)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_PREVENT_DESTROY))

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt destroy --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_PREVENT_DESTROY), os.Stdout, os.Stderr)

	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, cli.ModuleIsProtected{}, underlying)
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
		modulePaths[moduleName] = util.JoinPath(TEST_FIXTURE_LOCAL_PREVENT_DESTROY_DEPENDENCIES, moduleName)
	}

	// Cleanup all modules directories.
	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_PREVENT_DESTROY_DEPENDENCIES)
	for _, modulePath := range modulePaths {
		cleanupTerraformFolder(t, modulePath)
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	// Apply and destroy all modules.
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_PREVENT_DESTROY_DEPENDENCIES), &applyAllStdout, &applyAllStderr)
	logBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
	logBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")

	if err != nil {
		t.Fatalf("apply-all in TestPreventDestroyDependencies failed with error: %v. Full std", err)
	}

	var (
		destroyAllStdout bytes.Buffer
		destroyAllStderr bytes.Buffer
	)

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt destroy-all --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_PREVENT_DESTROY_DEPENDENCIES), &destroyAllStdout, &destroyAllStderr)
	logBufferContentsLineByLine(t, destroyAllStdout, "destroy-all stdout")
	logBufferContentsLineByLine(t, destroyAllStderr, "destroy-all stderr")

	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, configstack.MultiError{}, underlying)
	}

	// Check that modules C, D and E were deleted and modules A and B weren't.
	for moduleName, modulePath := range modulePaths {
		var (
			showStdout bytes.Buffer
			showStderr bytes.Buffer
		)

		err = runTerragruntCommand(t, fmt.Sprintf("terragrunt show --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), &showStdout, &showStderr)
		logBufferContentsLineByLine(t, showStdout, fmt.Sprintf("show stdout for %s", modulePath))
		logBufferContentsLineByLine(t, showStderr, fmt.Sprintf("show stderr for %s", modulePath))

		assert.NoError(t, err)
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

func TestInputsPassedThroughCorrectly(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_INPUTS)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_INPUTS))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_INPUTS), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))

	assert.Equal(t, outputs["bool"].Value, true)
	assert.Equal(t, outputs["list_bool"].Value, []interface{}{true, false})
	assert.Equal(t, outputs["list_number"].Value, []interface{}{1.0, 2.0, 3.0})
	assert.Equal(t, outputs["list_string"].Value, []interface{}{"a", "b", "c"})
	assert.Equal(t, outputs["map_bool"].Value, map[string]interface{}{"foo": true, "bar": false, "baz": true})
	assert.Equal(t, outputs["map_number"].Value, map[string]interface{}{"foo": 42.0, "bar": 12345.0})
	assert.Equal(t, outputs["map_string"].Value, map[string]interface{}{"foo": "bar"})
	assert.Equal(t, outputs["number"].Value, 42.0)
	assert.Equal(t, outputs["object"].Value, map[string]interface{}{"list": []interface{}{1.0, 2.0, 3.0}, "map": map[string]interface{}{"foo": "bar"}, "num": 42.0, "str": "string"})
	assert.Equal(t, outputs["string"].Value, "string")
	assert.Equal(t, outputs["from_env"].Value, "default")
}

type TerraformOutput struct {
	Sensitive bool
	Type      interface{}
	Value     interface{}
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
		modulePaths[moduleName] = util.JoinPath(TEST_FIXTURE_LOCAL_INCLUDE_PREVENT_DESTROY_DEPENDENCIES, moduleName)
	}

	// Cleanup all modules directories.
	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_INCLUDE_PREVENT_DESTROY_DEPENDENCIES)
	for _, modulePath := range modulePaths {
		cleanupTerraformFolder(t, modulePath)
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	// Apply and destroy all modules.
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_INCLUDE_PREVENT_DESTROY_DEPENDENCIES), &applyAllStdout, &applyAllStderr)
	logBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
	logBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")

	if err != nil {
		t.Fatalf("apply-all in TestPreventDestroyDependenciesIncludedConfig failed with error: %v. Full std", err)
	}

	var (
		destroyAllStdout bytes.Buffer
		destroyAllStderr bytes.Buffer
	)

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt destroy-all --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_INCLUDE_PREVENT_DESTROY_DEPENDENCIES), &destroyAllStdout, &destroyAllStderr)
	logBufferContentsLineByLine(t, destroyAllStdout, "destroy-all stdout")
	logBufferContentsLineByLine(t, destroyAllStderr, "destroy-all stderr")

	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, configstack.MultiError{}, underlying)
	}

	// Check that modules C, D and E were deleted and modules A and B weren't.
	for moduleName, modulePath := range modulePaths {
		var (
			showStdout bytes.Buffer
			showStderr bytes.Buffer
		)

		err = runTerragruntCommand(t, fmt.Sprintf("terragrunt show --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), &showStdout, &showStderr)
		logBufferContentsLineByLine(t, showStdout, fmt.Sprintf("show stdout for %s", modulePath))
		logBufferContentsLineByLine(t, showStderr, fmt.Sprintf("show stderr for %s", modulePath))

		assert.NoError(t, err)
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
		{TEST_FIXTURE_LOCAL_WITH_EXCLUDE_DIR, "--terragrunt-exclude-dir */gce", []string{"Module GCE B", "Module GCE C", "Module GCE E"}},
		{TEST_FIXTURE_LOCAL_WITH_EXCLUDE_DIR, "--terragrunt-exclude-dir production-env --terragrunt-exclude-dir **/module-gce-c", []string{"Module GCE C", "Module AWS D", "Module GCE E"}},
		{TEST_FIXTURE_LOCAL_WITH_EXCLUDE_DIR, "--terragrunt-exclude-dir integration-env/gce/module-gce-b --terragrunt-exclude-dir integration-env/gce/module-gce-c --terragrunt-exclude-dir **/module-aws*", []string{"Module AWS A", "Module GCE B", "Module GCE C", "Module AWS D"}},
	}

	modulePaths := make(map[string]string, len(moduleNames))
	for _, moduleName := range moduleNames {
		modulePaths[moduleName] = util.JoinPath(TEST_FIXTURE_LOCAL_WITH_EXCLUDE_DIR, moduleName)
	}

	for _, testCase := range testCases {
		applyAllStdout := bytes.Buffer{}
		applyAllStderr := bytes.Buffer{}

		// Cleanup all modules directories.
		cleanupTerragruntFolder(t, TEST_FIXTURE_LOCAL_WITH_EXCLUDE_DIR)
		for _, modulePath := range modulePaths {
			cleanupTerragruntFolder(t, modulePath)
		}

		// Apply modules according to test cases
		err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s %s", testCase.workingDir, testCase.excludeArgs), &applyAllStdout, &applyAllStderr)

		logBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
		logBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")

		if err != nil {
			t.Fatalf("apply-all in TestExcludeDirs failed with error: %v. Full std", err)
		}

		// Check that the excluded module output is not present
		for _, modulePath := range modulePaths {
			showStdout := bytes.Buffer{}
			showStderr := bytes.Buffer{}

			err = runTerragruntCommand(t, fmt.Sprintf("terragrunt show --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), &showStdout, &showStderr)
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
		{TEST_FIXTURE_LOCAL_WITH_INCLUDE_DIR, "--terragrunt-include-dir */aws", []string{"Module GCE B", "Module GCE C", "Module GCE E"}},
		{TEST_FIXTURE_LOCAL_WITH_INCLUDE_DIR, "--terragrunt-include-dir production-env --terragrunt-include-dir **/module-gce-c", []string{"Module GCE B", "Module AWS A"}},
		{TEST_FIXTURE_LOCAL_WITH_INCLUDE_DIR, "--terragrunt-include-dir integration-env/gce/module-gce-b --terragrunt-include-dir integration-env/gce/module-gce-c --terragrunt-include-dir **/module-aws*", []string{"Module GCE E"}},
	}

	modulePaths := make(map[string]string, len(moduleNames))
	for _, moduleName := range moduleNames {
		modulePaths[moduleName] = util.JoinPath(TEST_FIXTURE_LOCAL_WITH_INCLUDE_DIR, moduleName)
	}

	for _, testCase := range testCases {
		applyAllStdout := bytes.Buffer{}
		applyAllStderr := bytes.Buffer{}

		// Cleanup all modules directories.
		cleanupTerragruntFolder(t, TEST_FIXTURE_LOCAL_WITH_INCLUDE_DIR)
		for _, modulePath := range modulePaths {
			cleanupTerragruntFolder(t, modulePath)
		}

		// Apply modules according to test cases
		err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s %s", testCase.workingDir, testCase.includeArgs), &applyAllStdout, &applyAllStderr)

		logBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
		logBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")

		if err != nil {
			t.Fatalf("apply-all in TestExcludeDirs failed with error: %v. Full std", err)
		}

		// Check that the included module output is present
		for _, modulePath := range modulePaths {
			showStdout := bytes.Buffer{}
			showStderr := bytes.Buffer{}

			err = runTerragruntCommand(t, fmt.Sprintf("terragrunt show --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), &showStdout, &showStderr)
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

func TestTerragruntExternalDependencies(t *testing.T) {
	t.Parallel()

	t.Skip("Skipping for now as --terragrunt-non-interactive no longer automatically applies external dependencies. In the future, we should add a specific flag to control that behavior.")

	modules := []string{
		"module-a",
		"module-b",
	}

	cleanupTerraformFolder(t, TEST_FIXTURE_EXTERNAL_DEPENDENCIE)
	for _, module := range modules {
		cleanupTerraformFolder(t, util.JoinPath(TEST_FIXTURE_EXTERNAL_DEPENDENCIE, module))
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	rootPath := copyEnvironment(t, TEST_FIXTURE_EXTERNAL_DEPENDENCIE)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_EXTERNAL_DEPENDENCIE, "module-b")

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), &applyAllStdout, &applyAllStderr)
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

func TestTerragruntExcludeExternalDependencies(t *testing.T) {
	t.Parallel()

	excludedModule := "module-a"
	includedModule := "module-b"

	modules := []string{
		excludedModule,
		includedModule,
	}

	cleanupTerraformFolder(t, TEST_FIXTURE_EXTERNAL_DEPENDENCIE)
	for _, module := range modules {
		cleanupTerraformFolder(t, util.JoinPath(TEST_FIXTURE_EXTERNAL_DEPENDENCIE, module))
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	rootPath := copyEnvironment(t, TEST_FIXTURE_EXTERNAL_DEPENDENCIE)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_EXTERNAL_DEPENDENCIE, includedModule)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-ignore-external-dependencies --terragrunt-working-dir %s", modulePath), &applyAllStdout, &applyAllStderr)
	logBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
	logBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")
	applyAllStdoutString := applyAllStdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Contains(t, applyAllStdoutString, fmt.Sprintf("Hello World, %s", includedModule))
	assert.NotContains(t, applyAllStdoutString, fmt.Sprintf("Hello World, %s", excludedModule))
}

func TestApplySkipTrue(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, TEST_FIXTURE_SKIP)
	rootPath = util.JoinPath(rootPath, TEST_FIXTURE_SKIP, "skip-true")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s --var person=Hobbs", rootPath), &showStdout, &showStderr)
	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	stdout := showStdout.String()
	stderr := showStderr.String()

	assert.Nil(t, err)
	assert.Regexp(t, regexp.MustCompile("Skipping terragrunt module .*fixture-skip/skip-true/terragrunt.hcl due to skip = true."), stderr)
	assert.NotContains(t, stdout, "hello, Hobbs")
}

func TestApplySkipFalse(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, TEST_FIXTURE_SKIP)
	rootPath = util.JoinPath(rootPath, TEST_FIXTURE_SKIP, "skip-false")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &showStdout, &showStderr)
	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	stderr := showStderr.String()
	stdout := showStdout.String()

	assert.Nil(t, err)
	assert.Contains(t, stdout, "hello, Hobbs")
	assert.NotContains(t, stderr, "Skipping terragrunt module")
}

func TestApplyAllSkipTrue(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, TEST_FIXTURE_SKIP)
	rootPath = util.JoinPath(rootPath, TEST_FIXTURE_SKIP, "skip-true")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &showStdout, &showStderr)
	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	stdout := showStdout.String()
	stderr := showStderr.String()

	assert.Nil(t, err)
	assert.Regexp(t, regexp.MustCompile("Skipping terragrunt module .*fixture-skip/skip-true/terragrunt.hcl due to skip = true."), stderr)
	assert.Contains(t, stdout, "hello, Ernie")
	assert.Contains(t, stdout, "hello, Bert")
}

func TestApplyAllSkipFalse(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, TEST_FIXTURE_SKIP)
	rootPath = util.JoinPath(rootPath, TEST_FIXTURE_SKIP, "skip-false")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &showStdout, &showStderr)
	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	stdout := showStdout.String()
	stderr := showStderr.String()

	assert.Nil(t, err)
	assert.Contains(t, stdout, "hello, Hobbs")
	assert.Contains(t, stdout, "hello, Ernie")
	assert.Contains(t, stdout, "hello, Bert")
	assert.NotContains(t, stderr, "Skipping terragrunt module")
}

func TestTerragruntInfo(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND)
	tmpEnvPath := copyEnvironment(t, "fixture-hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND)

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt terragrunt-info --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &showStdout, &showStderr)
	assert.Nil(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")

	var dat cli.TerragruntInfoGroup
	err_unmarshal := json.Unmarshal(showStdout.Bytes(), &dat)
	assert.Nil(t, err_unmarshal)

	assert.Equal(t, dat.DownloadDir, fmt.Sprintf("%s/%s", rootPath, TERRAGRUNT_CACHE))
	assert.Equal(t, dat.TerraformBinary, TERRAFORM_BINARY)
	assert.Equal(t, dat.IamRole, "")
}

func logBufferContentsLineByLine(t *testing.T, out bytes.Buffer, label string) {
	t.Logf("[%s] Full contents of %s:", t.Name(), label)
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		t.Logf("[%s] %s", t.Name(), line)
	}
}

func cleanupTerraformFolder(t *testing.T, templatesPath string) {
	removeFile(t, util.JoinPath(templatesPath, TERRAFORM_STATE))
	removeFile(t, util.JoinPath(templatesPath, TERRAFORM_STATE_BACKUP))
	removeFolder(t, util.JoinPath(templatesPath, TERRAFORM_FOLDER))
}

func cleanupTerragruntFolder(t *testing.T, templatesPath string) {
	removeFolder(t, util.JoinPath(templatesPath, TERRAGRUNT_CACHE))
}

func removeFile(t *testing.T, path string) {
	if util.FileExists(path) {
		if err := os.Remove(path); err != nil {
			t.Fatalf("Error while removing %s: %v", path, err)
		}
	}
}

func removeFolder(t *testing.T, path string) {
	if util.FileExists(path) {
		if err := os.RemoveAll(path); err != nil {
			t.Fatalf("Error while removing %s: %v", path, err)
		}
	}
}

func runTerragruntCommand(t *testing.T, command string, writer io.Writer, errwriter io.Writer) error {
	args := strings.Split(command, " ")

	app := cli.CreateTerragruntCli("TEST", writer, errwriter)
	return app.Run(args)
}

func runTerragrunt(t *testing.T, command string) {
	runTerragruntRedirectOutput(t, command, os.Stdout, os.Stderr)
}

func runTerragruntRedirectOutput(t *testing.T, command string, writer io.Writer, errwriter io.Writer) {
	if err := runTerragruntCommand(t, command, writer, errwriter); err != nil {
		stdout := "(see log output above)"
		if stdoutAsBuffer, stdoutIsBuffer := writer.(*bytes.Buffer); stdoutIsBuffer {
			stdout = stdoutAsBuffer.String()
		}

		stderr := "(see log output above)"
		if stderrAsBuffer, stderrIsBuffer := errwriter.(*bytes.Buffer); stderrIsBuffer {
			stderr = stderrAsBuffer.String()
		}

		t.Fatalf("Failed to run Terragrunt command '%s' due to error: %s\n\nStdout: %s\n\nStderr: %s", command, err, stdout, stderr)
	}
}

func copyEnvironment(t *testing.T, environmentPath string) string {
	tmpDir, err := ioutil.TempDir("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	t.Logf("Copying %s to %s", environmentPath, tmpDir)

	require.NoError(t, util.CopyFolderContents(environmentPath, util.JoinPath(tmpDir, environmentPath), ".terragrunt-test"))

	return tmpDir
}

func copyFile(srcPath string, destPath string) error {
	contents, err := ioutil.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(destPath, contents, 0644)
}

func createTmpTerragruntConfigWithParentAndChild(t *testing.T, parentPath string, childRelPath string, s3BucketName string, parentConfigFileName string, childConfigFileName string) string {
	tmpDir, err := ioutil.TempDir("", "terragrunt-parent-child-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	childDestPath := util.JoinPath(tmpDir, childRelPath)

	if err := os.MkdirAll(childDestPath, 0777); err != nil {
		t.Fatalf("Failed to create temp dir %s due to error %v", childDestPath, err)
	}

	parentTerragruntSrcPath := util.JoinPath(parentPath, parentConfigFileName)
	parentTerragruntDestPath := util.JoinPath(tmpDir, parentConfigFileName)
	copyTerragruntConfigAndFillPlaceholders(t, parentTerragruntSrcPath, parentTerragruntDestPath, s3BucketName, "not-used", "not-used")

	childTerragruntSrcPath := util.JoinPath(util.JoinPath(parentPath, childRelPath), childConfigFileName)
	childTerragruntDestPath := util.JoinPath(childDestPath, childConfigFileName)
	copyTerragruntConfigAndFillPlaceholders(t, childTerragruntSrcPath, childTerragruntDestPath, s3BucketName, "not-used", "not-used")

	return childTerragruntDestPath
}

func createTmpTerragruntConfig(t *testing.T, templatesPath string, s3BucketName string, lockTableName string, configFileName string) string {
	tmpFolder, err := ioutil.TempDir("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp folder due to error: %v", err)
	}

	tmpTerragruntConfigFile := util.JoinPath(tmpFolder, configFileName)
	originalTerragruntConfigPath := util.JoinPath(templatesPath, configFileName)
	copyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "not-used")

	return tmpTerragruntConfigFile
}

func createTmpTerragruntGCSConfig(t *testing.T, templatesPath string, project string, location string, gcsBucketName string, configFileName string) string {
	tmpFolder, err := ioutil.TempDir("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp folder due to error: %v", err)
	}

	tmpTerragruntConfigFile := util.JoinPath(tmpFolder, configFileName)
	originalTerragruntConfigPath := util.JoinPath(templatesPath, configFileName)
	copyTerragruntGCSConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, project, location, gcsBucketName)

	return tmpTerragruntConfigFile
}

func copyTerragruntConfigAndFillPlaceholders(t *testing.T, configSrcPath string, configDestPath string, s3BucketName string, lockTableName string, region string) {
	contents, err := util.ReadFileAsString(configSrcPath)
	if err != nil {
		t.Fatalf("Error reading Terragrunt config at %s: %v", configSrcPath, err)
	}

	contents = strings.Replace(contents, "__FILL_IN_BUCKET_NAME__", s3BucketName, -1)
	contents = strings.Replace(contents, "__FILL_IN_LOCK_TABLE_NAME__", lockTableName, -1)
	contents = strings.Replace(contents, "__FILL_IN_REGION__", region, -1)

	if err := ioutil.WriteFile(configDestPath, []byte(contents), 0444); err != nil {
		t.Fatalf("Error writing temp Terragrunt config to %s: %v", configDestPath, err)
	}
}

func copyTerragruntGCSConfigAndFillPlaceholders(t *testing.T, configSrcPath string, configDestPath string, project string, location string, gcsBucketName string) {
	contents, err := util.ReadFileAsString(configSrcPath)
	if err != nil {
		t.Fatalf("Error reading Terragrunt config at %s: %v", configSrcPath, err)
	}

	contents = strings.Replace(contents, "__FILL_IN_PROJECT__", project, -1)
	contents = strings.Replace(contents, "__FILL_IN_LOCATION__", location, -1)
	contents = strings.Replace(contents, "__FILL_IN_BUCKET_NAME__", gcsBucketName, -1)

	if err := ioutil.WriteFile(configDestPath, []byte(contents), 0444); err != nil {
		t.Fatalf("Error writing temp Terragrunt config to %s: %v", configDestPath, err)
	}
}

// Returns a unique (ish) id we can attach to resources and tfstate files so they don't conflict with each other
// Uses base 62 to generate a 6 character string that's unlikely to collide with the handful of tests we run in
// parallel. Based on code here: http://stackoverflow.com/a/9543797/483528
func uniqueId() string {
	const BASE_62_CHARS = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const UNIQUE_ID_LENGTH = 6 // Should be good for 62^6 = 56+ billion combinations

	var out bytes.Buffer

	for i := 0; i < UNIQUE_ID_LENGTH; i++ {
		out.WriteByte(BASE_62_CHARS[rand.Intn(len(BASE_62_CHARS))])
	}

	return out.String()
}

// Check that the S3 Bucket of the given name and region exists. Terragrunt should create this bucket during the test.
// Also check if bucket got tagged properly and that public access is disabled completely.
func validateS3BucketExistsAndIsTagged(t *testing.T, awsRegion string, bucketName string, expectedTags map[string]string) {
	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		t.Fatalf("Error creating mockOptions: %v", err)
	}

	sessionConfig := &aws_helper.AwsSessionConfig{
		Region: awsRegion,
	}

	s3Client, err := remote.CreateS3Client(sessionConfig, mockOptions)
	if err != nil {
		t.Fatalf("Error creating S3 client: %v", err)
	}

	remoteStateConfig := remote.RemoteStateConfigS3{Bucket: bucketName, Region: awsRegion}
	assert.True(t, remote.DoesS3BucketExist(s3Client, &remoteStateConfig), "Terragrunt failed to create remote state S3 bucket %s", bucketName)

	if expectedTags != nil {
		assertS3Tags(expectedTags, bucketName, s3Client, t)
	}

	assertS3PublicAccessBlocks(t, s3Client, bucketName)
}

// Check that the DynamoDB table of the given name and region exists. Terragrunt should create this table during the test.
// Also check if table got tagged properly
func validateDynamoDBTableExistsAndIsTagged(t *testing.T, awsRegion string, tableName string, expectedTags map[string]string) {
	client := createDynamoDbClientForTest(t, awsRegion)

	var description, err = client.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(tableName)})

	if err != nil {
		// This is a ResourceNotFoundException in case the table does not exist
		t.Fatal(err)
	}

	var tags, err2 = client.ListTagsOfResource(&dynamodb.ListTagsOfResourceInput{ResourceArn: description.Table.TableArn})

	if err2 != nil {
		t.Fatal(err2)
	}

	var actualTags = make(map[string]string)

	for _, element := range tags.Tags {
		actualTags[*element.Key] = *element.Value
	}

	assert.Equal(t, expectedTags, actualTags, "Did not find expected tags on dynamo table.")
}

func assertS3Tags(expectedTags map[string]string, bucketName string, client *s3.S3, t *testing.T) {

	var in = s3.GetBucketTaggingInput{}
	in.SetBucket(bucketName)

	var tags, err2 = client.GetBucketTagging(&in)

	if err2 != nil {
		t.Fatal(err2)
	}

	var actualTags = make(map[string]string)

	for _, element := range tags.TagSet {
		actualTags[*element.Key] = *element.Value
	}

	assert.Equal(t, expectedTags, actualTags, "Did not find expected tags on s3 bucket.")
}

func assertS3PublicAccessBlocks(t *testing.T, client *s3.S3, bucketName string) {
	resp, err := client.GetPublicAccessBlock(
		&s3.GetPublicAccessBlockInput{Bucket: aws.String(bucketName)},
	)
	require.NoError(t, err)

	publicAccessBlockConfig := resp.PublicAccessBlockConfiguration
	assert.True(t, aws.BoolValue(publicAccessBlockConfig.BlockPublicAcls))
	assert.True(t, aws.BoolValue(publicAccessBlockConfig.BlockPublicPolicy))
	assert.True(t, aws.BoolValue(publicAccessBlockConfig.IgnorePublicAcls))
	assert.True(t, aws.BoolValue(publicAccessBlockConfig.RestrictPublicBuckets))
}

// Delete the specified S3 bucket to clean up after a test
func deleteS3Bucket(t *testing.T, awsRegion string, bucketName string) {
	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		t.Fatalf("Error creating mockOptions: %v", err)
	}

	sessionConfig := &aws_helper.AwsSessionConfig{
		Region: awsRegion,
	}

	s3Client, err := remote.CreateS3Client(sessionConfig, mockOptions)
	if err != nil {
		t.Fatalf("Error creating S3 client: %v", err)
	}

	t.Logf("Deleting test s3 bucket %s", bucketName)

	out, err := s3Client.ListObjectVersions(&s3.ListObjectVersionsInput{Bucket: aws.String(bucketName)})
	if err != nil {
		t.Fatalf("Failed to list object versions in s3 bucket %s: %v", bucketName, err)
	}

	objectIdentifiers := []*s3.ObjectIdentifier{}
	for _, version := range out.Versions {
		objectIdentifiers = append(objectIdentifiers, &s3.ObjectIdentifier{
			Key:       version.Key,
			VersionId: version.VersionId,
		})
	}

	if len(objectIdentifiers) > 0 {
		deleteInput := &s3.DeleteObjectsInput{
			Bucket: aws.String(bucketName),
			Delete: &s3.Delete{Objects: objectIdentifiers},
		}
		if _, err := s3Client.DeleteObjects(deleteInput); err != nil {
			t.Fatalf("Error deleting all versions of all objects in bucket %s: %v", bucketName, err)
		}
	}

	if _, err := s3Client.DeleteBucket(&s3.DeleteBucketInput{Bucket: aws.String(bucketName)}); err != nil {
		t.Fatalf("Failed to delete S3 bucket %s: %v", bucketName, err)
	}
}

// Create an authenticated client for DynamoDB
func createDynamoDbClient(awsRegion, awsProfile string, iamRoleArn string) (*dynamodb.DynamoDB, error) {
	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		return nil, err
	}

	sessionConfig := &aws_helper.AwsSessionConfig{
		Region:  awsRegion,
		Profile: awsProfile,
		RoleArn: iamRoleArn,
	}

	session, err := aws_helper.CreateAwsSession(sessionConfig, mockOptions)
	if err != nil {
		return nil, err
	}

	return dynamodb.New(session), nil
}

func createDynamoDbClientForTest(t *testing.T, awsRegion string) *dynamodb.DynamoDB {
	client, err := createDynamoDbClient(awsRegion, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func cleanupTableForTest(t *testing.T, tableName string, awsRegion string) {
	client := createDynamoDbClientForTest(t, awsRegion)
	err := terragruntDynamoDb.DeleteTable(tableName, client)
	assert.Nil(t, err, "Unexpected error: %v", err)
}

// Check that the GCS Bucket of the given name and location exists. Terragrunt should create this bucket during the test.
// Also check if bucket got labeled properly.
func validateGCSBucketExistsAndIsLabeled(t *testing.T, location string, bucketName string, expectedLabels map[string]string) {
	gcsClient, err := remote.CreateGCSClient()
	if err != nil {
		t.Fatalf("Error creating GCS client: %v", err)
	}

	remoteStateConfig := remote.RemoteStateConfigGCS{Bucket: bucketName}
	assert.True(t, remote.DoesGCSBucketExist(gcsClient, &remoteStateConfig), "Terragrunt failed to create remote state GCS bucket %s", bucketName)

	// verify the bucket location
	ctx := context.Background()
	bucket := gcsClient.Bucket(bucketName)

	attrs, err := bucket.Attrs(ctx)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, strings.ToUpper(location), attrs.Location, "Did not find GCS bucket in expected location.")

	if expectedLabels != nil {
		assertGCSLabels(t, expectedLabels, bucketName, gcsClient)
	}
}

func assertGCSLabels(t *testing.T, expectedLabels map[string]string, bucketName string, client *storage.Client) {
	ctx := context.Background()
	bucket := client.Bucket(bucketName)

	attrs, err := bucket.Attrs(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var actualLabels = make(map[string]string)

	for key, value := range attrs.Labels {
		actualLabels[key] = value
	}

	assert.Equal(t, expectedLabels, actualLabels, "Did not find expected labels on GCS bucket.")
}

// Create the specified GCS bucket
func createGCSBucket(t *testing.T, projectID string, location string, bucketName string) {
	gcsClient, err := remote.CreateGCSClient()
	if err != nil {
		t.Fatalf("Error creating GCS client: %v", err)
	}

	t.Logf("Creating test GCS bucket %s in project %s, location %s", bucketName, projectID, location)

	ctx := context.Background()
	bucket := gcsClient.Bucket(bucketName)

	bucketAttrs := &storage.BucketAttrs{
		Location:          location,
		VersioningEnabled: true,
	}

	if err := bucket.Create(ctx, projectID, bucketAttrs); err != nil {
		t.Fatalf("Failed to create GCS bucket %s: %v", bucketName, err)
	}
}

// Delete the specified GCS bucket to clean up after a test
func deleteGCSBucket(t *testing.T, bucketName string) {
	gcsClient, err := remote.CreateGCSClient()
	if err != nil {
		t.Fatalf("Error creating GCS client: %v", err)
	}

	t.Logf("Deleting test GCS bucket %s", bucketName)

	ctx := context.Background()

	// List all objects including their versions in the bucket
	bucket := gcsClient.Bucket(bucketName)
	q := &storage.Query{
		Versions: true,
	}
	it := bucket.Objects(ctx, q)
	for {
		objectAttrs, err := it.Next()

		if err == iterator.Done {
			break
		}

		if err != nil {
			t.Fatalf("Failed to list objects and versions in GCS bucket %s: %v", bucketName, err)
		}

		// purge the object version
		if err := bucket.Object(objectAttrs.Name).Generation(objectAttrs.Generation).Delete(ctx); err != nil {
			t.Fatalf("Failed to delete GCS bucket object %s: %v", objectAttrs.Name, err)
		}
	}

	// remote empty bucket
	if err := bucket.Delete(ctx); err != nil {
		t.Fatalf("Failed to delete GCS bucket %s: %v", bucketName, err)
	}
}
