package test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/config"
	terragruntDynamoDb "github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
)

// hard-code this to match the test fixture for now
const (
	TERRAFORM_REMOTE_STATE_S3_REGION                    = "us-west-2"
	TEST_FIXTURE_PATH                                   = "fixture/"
	TEST_FIXTURE_INCLUDE_PATH                           = "fixture-include/"
	TEST_FIXTURE_INCLUDE_CHILD_REL_PATH                 = "qa/my-app"
	TEST_FIXTURE_STACK                                  = "fixture-stack/"
	TEST_FIXTURE_OUTPUT_ALL                             = "fixture-output-all"
	TEST_FIXTURE_STDOUT                                 = "fixture-download/stdout-test"
	TEST_FIXTURE_EXTRA_ARGS_PATH                        = "fixture-extra-args/"
	TEST_FIXTURE_LOCAL_DOWNLOAD_PATH                    = "fixture-download/local"
	TEST_FIXTURE_REMOTE_DOWNLOAD_PATH                   = "fixture-download/remote"
	TEST_FIXTURE_OVERRIDE_DOWNLOAD_PATH                 = "fixture-download/override"
	TEST_FIXTURE_LOCAL_RELATIVE_DOWNLOAD_PATH           = "fixture-download/local-relative"
	TEST_FIXTURE_REMOTE_RELATIVE_DOWNLOAD_PATH          = "fixture-download/remote-relative"
	TEST_FIXTURE_LOCAL_WITH_BACKEND                     = "fixture-download/local-with-backend"
	TEST_FIXTURE_REMOTE_WITH_BACKEND                    = "fixture-download/remote-with-backend"
	TEST_FIXTURE_REMOTE_MODULE_IN_ROOT                  = "fixture-download/remote-module-in-root"
	TEST_FIXTURE_LOCAL_MISSING_BACKEND                  = "fixture-download/local-with-missing-backend"
	TEST_FIXTURE_LOCAL_WITH_HIDDEN_FOLDER               = "fixture-download/local-with-hidden-folder"
	TEST_FIXTURE_OLD_CONFIG_INCLUDE_PATH                = "fixture-old-terragrunt-config/include"
	TEST_FIXTURE_OLD_CONFIG_INCLUDE_CHILD_UPDATED_PATH  = "fixture-old-terragrunt-config/include-child-updated"
	TEST_FIXTURE_OLD_CONFIG_INCLUDE_PARENT_UPDATED_PATH = "fixture-old-terragrunt-config/include-parent-updated"
	TEST_FIXTURE_OLD_CONFIG_STACK_PATH                  = "fixture-old-terragrunt-config/stack"
	TEST_FIXTURE_OLD_CONFIG_DOWNLOAD_PATH               = "fixture-old-terragrunt-config/download"
	TEST_FIXTURE_HOOKS_BEFORE_ONLY_PATH                 = "fixture-hooks/before-only"
	TEST_FIXTURE_HOOKS_AFTER_ONLY_PATH                  = "fixture-hooks/after-only"
	TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_PATH            = "fixture-hooks/before-and-after"
	TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_MERGE_PATH      = "fixture-hooks/before-and-after-merge"
	TEST_FIXTURE_HOOKS_SKIP_ON_ERROR_PATH               = "fixture-hooks/skip-on-error"
	TEST_FIXTURE_HOOKS_ONE_ARG_ACTION_PATH              = "fixture-hooks/one-arg-action"
	TEST_FIXTURE_HOOKS_EMPTY_STRING_COMMAND_PATH        = "fixture-hooks/bad-arg-action/empty-string-command"
	TEST_FIXTURE_HOOKS_EMPTY_COMMAND_LIST_PATH          = "fixture-hooks/bad-arg-action/empty-command-list"
	TEST_FIXTURE_HOOKS_INTERPOLATIONS_PATH              = "fixture-hooks/interpolations"
	TEST_FIXTURE_FAILED_TERRAFORM                       = "fixture-failure"
	TEST_FIXTURE_EXIT_CODE                              = "fixture-exit-code"
	TERRAFORM_FOLDER                                    = ".terraform"
	TERRAFORM_STATE                                     = "terraform.tfstate"
	TERRAFORM_STATE_BACKUP                              = "terraform.tfstate.backup"
	TEST_FIXTURE_INIT_ERROR                             = "fixture-retryableerrors/retry-init"
)

func init() {
	rand.Seed(time.Now().UnixNano())
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

func TestTerragruntWorksWithIncludes(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(TEST_FIXTURE_INCLUDE_PATH, TEST_FIXTURE_INCLUDE_CHILD_REL_PATH)
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, TEST_FIXTURE_INCLUDE_PATH, TEST_FIXTURE_INCLUDE_CHILD_REL_PATH, s3BucketName, config.DefaultTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
}

func TestTerragruntWorksWithIncludesAndOldConfig(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(TEST_FIXTURE_OLD_CONFIG_INCLUDE_PATH, "child")
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, TEST_FIXTURE_OLD_CONFIG_INCLUDE_PATH, "child", s3BucketName, config.OldTerragruntConfigPath, config.OldTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
}

func TestTerragruntWorksWithIncludesChildUpdatedAndOldConfig(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(TEST_FIXTURE_OLD_CONFIG_INCLUDE_CHILD_UPDATED_PATH, "child")
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, TEST_FIXTURE_OLD_CONFIG_INCLUDE_CHILD_UPDATED_PATH, "child", s3BucketName, config.OldTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
}

func TestTerragruntWorksWithIncludesParentUpdatedAndOldConfig(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(TEST_FIXTURE_OLD_CONFIG_INCLUDE_PARENT_UPDATED_PATH, "child")
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, TEST_FIXTURE_OLD_CONFIG_INCLUDE_PARENT_UPDATED_PATH, "child", s3BucketName, config.DefaultTerragruntConfigPath, config.OldTerragruntConfigPath)

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
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s -var terraform_remote_state_s3_bucket=\"%s\"", environmentPath, s3BucketName))

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
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL)

	runTerragrunt(t, fmt.Sprintf("terragrunt validate-all --terragrunt-non-interactive --terragrunt-working-dir %s -var terraform_remote_state_s3_bucket=\"%s\"", environmentPath, s3BucketName))
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
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s -var terraform_remote_state_s3_bucket=\"%s\"", environmentPath, s3BucketName))

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
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName)

	mgmtEnvironmentPath := fmt.Sprintf("%s/fixture-stack/mgmt", tmpEnvPath)
	stageEnvironmentPath := fmt.Sprintf("%s/fixture-stack/stage", tmpEnvPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s -var terraform_remote_state_s3_bucket=\"%s\"", mgmtEnvironmentPath, s3BucketName))
	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s -var terraform_remote_state_s3_bucket=\"%s\"", stageEnvironmentPath, s3BucketName))

	runTerragrunt(t, fmt.Sprintf("terragrunt output-all --terragrunt-non-interactive --terragrunt-working-dir %s", mgmtEnvironmentPath))
	runTerragrunt(t, fmt.Sprintf("terragrunt output-all --terragrunt-non-interactive --terragrunt-working-dir %s", stageEnvironmentPath))

	runTerragrunt(t, fmt.Sprintf("terragrunt destroy-all --terragrunt-non-interactive --terragrunt-working-dir %s -var terraform_remote_state_s3_bucket=\"%s\"", stageEnvironmentPath, s3BucketName))
	runTerragrunt(t, fmt.Sprintf("terragrunt destroy-all --terragrunt-non-interactive --terragrunt-working-dir %s -var terraform_remote_state_s3_bucket=\"%s\"", mgmtEnvironmentPath, s3BucketName))
}

func TestTerragruntStackCommandsWithOldConfig(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_OLD_CONFIG_STACK_PATH)

	rootPath := util.JoinPath(tmpEnvPath, "fixture-old-terragrunt-config/stack")
	stagePath := util.JoinPath(rootPath, "stage")
	rootTerragruntConfigPath := util.JoinPath(rootPath, config.OldTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s -var terraform_remote_state_s3_bucket=\"%s\"", stagePath, s3BucketName))
	runTerragrunt(t, fmt.Sprintf("terragrunt output-all --terragrunt-non-interactive --terragrunt-working-dir %s", stagePath))
	runTerragrunt(t, fmt.Sprintf("terragrunt destroy-all --terragrunt-non-interactive --terragrunt-working-dir %s -var terraform_remote_state_s3_bucket=\"%s\"", stagePath, s3BucketName))
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

func TestLocalDownloadWithOldConfig(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_OLD_CONFIG_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_OLD_CONFIG_DOWNLOAD_PATH))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_OLD_CONFIG_DOWNLOAD_PATH))
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
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
}

func TestLocalWithMissingBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-lock-table-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpEnvPath := copyEnvironment(t, "fixture-download")
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_LOCAL_MISSING_BACKEND)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName)

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
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName)

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

func TestInitRetry(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_INIT_ERROR)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_INIT_ERROR)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.Nil(t, err)
	// t.Log(out)
	assert.NotContains(t, out.String(), "may need to run 'terraform init'")
}


func cleanupTerraformFolder(t *testing.T, templatesPath string) {
	removeFile(t, util.JoinPath(templatesPath, TERRAFORM_STATE))
	removeFile(t, util.JoinPath(templatesPath, TERRAFORM_STATE_BACKUP))
	removeFolder(t, util.JoinPath(templatesPath, TERRAFORM_FOLDER))
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
		t.Fatalf("Failed to run Terragrunt command '%s' due to error: %s", command, err)
	}
}

func copyEnvironment(t *testing.T, environmentPath string) string {
	tmpDir, err := ioutil.TempDir("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	t.Logf("Copying %s to %s", environmentPath, tmpDir)

	err = filepath.Walk(environmentPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		destPath := util.JoinPath(tmpDir, path)

		destPathDir := filepath.Dir(destPath)
		if err := os.MkdirAll(destPathDir, 0777); err != nil {
			return err
		}

		return copyFile(path, destPath)
	})

	if err != nil {
		t.Fatalf("Error walking file path %s due to error: %v", environmentPath, err)
	}

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
	copyTerragruntConfigAndFillPlaceholders(t, parentTerragruntSrcPath, parentTerragruntDestPath, s3BucketName, "not-used")

	childTerragruntSrcPath := util.JoinPath(util.JoinPath(parentPath, childRelPath), childConfigFileName)
	childTerragruntDestPath := util.JoinPath(childDestPath, childConfigFileName)
	copyTerragruntConfigAndFillPlaceholders(t, childTerragruntSrcPath, childTerragruntDestPath, s3BucketName, "not-used")

	return childTerragruntDestPath
}

func createTmpTerragruntConfig(t *testing.T, templatesPath string, s3BucketName string, lockTableName string, configFileName string) string {
	tmpFolder, err := ioutil.TempDir("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp folder due to error: %v", err)
	}

	tmpTerragruntConfigFile := util.JoinPath(tmpFolder, configFileName)
	originalTerragruntConfigPath := util.JoinPath(templatesPath, configFileName)
	copyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName)

	return tmpTerragruntConfigFile
}

func copyTerragruntConfigAndFillPlaceholders(t *testing.T, configSrcPath string, configDestPath string, s3BucketName string, lockTableName string) {
	contents, err := util.ReadFileAsString(configSrcPath)
	if err != nil {
		t.Fatalf("Error reading Terragrunt config at %s: %v", configSrcPath, err)
	}

	contents = strings.Replace(contents, "__FILL_IN_BUCKET_NAME__", s3BucketName, -1)
	contents = strings.Replace(contents, "__FILL_IN_LOCK_TABLE_NAME__", lockTableName, -1)

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
// Also check if bucket got tagged properly
func validateS3BucketExistsAndIsTagged(t *testing.T, awsRegion string, bucketName string, expectedTags map[string]string) {
	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		t.Fatalf("Error creating mockOptions: %v", err)
	}

	s3Client, err := remote.CreateS3Client(awsRegion, "", "", "", mockOptions)
	if err != nil {
		t.Fatalf("Error creating S3 client: %v", err)
	}

	remoteStateConfig := remote.RemoteStateConfigS3{Bucket: bucketName, Region: awsRegion}
	assert.True(t, remote.DoesS3BucketExist(s3Client, &remoteStateConfig), "Terragrunt failed to create remote state S3 bucket %s", bucketName)

	if expectedTags != nil {
		assertS3Tags(expectedTags, bucketName, s3Client, t)
	}
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

// Delete the specified S3 bucket to clean up after a test
func deleteS3Bucket(t *testing.T, awsRegion string, bucketName string) {
	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		t.Fatalf("Error creating mockOptions: %v", err)
	}

	s3Client, err := remote.CreateS3Client(awsRegion, "", "", "", mockOptions)
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

	session, err := aws_helper.CreateAwsSession(awsRegion, "", awsProfile, iamRoleArn, mockOptions)
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
