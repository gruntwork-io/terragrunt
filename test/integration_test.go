package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
	terraws "github.com/gruntwork-io/terratest/modules/aws"
	"github.com/gruntwork-io/terratest/modules/git"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"

	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/cli/tfsource"
	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	terragruntDynamoDb "github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

// hard-code this to match the test fixture for now
const (
	TERRAFORM_REMOTE_STATE_S3_REGION                        = "us-west-2"
	TERRAFORM_REMOTE_STATE_GCP_REGION                       = "eu"
	TEST_FIXTURE_PATH                                       = "fixture/"
	TEST_FIXTURE_CODEGEN_PATH                               = "fixture-codegen"
	TEST_FIXTURE_GCS_PATH                                   = "fixture-gcs/"
	TEST_FIXTURE_GCS_BYO_BUCKET_PATH                        = "fixture-gcs-byo-bucket/"
	TEST_FIXTURE_STACK                                      = "fixture-stack/"
	TEST_FIXTURE_GRAPH_DEPENDENCIES                         = "fixture-graph-dependencies"
	TEST_FIXTURE_OUTPUT_ALL                                 = "fixture-output-all"
	TEST_FIXTURE_STDOUT                                     = "fixture-download/stdout-test"
	TEST_FIXTURE_EXTRA_ARGS_PATH                            = "fixture-extra-args/"
	TEST_FIXTURE_ENV_VARS_BLOCK_PATH                        = "fixture-env-vars-block/"
	TEST_FIXTURE_SKIP                                       = "fixture-skip/"
	TEST_FIXTURE_CONFIG_SINGLE_JSON_PATH                    = "fixture-config-files/single-json-config"
	TEST_FIXTURE_LOCAL_DOWNLOAD_PATH                        = "fixture-download/local"
	TEST_FIXTURE_CUSTOM_LOCK_FILE                           = "fixture-download/custom-lock-file"
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
	TEST_FIXTURE_PREVENT_DESTROY_OVERRIDE                   = "fixture-prevent-destroy-override/child"
	TEST_FIXTURE_PREVENT_DESTROY_NOT_SET                    = "fixture-prevent-destroy-not-set/child"
	TEST_FIXTURE_LOCAL_PREVENT_DESTROY                      = "fixture-download/local-with-prevent-destroy"
	TEST_FIXTURE_LOCAL_PREVENT_DESTROY_DEPENDENCIES         = "fixture-download/local-with-prevent-destroy-dependencies"
	TEST_FIXTURE_LOCAL_INCLUDE_PREVENT_DESTROY_DEPENDENCIES = "fixture-download/local-include-with-prevent-destroy-dependencies"
	TEST_FIXTURE_EXTERNAL_DEPENDENCIE                       = "fixture-external-dependencies"
	TEST_FIXTURE_GET_OUTPUT                                 = "fixture-get-output"
	TEST_FIXTURE_HOOKS_BEFORE_ONLY_PATH                     = "fixture-hooks/before-only"
	TEST_FIXTURE_HOOKS_ALL_PATH                             = "fixture-hooks/all"
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
	TEST_FIXTURE_AUTO_RETRY_CUSTOM_ERRORS                   = "fixture-auto-retry/custom-errors"
	TEST_FIXTURE_AUTO_RETRY_CUSTOM_ERRORS_NOT_SET           = "fixture-auto-retry/custom-errors-not-set"
	TEST_FIXTURE_AUTO_RETRY_APPLY_ALL_RETRIES               = "fixture-auto-retry/apply-all"
	TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES            = "fixture-auto-retry/configurable-retries"
	TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES_ERROR_1    = "fixture-auto-retry/configurable-retries-incorrect-retry-attempts"
	TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES_ERROR_2    = "fixture-auto-retry/configurable-retries-incorrect-sleep-interval"
	TEST_FIXTURE_AWS_PROVIDER_PATCH                         = "fixture-aws-provider-patch"
	TEST_FIXTURE_INPUTS                                     = "fixture-inputs"
	TEST_FIXTURE_LOCALS_ERROR_UNDEFINED_LOCAL               = "fixture-locals-errors/undefined-local"
	TEST_FIXTURE_LOCALS_ERROR_UNDEFINED_LOCAL_BUT_INPUT     = "fixture-locals-errors/undefined-local-but-input"
	TEST_FIXTURE_LOCALS_CANONICAL                           = "fixture-locals/canonical"
	TEST_FIXTURE_LOCALS_IN_INCLUDE                          = "fixture-locals/local-in-include"
	TEST_FIXTURE_LOCALS_IN_INCLUDE_CHILD_REL_PATH           = "qa/my-app"
	TEST_FIXTURE_READ_CONFIG                                = "fixture-read-config"
	TEST_FIXTURE_AWS_GET_CALLER_IDENTITY                    = "fixture-get-aws-caller-identity"
	TEST_FIXTURE_GET_PLATFORM                               = "fixture-get-platform"
	TEST_FIXTURE_GET_TERRAGRUNT_SOURCE_HCL                  = "fixture-get-terragrunt-source-hcl"
	TEST_FIXTURE_GET_TERRAGRUNT_SOURCE_CLI                  = "fixture-get-terragrunt-source-cli"
	TEST_FIXTURE_REGRESSIONS                                = "fixture-regressions"
	TEST_FIXTURE_DIRS_PATH                                  = "fixture-dirs"
	TEST_FIXTURE_PARALLELISM                                = "fixture-parallelism"
	TEST_FIXTURE_SOPS                                       = "fixture-sops"
	TERRAFORM_BINARY                                        = "terraform"
	TERRAFORM_FOLDER                                        = ".terraform"
	TERRAFORM_STATE                                         = "terraform.tfstate"
	TERRAFORM_STATE_BACKUP                                  = "terraform.tfstate.backup"
	TERRAGRUNT_CACHE                                        = ".terragrunt-cache"

	qaMyAppRelPath = "qa/my-app"
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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	output := stderr.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_ONLY_ONCE"), "Hooks on init command executed more than once")
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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	output := stderr.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_ONLY_ONCE"), "Hooks on init command executed more than once")
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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level debug", rootPath), &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")
	output := stderr.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_ONLY_ONCE\n"), "Hooks on init command executed more than once")
	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE\n"), "Hooks on init-from-module command executed more than once")
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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	output := stderr.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	// `init` hook should execute only once
	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_ONLY_ONCE"), "Hooks on init command executed more than once")
	// `init-from-module` hook should execute only once
	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE"), "Hooks on init-from-module command executed more than once")
}

func TestTerragruntHookRunAllApply(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_ALL_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_ALL_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_ALL_PATH)
	beforeOnlyPath := util.JoinPath(rootPath, "before-only")
	afterOnlyPath := util.JoinPath(rootPath, "after-only")

	runTerragrunt(t, fmt.Sprintf("terragrunt run-all apply -auto-approve --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	_, beforeErr := ioutil.ReadFile(beforeOnlyPath + "/file.out")
	assert.NoError(t, beforeErr)
	_, afterErr := ioutil.ReadFile(afterOnlyPath + "/file.out")
	assert.NoError(t, afterErr)
}

func TestTerragruntHookApplyAll(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_ALL_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_ALL_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_ALL_PATH)
	beforeOnlyPath := util.JoinPath(rootPath, "before-only")
	afterOnlyPath := util.JoinPath(rootPath, "after-only")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all -auto-approve --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	_, beforeErr := ioutil.ReadFile(beforeOnlyPath + "/file.out")
	assert.NoError(t, beforeErr)
	_, afterErr := ioutil.ReadFile(afterOnlyPath + "/file.out")
	assert.NoError(t, afterErr)
}

func TestTerragruntBeforeHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_BEFORE_ONLY_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_BEFORE_ONLY_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_BEFORE_ONLY_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	_, exception := ioutil.ReadFile(rootPath + "/file.out")

	assert.NoError(t, exception)
}

func TestTerragruntHookWorkingDir(t *testing.T) {
	t.Parallel()

	fixturePath := "fixture-hooks/working_dir"
	cleanupTerraformFolder(t, fixturePath)
	tmpEnvPath := copyEnvironment(t, fixturePath)
	rootPath := util.JoinPath(tmpEnvPath, fixturePath)

	runTerragrunt(t, fmt.Sprintf("terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
}

func TestTerragruntAfterHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_AFTER_ONLY_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_AFTER_ONLY_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_AFTER_ONLY_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	_, exception := ioutil.ReadFile(rootPath + "/file.out")

	assert.NoError(t, exception)
}

func TestTerragruntBeforeAndAfterHook(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_PATH)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)

	_, beforeException := ioutil.ReadFile(rootPath + "/before.out")
	_, afterException := ioutil.ReadFile(rootPath + "/after.out")

	output := stderr.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(t, 0, strings.Count(output, "BEFORE_TERRAGRUNT_READ_CONFIG"), "terragrunt-read-config before_hook should not be triggered")
	t.Logf("output: %s", output)

	assert.Equal(t, 1, strings.Count(output, "AFTER_TERRAGRUNT_READ_CONFIG"), "Hooks on terragrunt-read-config command executed more than once")

	assert.NoError(t, beforeException)
	assert.NoError(t, afterException)
}

func TestTerragruntBeforeAndAfterMergeHook(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_MERGE_PATH, qaMyAppRelPath)
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	t.Logf("bucketName: %s", s3BucketName)
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_MERGE_PATH, qaMyAppRelPath, s3BucketName, config.DefaultTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))

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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)

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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level debug", rootPath), &stdout, &stderr)
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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)

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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)

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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
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

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, TEST_FIXTURE_PATH))

	var expectedS3Tags = map[string]string{
		"owner": "terragrunt integration test",
		"name":  "Terraform state storage"}
	validateS3BucketExistsAndIsTagged(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName, expectedS3Tags)

	var expectedDynamoDBTableTags = map[string]string{
		"owner": "terragrunt integration test",
		"name":  "Terraform lock table"}
	validateDynamoDBTableExistsAndIsTagged(t, TERRAFORM_REMOTE_STATE_S3_REGION, lockTableName, expectedDynamoDBTableTags)
}

// Regression test to ensure that `accesslogging_bucket_name` and `accesslogging_target_prefix` are taken into account
// & the TargetLogs bucket is set to a new S3 bucket, different from the origin S3 bucket
// & the logs objects are prefixed with the `accesslogging_target_prefix` value
func TestTerragruntSetsAccessLoggingForTfSTateS3BuckeToADifferentBucketWithGivenTargetPrefix(t *testing.T) {
	t.Parallel()

	examplePath := filepath.Join(TEST_FIXTURE_REGRESSIONS, "accesslogging-bucket/with-target-prefix-input")
	cleanupTerraformFolder(t, examplePath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	s3BucketLogsName := fmt.Sprintf("%s-tf-state-logs", s3BucketName)
	s3BucketLogsTargetPrefix := "logs/"
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(
		t,
		examplePath,
		s3BucketName,
		lockTableName,
		"remote_terragrunt.hcl",
	)

	runTerragrunt(t, fmt.Sprintf("terragrunt validate --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, examplePath))

	targetLoggingBucket := terraws.GetS3BucketLoggingTarget(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	targetLoggingBucketPrefix := terraws.GetS3BucketLoggingTargetPrefix(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	assert.Equal(t, s3BucketLogsName, targetLoggingBucket)
	assert.Equal(t, s3BucketLogsTargetPrefix, targetLoggingBucketPrefix)
}

// Regression test to ensure that `accesslogging_bucket_name` is taken into account
// & when no `accesslogging_target_prefix` provided, then **default** value is used for TargetPrefix
func TestTerragruntSetsAccessLoggingForTfSTateS3BuckeToADifferentBucketWithDefaultTargetPrefix(t *testing.T) {
	t.Parallel()

	examplePath := filepath.Join(TEST_FIXTURE_REGRESSIONS, "accesslogging-bucket/no-target-prefix-input")
	cleanupTerraformFolder(t, examplePath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	s3BucketLogsName := fmt.Sprintf("%s-tf-state-logs", s3BucketName)
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(
		t,
		examplePath,
		s3BucketName,
		lockTableName,
		"remote_terragrunt.hcl",
	)

	runTerragrunt(t, fmt.Sprintf("terragrunt validate --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, examplePath))

	targetLoggingBucket := terraws.GetS3BucketLoggingTarget(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	targetLoggingBucketPrefix := terraws.GetS3BucketLoggingTargetPrefix(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	assert.Equal(t, s3BucketLogsName, targetLoggingBucket)
	assert.Equal(t, remote.DefaultS3BucketAccessLoggingTargetPrefix, targetLoggingBucketPrefix)
}

func TestTerragruntWorksWithGCSBackend(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GCS_PATH)

	// We need a project to create the bucket in, so we pull one from the recommended environment variable.
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	defer deleteGCSBucket(t, gcsBucketName)

	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(t, TEST_FIXTURE_GCS_PATH, project, TERRAFORM_REMOTE_STATE_GCP_REGION, gcsBucketName, config.DefaultTerragruntConfigPath)
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntGCSConfigPath, TEST_FIXTURE_GCS_PATH))

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
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntGCSConfigPath, TEST_FIXTURE_GCS_BYO_BUCKET_PATH))

	validateGCSBucketExistsAndIsLabeled(t, location, gcsBucketName, nil)
}

func TestTerragruntWorksWithSingleJsonConfig(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_CONFIG_SINGLE_JSON_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_CONFIG_SINGLE_JSON_PATH)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_CONFIG_SINGLE_JSON_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", rootTerragruntConfigPath))
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

func TestTerragruntGraphDependenciesCommand(t *testing.T) {
	t.Parallel()

	// this test doesn't even run plan, it exits right after the stack was created
	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GRAPH_DEPENDENCIES)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GRAPH_DEPENDENCIES, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/root", tmpEnvPath, TEST_FIXTURE_GRAPH_DEPENDENCIES)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt graph-dependencies --terragrunt-working-dir %s", environmentPath), &stdout, &stderr)
	output := stdout.String()
	assert.True(t, strings.Contains(output, strings.TrimSpace(`
digraph {
	"backend-app" ;
	"backend-app" -> "mysql";
	"backend-app" -> "redis";
	"backend-app" -> "vpc";
	"frontend-app" ;
	"frontend-app" -> "backend-app";
	"frontend-app" -> "vpc";
	"mysql" ;
	"mysql" -> "vpc";
	"redis" ;
	"redis" -> "vpc";
	"vpc" ;
}
	`)))
}

func TestTerragruntRunAllCommand(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_OUTPUT_ALL)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL)

	runTerragrunt(t, fmt.Sprintf("terragrunt run-all init --terragrunt-non-interactive --terragrunt-working-dir %s", environmentPath))
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

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_STDOUT))
	runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt output foo --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_STDOUT), &stdout, &stderr)

	output := stdout.String()
	assert.Equal(t, "\"foo\"\n", output)
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

	logBufferContentsLineByLine(t, stdout, "output-all stdout")
	logBufferContentsLineByLine(t, stderr, "output-all stderr")

	// Without --terragrunt-ignore-dependency-errors, app2 never runs because its dependencies have "errors" since they don't have the output "app2_text".
	assert.True(t, strings.Contains(output, "app2 output"))
}

func testRemoteFixtureParallelism(t *testing.T, parallelism int, numberOfModules int, timeToDeployEachModule time.Duration) (string, int, error) {
	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	// folders inside the fixture
	//fixtureTemplate := path.Join(TEST_FIXTURE_PARALLELISM, "template")
	//fixtureApp := path.Join(TEST_FIXTURE_PARALLELISM, "app")

	// copy the template `numberOfModules` times into the app
	tmpEnvPath, err := ioutil.TempDir("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}
	for i := 0; i < numberOfModules; i++ {
		err := util.CopyFolderContents(TEST_FIXTURE_PARALLELISM, tmpEnvPath, ".terragrunt-test")
		if err != nil {
			return "", 0, err
		}
		err = os.Rename(
			path.Join(tmpEnvPath, "template"),
			path.Join(tmpEnvPath, "app"+strconv.Itoa(i)))
		if err != nil {
			return "", 0, err
		}
	}

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s", tmpEnvPath)

	// forces plugin download & initialization (no parallelism control)
	runTerragrunt(t, fmt.Sprintf("terragrunt plan-all --terragrunt-non-interactive --terragrunt-working-dir %s -var sleep_seconds=%d", environmentPath, timeToDeployEachModule/time.Second))
	// apply all with parallelism set
	// NOTE: we can't run just apply-all and not plan-all because the time to initialize the plugins skews the results of the test
	testStart := int(time.Now().Unix())
	t.Logf("apply-all start time = %d, %s", testStart, time.Now().Format(time.RFC3339))
	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-parallelism %d --terragrunt-non-interactive --terragrunt-working-dir %s -var sleep_seconds=%d", parallelism, environmentPath, timeToDeployEachModule/time.Second))

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	// Call runTerragruntCommand directly because this command contains failures (which causes runTerragruntRedirectOutput to abort) but we don't care.
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt output-all --terragrunt-non-interactive --terragrunt-working-dir %s", environmentPath), &stdout, &stderr)
	if err != nil {
		return "", 0, err
	}

	return stdout.String(), testStart, nil
}

func testTerragruntParallelism(t *testing.T, parallelism int, numberOfModules int, timeToDeployEachModule time.Duration, expectedTimings []int) {
	output, testStart, err := testRemoteFixtureParallelism(t, parallelism, numberOfModules, timeToDeployEachModule)
	require.NoError(t, err)

	// parse output and sort the times, the regex captures a string in the format time.RFC3339 emitted by terraform's timestamp function
	r, err := regexp.Compile(`out = "([-:\w]+)"`)
	require.NoError(t, err)

	matches := r.FindAllStringSubmatch(output, -1)
	require.True(t, len(matches) == numberOfModules)
	var times []int
	for _, v := range matches {
		// timestamp() is parsed
		parsed, err := time.Parse(time.RFC3339, v[1])
		require.NoError(t, err)
		times = append(times, int(parsed.Unix())-testStart)
	}
	sort.Slice(times, func(i, j int) bool {
		return times[i] < times[j]
	})

	// the reported times are skewed (running terragrunt/terraform apply adds a little bit of overhead)
	// we apply a simple scaling algorithm on the times based on the last expected time and the last actual time
	k := float64(times[len(times)-1]) / float64(expectedTimings[len(expectedTimings)-1])

	scaledTimes := make([]float64, len(times))
	for i := 0; i < len(times); i += 1 {
		scaledTimes[i] = float64(times[i]) / k
	}

	t.Logf("Parallelism test numberOfModules=%d p=%d expectedTimes=%v times=%v scaledTimes=%v scaleFactor=%f", numberOfModules, parallelism, expectedTimings, times, scaledTimes, k)

	maxDiffInSeconds := 3.0
	isEqual := func(x, y float64) bool {
		return math.Abs(x-y) <= maxDiffInSeconds
	}
	for i := 0; i < len(times); i += 1 {
		// it's impossible to know when will the first test finish however once a test finishes
		// we know that all the other times are relative to the first one
		assert.True(t, isEqual(scaledTimes[i], float64(expectedTimings[i])))
	}
}

func TestTerragruntParallelism(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		parallelism            int
		numberOfModules        int
		timeToDeployEachModule time.Duration
		expectedTimings        []int
	}{
		{1, 10, 5 * time.Second, []int{5, 10, 15, 20, 25, 30, 35, 40, 45, 50}},
		{3, 10, 5 * time.Second, []int{5, 5, 5, 10, 10, 10, 15, 15, 15, 20}},
		{5, 10, 5 * time.Second, []int{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}},
	}
	for _, tc := range testCases {
		tc := tc // shadow and force execution with this case
		t.Run(fmt.Sprintf("parallelism=%d numberOfModules=%d timeToDeployEachModule=%v expectedTimings=%v", tc.parallelism, tc.numberOfModules, tc.timeToDeployEachModule, tc.expectedTimings), func(t *testing.T) {
			// t.Parallel()
			testTerragruntParallelism(t, tc.parallelism, tc.numberOfModules, tc.timeToDeployEachModule, tc.expectedTimings)
		})
	}
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

func TestTerragruntStackCommandsWithPlanFile(t *testing.T) {
	t.Parallel()

	disjointEnvironmentPath := "fixture-stack/disjoint"
	cleanupTerraformFolder(t, disjointEnvironmentPath)
	runTerragrunt(t, fmt.Sprintf("terragrunt plan-all -out=plan.tfplan --terragrunt-log-level info --terragrunt-non-interactive --terragrunt-working-dir %s", disjointEnvironmentPath))
	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all plan.tfplan --terragrunt-log-level info --terragrunt-non-interactive --terragrunt-working-dir %s", disjointEnvironmentPath))
}

func TestLocalDownload(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_DOWNLOAD_PATH))

	// As of Terraform 0.14.0 we should be copying the lock file from .terragrunt-cache to the working directory
	assert.FileExists(t, util.JoinPath(TEST_FIXTURE_LOCAL_DOWNLOAD_PATH, util.TerraformLockFile))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_DOWNLOAD_PATH))
}

// As of Terraform 0.14.0, if there's already a lock file in the working directory, we should be copying it into
// .terragrunt-cache
func TestCustomLockFile(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_CUSTOM_LOCK_FILE)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_CUSTOM_LOCK_FILE))

	source := "../custom-lock-file-module"
	downloadDir := util.JoinPath(TEST_FIXTURE_CUSTOM_LOCK_FILE, TERRAGRUNT_CACHE)
	result, err := tfsource.NewTerraformSource(source, downloadDir, TEST_FIXTURE_CUSTOM_LOCK_FILE, util.CreateLogEntry("", options.DEFAULT_LOG_LEVEL))
	require.NoError(t, err)

	lockFilePath := util.JoinPath(result.WorkingDir, util.TerraformLockFile)
	require.FileExists(t, lockFilePath)

	readFile, err := ioutil.ReadFile(lockFilePath)
	require.NoError(t, err)

	// In our lock file, we intentionally have hashes for an older version of the AWS provider. If the lock file
	// copying works, then Terraform will stick with this older version. If there is a bug, Terraform will end up
	// installing a newer version (since the version is not pinned in the .tf code, only in the lock file).
	assert.Contains(t, string(readFile), `version = "3.0.0"`)
}

func TestLocalDownloadWithHiddenFolder(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_WITH_HIDDEN_FOLDER)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_WITH_HIDDEN_FOLDER))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_WITH_HIDDEN_FOLDER))
}

func TestLocalDownloadWithRelativePath(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_RELATIVE_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_RELATIVE_DOWNLOAD_PATH))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_RELATIVE_DOWNLOAD_PATH))
}

func TestRemoteDownload(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_REMOTE_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_REMOTE_DOWNLOAD_PATH))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_REMOTE_DOWNLOAD_PATH))
}

func TestRemoteDownloadWithRelativePath(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_REMOTE_RELATIVE_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_REMOTE_RELATIVE_DOWNLOAD_PATH))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_REMOTE_RELATIVE_DOWNLOAD_PATH))
}

func TestRemoteDownloadOverride(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_OVERRIDE_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", TEST_FIXTURE_OVERRIDE_DOWNLOAD_PATH, "../hello-world"))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", TEST_FIXTURE_OVERRIDE_DOWNLOAD_PATH, "../hello-world"))
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

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
}

func TestLocalWithMissingBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-lock-table-%s", strings.ToLower(uniqueId()))

	tmpEnvPath := copyEnvironment(t, "fixture-download")
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_LOCAL_MISSING_BACKEND)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), os.Stdout, os.Stderr)
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

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
}

func TestRemoteWithModuleInRoot(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_REMOTE_MODULE_IN_ROOT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_REMOTE_MODULE_IN_ROOT)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
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

func TestAutoRetryBasicRerun(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_RERUN)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_RERUN)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.Nil(t, err)
	assert.Contains(t, out.String(), "Apply complete!")
}

func TestAutoRetrySkip(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_RERUN)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_RERUN)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-no-auto-retry --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.NotNil(t, err)
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryExhaustRetries(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_EXHAUST)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_EXHAUST)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.NotNil(t, err)
	assert.Contains(t, out.String(), "Failed to load backend")
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryCustomRetryableErrors(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_CUSTOM_ERRORS)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_CUSTOM_ERRORS)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.Nil(t, err)
	assert.Contains(t, out.String(), "My own little error")
	assert.Contains(t, out.String(), "Apply complete!")
}

func TestAutoRetryCustomRetryableErrorsFailsWhenRetryableErrorsNotSet(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_CUSTOM_ERRORS_NOT_SET)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_CUSTOM_ERRORS_NOT_SET)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.NotNil(t, err)
	assert.Contains(t, out.String(), "My own little error")
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryFlagWithRecoverableError(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_RERUN)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_RERUN)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-no-auto-retry --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.NotNil(t, err)
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryEnvVarWithRecoverableError(t *testing.T) {
	os.Setenv("TERRAGRUNT_AUTO_RETRY", "false")
	defer os.Unsetenv("TERRAGRUNT_AUTO_RETRY")
	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_RERUN)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_RERUN)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.NotNil(t, err)
	assert.NotContains(t, out.String(), "Apply complete!")
}

func TestAutoRetryApplyAllDependentModuleRetries(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_APPLY_ALL_RETRIES)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_APPLY_ALL_RETRIES)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), out, os.Stderr)

	assert.Nil(t, err)
	s := out.String()
	assert.Contains(t, s, "app1 output")
	assert.Contains(t, s, "app2 output")
	assert.Contains(t, s, "app3 output")
	assert.Contains(t, s, "Apply complete!")
}

func TestAutoRetryConfigurableRetries(t *testing.T) {
	t.Parallel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), stdout, stderr)
	sleeps := regexp.MustCompile("Sleeping 0s before retrying.").FindAllStringIndex(stderr.String(), -1)

	assert.Nil(t, err)
	assert.Equal(t, 4, len(sleeps)) // 5 retries, so 4 sleeps
	assert.Contains(t, stdout.String(), "Apply complete!")
}

func TestAutoRetryConfigurableRetriesErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		fixture      string
		errorMessage string
	}{
		{TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES_ERROR_1, "Cannot have less than 1 max retry"},
		{TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES_ERROR_2, "Cannot sleep for less than 0 seconds"},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			rootPath := copyEnvironment(t, tc.fixture)
			modulePath := util.JoinPath(rootPath, tc.fixture)

			err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", modulePath), stdout, stderr)
			assert.NotNil(t, err)
			assert.NotContains(t, stdout.String(), "Apply complete!")
			assert.Contains(t, err.Error(), tc.errorMessage)
		})
	}
}

func TestAwsProviderPatch(t *testing.T) {
	t.Parallel()

	stderr := new(bytes.Buffer)
	rootPath := copyEnvironment(t, TEST_FIXTURE_AWS_PROVIDER_PATCH)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_AWS_PROVIDER_PATCH)
	mainTFFile := filepath.Join(modulePath, "main.tf")

	// fill in branch so we can test against updates to the test case file
	mainContents, err := util.ReadFileAsString(mainTFFile)
	require.NoError(t, err)
	mainContents = strings.Replace(mainContents, "__BRANCH_NAME__", git.GetCurrentBranchName(t), -1)
	require.NoError(t, ioutil.WriteFile(mainTFFile, []byte(mainContents), 0444))

	assert.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt aws-provider-patch --terragrunt-override-attr region=\"eu-west-1\" --terragrunt-override-attr allowed_account_ids=[\"00000000000\"] --terragrunt-working-dir %s --terragrunt-log-level debug", modulePath), os.Stdout, stderr),
	)
	t.Log(stderr.String())

	assert.Regexp(t, "Patching AWS provider in .+test/fixture-aws-provider-patch/example-module/main.tf", stderr.String())

	// Make sure the resulting terraform code is still valid
	assert.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt validate --terragrunt-working-dir %s", modulePath), os.Stdout, os.Stderr),
	)
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
		cmd := fmt.Sprintf("terragrunt %s --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", strings.Join(testCase.command, " "), TEST_FIXTURE_EXTRA_ARGS_PATH)

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
		cmd := fmt.Sprintf("terragrunt %s --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", strings.Join(testCase.command, " "), TEST_FIXTURE_EXTRA_ARGS_PATH)

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

func TestPreventDestroyOverride(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_PREVENT_DESTROY_OVERRIDE)

	assert.NoError(t, runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-working-dir %s", TEST_FIXTURE_PREVENT_DESTROY_OVERRIDE), os.Stdout, os.Stderr))
	assert.NoError(t, runTerragruntCommand(t, fmt.Sprintf("terragrunt destroy -auto-approve --terragrunt-working-dir %s", TEST_FIXTURE_PREVENT_DESTROY_OVERRIDE), os.Stdout, os.Stderr))
}

func TestPreventDestroyNotSet(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_PREVENT_DESTROY_NOT_SET)

	assert.NoError(t, runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-working-dir %s", TEST_FIXTURE_PREVENT_DESTROY_NOT_SET), os.Stdout, os.Stderr))
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt destroy -auto-approve --terragrunt-working-dir %s", TEST_FIXTURE_PREVENT_DESTROY_NOT_SET), os.Stdout, os.Stderr)

	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, cli.ModuleIsProtected{}, underlying)
	}
}

func TestPreventDestroy(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_PREVENT_DESTROY)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_PREVENT_DESTROY))

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt destroy -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_PREVENT_DESTROY), os.Stdout, os.Stderr)

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
		assert.IsType(t, &multierror.Error{}, underlying)
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

func validateInputs(t *testing.T, outputs map[string]TerraformOutput) {
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

func TestInputsPassedThroughCorrectly(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_INPUTS)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_INPUTS)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_INPUTS)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	validateInputs(t, outputs)
}

func TestNoAutoInit(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_REGRESSIONS)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_REGRESSIONS)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_REGRESSIONS, "skip-init")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-no-auto-init --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "no force apply stdout")
	logBufferContentsLineByLine(t, stderr, "no force apply stderr")
	require.Error(t, err)
	require.Contains(t, stderr.String(), "This module is not yet installed.")
}

func TestLocalsParsing(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCALS_CANONICAL)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCALS_CANONICAL))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCALS_CANONICAL), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))

	assert.Equal(t, outputs["data"].Value, "Hello world\n")
	assert.Equal(t, outputs["answer"].Value, float64(42))
}

func TestLocalsInInclude(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCALS_IN_INCLUDE)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_LOCALS_IN_INCLUDE)
	childPath := filepath.Join(tmpEnvPath, TEST_FIXTURE_LOCALS_IN_INCLUDE, TEST_FIXTURE_LOCALS_IN_INCLUDE_CHILD_REL_PATH)
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve -no-color --terragrunt-non-interactive --terragrunt-working-dir %s", childPath))

	// Check the outputs of the dir functions referenced in locals to make sure they return what is expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))

	assert.Equal(
		t,
		filepath.Join(tmpEnvPath, TEST_FIXTURE_LOCALS_IN_INCLUDE),
		outputs["parent_terragrunt_dir"].Value,
	)
	assert.Equal(
		t,
		childPath,
		outputs["terragrunt_dir"].Value,
	)
	assert.Equal(
		t,
		"apply",
		outputs["terraform_command"].Value,
	)
	assert.Equal(
		t,
		"[\"apply\",\"-auto-approve\",\"-no-color\"]",
		outputs["terraform_cli_args"].Value,
	)
}

func TestUndefinedLocalsReferenceBreaks(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCALS_ERROR_UNDEFINED_LOCAL)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCALS_ERROR_UNDEFINED_LOCAL), os.Stdout, os.Stderr)
	assert.Error(t, err)
}

func TestUndefinedLocalsReferenceToInputsBreaks(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCALS_ERROR_UNDEFINED_LOCAL_BUT_INPUT)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCALS_ERROR_UNDEFINED_LOCAL_BUT_INPUT), os.Stdout, os.Stderr)
	assert.Error(t, err)
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
		assert.IsType(t, &multierror.Error{}, underlying)
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
		{TEST_FIXTURE_LOCAL_WITH_EXCLUDE_DIR, "--terragrunt-exclude-dir **/gce/**/*", []string{"Module GCE B", "Module GCE C", "Module GCE E"}},
		{TEST_FIXTURE_LOCAL_WITH_EXCLUDE_DIR, "--terragrunt-exclude-dir production-env/**/* --terragrunt-exclude-dir **/module-gce-c", []string{"Module GCE C", "Module AWS D", "Module GCE E"}},
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

func TestIncludeDirsDependencyConsistencyRegression(t *testing.T) {
	t.Parallel()

	modulePaths := []string{
		"amazing-app/k8s",
		"clusters/eks",
		"testapp/k8s",
	}

	testPath := filepath.Join(TEST_FIXTURE_REGRESSIONS, "exclude-dependency")
	cleanupTerragruntFolder(t, testPath)
	for _, modulePath := range modulePaths {
		cleanupTerragruntFolder(t, filepath.Join(testPath, modulePath))
	}

	includedModulesWithNone := runValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{}, false)
	assert.Greater(t, len(includedModulesWithNone), 0)

	includedModulesWithAmzApp := runValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{"amazing-app/k8s"}, false)
	assert.Equal(t, includedModulesWithAmzApp, []string{"amazing-app/k8s", "clusters/eks"})

	includedModulesWithTestApp := runValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{"testapp/k8s"}, false)
	assert.Equal(t, includedModulesWithTestApp, []string{"clusters/eks", "testapp/k8s"})
}

func TestIncludeDirsStrict(t *testing.T) {
	t.Parallel()

	modulePaths := []string{
		"amazing-app/k8s",
		"clusters/eks",
		"testapp/k8s",
	}

	testPath := filepath.Join(TEST_FIXTURE_REGRESSIONS, "exclude-dependency")
	cleanupTerragruntFolder(t, testPath)
	for _, modulePath := range modulePaths {
		cleanupTerragruntFolder(t, filepath.Join(testPath, modulePath))
	}

	includedModulesWithNone := runValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{}, true)
	assert.Equal(t, includedModulesWithNone, []string{})

	includedModulesWithAmzApp := runValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{"amazing-app/k8s"}, true)
	assert.Equal(t, includedModulesWithAmzApp, []string{"amazing-app/k8s"})

	includedModulesWithTestApp := runValidateAllWithIncludeAndGetIncludedModules(t, testPath, []string{"testapp/k8s"}, true)
	assert.Equal(t, includedModulesWithTestApp, []string{"testapp/k8s"})
}

func TestTerragruntExternalDependencies(t *testing.T) {
	t.Parallel()

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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-log-level info --terragrunt-non-interactive --terragrunt-working-dir %s --var person=Hobbs", rootPath), &showStdout, &showStderr)
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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &showStdout, &showStderr)
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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level info", rootPath), &showStdout, &showStderr)
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

// Test case for yamldecode bug: https://github.com/gruntwork-io/terragrunt/issues/834
func TestYamlDecodeRegressions(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_REGRESSIONS)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_REGRESSIONS)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_REGRESSIONS, "yamldecode")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// Check the output of yamldecode and make sure it doesn't parse the string incorrectly
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	assert.Equal(t, outputs["test1"].Value, "003")
	assert.Equal(t, outputs["test2"].Value, "1.00")
	assert.Equal(t, outputs["test3"].Value, "0ba")
}

// We test the path with remote_state blocks by:
// - Applying all modules initially
// - Deleting the local state of the nested deep dependency
// - Running apply on the root module
// If output optimization is working, we should still get the same correct output even though the state of the upmost
// module has been destroyed.
func TestDependencyOutputOptimization(t *testing.T) {
	expectOutputLogs := []string{
		`Running command: terraform init -get=false prefix=\[.*fixture-get-output/nested-optimization/dep\]`,
	}
	dependencyOutputOptimizationTest(t, "nested-optimization", true, expectOutputLogs)
}

func TestDependencyOutputOptimizationSkipInit(t *testing.T) {
	expectOutputLogs := []string{
		`Detected module .*nested-optimization/dep/terragrunt.hcl is already init-ed. Retrieving outputs directly from working directory. prefix=\[.*fixture-get-output/nested-optimization/dep\]`,
	}
	dependencyOutputOptimizationTest(t, "nested-optimization", false, expectOutputLogs)
}

func TestDependencyOutputOptimizationNoGenerate(t *testing.T) {
	expectOutputLogs := []string{
		`Running command: terraform init -get=false prefix=\[.*fixture-get-output/nested-optimization-nogen/dep\]`,
	}
	dependencyOutputOptimizationTest(t, "nested-optimization-nogen", true, expectOutputLogs)
}

func dependencyOutputOptimizationTest(t *testing.T, moduleName string, forceInit bool, expectedOutputLogs []string) {
	t.Parallel()

	expectedOutput := `They said, "No, The answer is 42"`
	generatedUniqueId := uniqueId()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := filepath.Join(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, moduleName)
	rootTerragruntConfigPath := filepath.Join(rootPath, config.DefaultTerragruntConfigPath)
	livePath := filepath.Join(rootPath, "live")
	deepDepPath := filepath.Join(rootPath, "deepdep")
	depPath := filepath.Join(rootPath, "dep")

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(generatedUniqueId))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(generatedUniqueId))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// verify expected output
	stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", livePath))
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))
	assert.Equal(t, expectedOutput, outputs["output"].Value)

	// If we want to force reinit, delete the relevant .terraform directories
	if forceInit {
		cleanupTerraformFolder(t, depPath)
	}

	// Now delete the deepdep state and verify still works (note we need to bust the cache again)
	config.ClearOutputCache()
	require.NoError(t, os.Remove(filepath.Join(deepDepPath, "terraform.tfstate")))
	reout, reerr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", livePath))
	require.NoError(t, err)

	require.NoError(t, json.Unmarshal([]byte(reout), &outputs))
	assert.Equal(t, expectedOutput, outputs["output"].Value)

	for _, logRegexp := range expectedOutputLogs {
		re, err := regexp.Compile(logRegexp)
		require.NoError(t, err)
		matches := re.FindAllString(reerr, -1)
		assert.Greater(t, len(matches), 0)
	}
}

func TestDependencyOutputOptimizationDisableTest(t *testing.T) {
	t.Parallel()

	expectedOutput := `They said, "No, The answer is 42"`
	generatedUniqueId := uniqueId()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := filepath.Join(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "nested-optimization-disable")
	rootTerragruntConfigPath := filepath.Join(rootPath, config.DefaultTerragruntConfigPath)
	livePath := filepath.Join(rootPath, "live")
	deepDepPath := filepath.Join(rootPath, "deepdep")

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(generatedUniqueId))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(generatedUniqueId))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// verify expected output
	stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", livePath))
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))
	assert.Equal(t, expectedOutput, outputs["output"].Value)

	// Now delete the deepdep state and verify it no longer works, because it tries to fetch the deepdep dependency
	config.ClearOutputCache()
	require.NoError(t, os.Remove(filepath.Join(deepDepPath, "terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(deepDepPath, ".terraform")))
	_, _, err = runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", livePath))
	require.Error(t, err)
}

func TestDependencyOutput(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "integration")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// verify expected output 42
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	app3Path := util.JoinPath(rootPath, "app3")
	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", app3Path), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	assert.Equal(t, int(outputs["z"].Value.(float64)), 42)
}

func TestDependencyOutputErrorBeforeApply(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := filepath.Join(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "integration")
	app3Path := filepath.Join(rootPath, "app3")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", app3Path), &showStdout, &showStderr)
	assert.Error(t, err)
	// Verify that we fail because the dependency is not applied yet
	assert.Contains(t, err.Error(), "has not been applied yet")

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")
}

func TestDependencyOutputSkipOutputs(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := filepath.Join(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "integration")
	emptyPath := filepath.Join(rootPath, "empty")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	// Test that even if the dependency (app1) is not applied, using skip_outputs will skip pulling the outputs so there
	// will be no errors.
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", emptyPath), &showStdout, &showStderr)
	assert.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")
}

func TestDependencyOutputSkipOutputsWithMockOutput(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := filepath.Join(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs")
	dependent3Path := filepath.Join(rootPath, "dependent3")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", dependent3Path), &showStdout, &showStderr)
	assert.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", dependent3Path), &stdout, &stderr),
	)
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	assert.Equal(t, outputs["truth"].Value, "The answer is 0")

	// Now apply-all so that the dependency is applied, and verify it still uses the mock output
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &showStdout, &showStderr)
	assert.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", dependent3Path), &stdout, &stderr),
	)
	outputs = map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	assert.Equal(t, outputs["truth"].Value, "The answer is 0")
}

// Test that when you have a mock_output on a dependency, the dependency will use the mock as the output instead
// of erroring out.
func TestDependencyMockOutput(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := filepath.Join(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs")
	dependent1Path := filepath.Join(rootPath, "dependent1")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", dependent1Path), &showStdout, &showStderr)
	assert.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", dependent1Path), &stdout, &stderr),
	)
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	assert.Equal(t, outputs["truth"].Value, "The answer is 0")

	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// Now apply-all so that the dependency is applied, and verify it uses the dependency output
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &showStdout, &showStderr)
	assert.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", dependent1Path), &stdout, &stderr),
	)
	outputs = map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	assert.Equal(t, outputs["truth"].Value, "The answer is 42")
}

// Test that when you have a mock_output on a dependency, the dependency will use the mock as the output instead
// of erroring out when running an allowed command.
func TestDependencyMockOutputRestricted(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := filepath.Join(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs")
	dependent2Path := filepath.Join(rootPath, "dependent2")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", dependent2Path), &showStdout, &showStderr)
	assert.Error(t, err)
	// Verify that we fail because the dependency is not applied yet
	assert.Contains(t, err.Error(), "has not been applied yet")

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// Verify we can run when using one of the allowed commands
	showStdout.Reset()
	showStderr.Reset()
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir %s", dependent2Path), &showStdout, &showStderr)
	assert.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// Verify that validate-all works as well.
	showStdout.Reset()
	showStderr.Reset()
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt validate-all --terragrunt-non-interactive --terragrunt-working-dir %s", dependent2Path), &showStdout, &showStderr)
	assert.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	showStdout.Reset()
	showStderr.Reset()
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt validate-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &showStdout, &showStderr)
	assert.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")
}

func TestDependencyOutputTypeConversion(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	cleanupTerraformFolder(t, TEST_FIXTURE_INPUTS)
	tmpEnvPath := copyEnvironment(t, ".")

	inputsPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_INPUTS)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "type-conversion")

	// First apply the inputs module
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", inputsPath))

	// Then apply the outputs module
	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}
	assert.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &showStdout, &showStderr),
	)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

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

// Regression testing for https://github.com/gruntwork-io/terragrunt/issues/1102: Ordering keys from
// maps to avoid random placements when terraform file is generated.
func TestOrderedMapOutputRegressions1102(t *testing.T) {
	t.Parallel()
	generateTestCase := filepath.Join(TEST_FIXTURE_GET_OUTPUT, "regression-1102")

	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	command := fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase)
	path := filepath.Join(generateTestCase, "backend.tf")

	// runs terragrunt for the first time and checks the output "backend.tf" file.
	require.NoError(
		t,
		runTerragruntCommand(t, command, &stdout, &stderr),
	)
	expected, _ := ioutil.ReadFile(path)
	require.Contains(t, string(expected), "local")

	// runs terragrunt again. All the outputs must be
	// equal to the first run.
	for i := 0; i < 20; i++ {
		require.NoError(
			t,
			runTerragruntCommand(t, command, &stdout, &stderr),
		)
		actual, _ := ioutil.ReadFile(path)
		require.Equal(t, expected, actual)
	}
}

// Test that we get the expected error message about dependency cycles when there is a cycle in the dependency chain
func TestDependencyOutputCycleHandling(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)

	testCases := []string{
		"aa",
		"aba",
		"abca",
		"abcda",
	}

	for _, testCase := range testCases {
		// Capture range variable into forloop so that the binding is consistent across runs.
		testCase := testCase

		t.Run(testCase, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
			rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "cycle", testCase)
			fooPath := util.JoinPath(rootPath, "foo")

			planStdout := bytes.Buffer{}
			planStderr := bytes.Buffer{}
			err := runTerragruntCommand(
				t,
				fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", fooPath),
				&planStdout,
				&planStderr,
			)
			logBufferContentsLineByLine(t, planStdout, "plan stdout")
			logBufferContentsLineByLine(t, planStderr, "plan stderr")
			assert.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), "Found a dependency cycle between modules"))
		})
	}
}

// Regression testing for https://github.com/gruntwork-io/terragrunt/issues/854: Referencing a dependency that is a
// subdirectory of the current config, which includes an `include` block has problems resolving the correct relative
// path.
func TestDependencyOutputRegression854(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "regression-854", "root")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(
		t,
		fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath),
		&stdout,
		&stderr,
	)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

// Regression testing for https://github.com/gruntwork-io/terragrunt/issues/906
func TestDependencyOutputSameOutputConcurrencyRegression(t *testing.T) {
	t.Parallel()

	// Use func to isolate each test run to a single s3 bucket that is deleted. We run the test multiple times
	// because the underlying error we are trying to test against is nondeterministic, and thus may not always work
	// the first time.
	testCase := func() {
		cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
		tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
		rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "regression-906")

		// Make sure to fill in the s3 bucket to the config. Also ensure the bucket is deleted before the next for
		// loop call.
		s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s%s", strings.ToLower(uniqueId()), strings.ToLower(uniqueId()))
		defer deleteS3BucketWithRetry(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
		commonDepConfigPath := util.JoinPath(rootPath, "common-dep", "terragrunt.hcl")
		copyTerragruntConfigAndFillPlaceholders(t, commonDepConfigPath, commonDepConfigPath, s3BucketName, "not-used", "not-used")

		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}
		err := runTerragruntCommand(
			t,
			fmt.Sprintf("terragrunt apply-all --terragrunt-source-update --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath),
			&stdout,
			&stderr,
		)
		logBufferContentsLineByLine(t, stdout, "stdout")
		logBufferContentsLineByLine(t, stderr, "stderr")
		require.NoError(t, err)
	}

	for i := 0; i < 3; i++ {
		testCase()
		// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
		// This is only a problem during testing, where the process is shared across terragrunt runs.
		config.ClearOutputCache()
	}
}

// Regression testing for bug where terragrunt output runs on dependency blocks are done in the terragrunt-cache for the
// child, not the parent.
func TestDependencyOutputCachePathBug(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "localstate", "live")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(
		t,
		fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath),
		&stdout,
		&stderr,
	)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

func TestDependencyOutputWithTerragruntSource(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "regression-1124", "live")
	modulePath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "regression-1124", "modules")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(
		t,
		fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", rootPath, modulePath),
		&stdout,
		&stderr,
	)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

func TestDependencyOutputWithHooks(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "regression-1273")
	depPathFileOut := util.JoinPath(rootPath, "dep", "file.out")
	mainPath := util.JoinPath(rootPath, "main")
	mainPathFileOut := util.JoinPath(mainPath, "file.out")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// The file should exist in the first run.
	assert.True(t, util.FileExists(depPathFileOut))
	assert.False(t, util.FileExists(mainPathFileOut))

	// Now delete file and run just main again. It should NOT create file.out.
	require.NoError(t, os.Remove(depPathFileOut))
	runTerragrunt(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", mainPath))
	assert.False(t, util.FileExists(depPathFileOut))
	assert.False(t, util.FileExists(mainPathFileOut))
}

func TestDeepDependencyOutputWithMock(t *testing.T) {
	// Test that the terraform command flows through for mock output retrieval to deeper dependencies. Previously the
	// terraform command was being overwritten, so by the time the deep dependency retrieval runs, it was replaced with
	// "output" instead of the original one.

	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := filepath.Join(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "nested-mocks", "live")

	// Since we haven't applied anything, this should only succeed if mock outputs are used.
	runTerragrunt(t, fmt.Sprintf("terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
}

func TestAWSGetCallerIdentityFunctions(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_AWS_GET_CALLER_IDENTITY)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_AWS_GET_CALLER_IDENTITY)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_AWS_GET_CALLER_IDENTITY)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

	// Get values from STS
	sess, err := session.NewSession()
	if err != nil {
		t.Fatalf("Error while creating AWS session: %v", err)
	}

	identity, err := sts.New(sess).GetCallerIdentity(nil)
	if err != nil {
		t.Fatalf("Error while getting AWS caller identity: %v", err)
	}

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	assert.Equal(t, outputs["account"].Value, *identity.Account)
	assert.Equal(t, outputs["arn"].Value, *identity.Arn)
	assert.Equal(t, outputs["user_id"].Value, *identity.UserId)
}

func TestGetPlatform(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_PLATFORM)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_PLATFORM)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_PLATFORM)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}

	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	platform, hasPlatform := outputs["platform"]
	require.True(t, hasPlatform)
	require.Equal(t, platform.Value, runtime.GOOS)
}

func TestDataDir(t *testing.T) {
	// Cannot be run in parallel with other tests as it modifies process' environment.

	cleanupTerraformFolder(t, TEST_FIXTURE_DIRS_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_DIRS_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_DIRS_PATH)

	os.Setenv("TF_DATA_DIR", util.JoinPath(tmpEnvPath, "data_dir"))
	defer os.Unsetenv("TF_DATA_DIR")

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	erroutput := stderr.String()

	if err != nil {
		t.Errorf("Did not expect to get an error: %s", err.Error())
	}

	assert.Contains(t, erroutput, "Initializing provider plugins")

	var (
		stdout2 bytes.Buffer
		stderr2 bytes.Buffer
	)

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout2, &stderr2)
	erroutput2 := stderr2.String()

	if err != nil {
		t.Errorf("Did not expect to get an error: %s", err.Error())
	}

	assert.NotContains(t, erroutput2, "Initializing provider plugins")
}

func TestReadTerragruntConfigWithDependency(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_READ_CONFIG)
	cleanupTerraformFolder(t, TEST_FIXTURE_INPUTS)
	tmpEnvPath := copyEnvironment(t, ".")

	inputsPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_INPUTS)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_READ_CONFIG, "with_dependency")

	// First apply the inputs module
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", inputsPath))

	// Then apply the read config module
	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}
	assert.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &showStdout, &showStderr),
	)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

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

func TestReadTerragruntConfigFromDependency(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_READ_CONFIG)
	tmpEnvPath := copyEnvironment(t, ".")
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_READ_CONFIG, "from_dependency")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}
	assert.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &showStdout, &showStderr),
	)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))

	assert.Equal(t, outputs["bar"].Value, "hello world")
}

func TestReadTerragruntConfigWithDefault(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_READ_CONFIG)
	rootPath := util.JoinPath(TEST_FIXTURE_READ_CONFIG, "with_default")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))

	assert.Equal(t, outputs["data"].Value, "default value")
}

func TestReadTerragruntConfigWithOriginalTerragruntDir(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_READ_CONFIG)
	rootPath := util.JoinPath(TEST_FIXTURE_READ_CONFIG, "with_original_terragrunt_dir")

	rootPathAbs, err := filepath.Abs(rootPath)
	require.NoError(t, err)
	fooPathAbs := filepath.Join(rootPathAbs, "foo")
	depPathAbs := filepath.Join(rootPathAbs, "dep")

	// Run apply on the dependency module and make sure we get the outputs we expect
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", depPathAbs))

	depStdout := bytes.Buffer{}
	depStderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", depPathAbs), &depStdout, &depStderr),
	)

	depOutputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(depStdout.String()), &depOutputs))

	assert.Equal(t, depPathAbs, depOutputs["terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, depOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, depOutputs["bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, depOutputs["bar_original_terragrunt_dir"].Value)

	// Run apply on the root module and make sure we get the expected outputs
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	rootStdout := bytes.Buffer{}
	rootStderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &rootStdout, &rootStderr),
	)

	rootOutputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(rootStdout.String()), &rootOutputs))

	assert.Equal(t, fooPathAbs, rootOutputs["terragrunt_dir"].Value)
	assert.Equal(t, rootPathAbs, rootOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, rootOutputs["dep_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, rootOutputs["dep_original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, rootOutputs["dep_bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, rootOutputs["dep_bar_original_terragrunt_dir"].Value)

	// Run 'run-all apply' and make sure all the outputs are identical in the root module and the dependency module
	runTerragrunt(t, fmt.Sprintf("terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	runAllRootStdout := bytes.Buffer{}
	runAllRootStderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &runAllRootStdout, &runAllRootStderr),
	)

	runAllRootOutputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(runAllRootStdout.String()), &runAllRootOutputs))

	runAllDepStdout := bytes.Buffer{}
	runAllDepStderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", depPathAbs), &runAllDepStdout, &runAllDepStderr),
	)

	runAllDepOutputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(runAllDepStdout.String()), &runAllDepOutputs))

	assert.Equal(t, fooPathAbs, runAllRootOutputs["terragrunt_dir"].Value)
	assert.Equal(t, rootPathAbs, runAllRootOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllRootOutputs["dep_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllRootOutputs["dep_original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, runAllRootOutputs["dep_bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllRootOutputs["dep_bar_original_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllDepOutputs["terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllDepOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, runAllDepOutputs["bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllDepOutputs["bar_original_terragrunt_dir"].Value)
}

func TestReadTerragruntConfigFull(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_READ_CONFIG)
	rootPath := util.JoinPath(TEST_FIXTURE_READ_CONFIG, "full")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))

	// Primitive config attributes
	assert.Equal(t, outputs["terraform_binary"].Value, "terragrunt")
	assert.Equal(t, outputs["terraform_version_constraint"].Value, "= 0.12.20")
	assert.Equal(t, outputs["terragrunt_version_constraint"].Value, "= 0.23.18")
	assert.Equal(t, outputs["download_dir"].Value, ".terragrunt-cache")
	assert.Equal(t, outputs["iam_role"].Value, "TerragruntIAMRole")
	assert.Equal(t, outputs["skip"].Value, "true")
	assert.Equal(t, outputs["prevent_destroy"].Value, "true")

	// Simple maps
	localstgOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["localstg"].Value.(string)), &localstgOut))
	assert.Equal(t, localstgOut, map[string]interface{}{"the_answer": float64(42)})
	inputsOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["inputs"].Value.(string)), &inputsOut))
	assert.Equal(t, inputsOut, map[string]interface{}{"doc": "Emmett Brown"})

	// Complex blocks
	depsOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["dependencies"].Value.(string)), &depsOut))
	assert.Equal(
		t,
		depsOut,
		map[string]interface{}{
			"paths": []interface{}{"../module-a"},
		},
	)
	generateOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["generate"].Value.(string)), &generateOut))
	assert.Equal(
		t,
		generateOut,
		map[string]interface{}{
			"provider": map[string]interface{}{
				"path":              "provider.tf",
				"if_exists":         "overwrite_terragrunt",
				"comment_prefix":    "# ",
				"disable_signature": false,
				"contents": `provider "aws" {
  region = "us-east-1"
}
`,
			},
		},
	)
	remoteStateOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["remote_state"].Value.(string)), &remoteStateOut))
	assert.Equal(
		t,
		remoteStateOut,
		map[string]interface{}{
			"backend":                         "local",
			"disable_init":                    false,
			"disable_dependency_optimization": false,
			"generate":                        map[string]interface{}{"path": "backend.tf", "if_exists": "overwrite_terragrunt"},
			"config":                          map[string]interface{}{"path": "foo.tfstate"},
		},
	)
	terraformOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["terraformtg"].Value.(string)), &terraformOut))
	assert.Equal(
		t,
		terraformOut,
		map[string]interface{}{
			"source": "./delorean",
			"extra_arguments": map[string]interface{}{
				"var-files": map[string]interface{}{
					"name":               "var-files",
					"commands":           []interface{}{"apply", "plan"},
					"arguments":          nil,
					"required_var_files": []interface{}{"extra.tfvars"},
					"optional_var_files": []interface{}{"optional.tfvars"},
					"env_vars": map[string]interface{}{
						"TF_VAR_custom_var": "I'm set in extra_arguments env_vars",
					},
				},
			},
			"before_hook": map[string]interface{}{
				"before_hook_1": map[string]interface{}{
					"name":         "before_hook_1",
					"commands":     []interface{}{"apply", "plan"},
					"execute":      []interface{}{"touch", "before.out"},
					"working_dir":  nil,
					"run_on_error": true,
				},
			},
			"after_hook": map[string]interface{}{
				"after_hook_1": map[string]interface{}{
					"name":         "after_hook_1",
					"commands":     []interface{}{"apply", "plan"},
					"execute":      []interface{}{"touch", "after.out"},
					"working_dir":  nil,
					"run_on_error": true,
				},
			},
		},
	)
}

func logBufferContentsLineByLine(t *testing.T, out bytes.Buffer, label string) {
	t.Logf("[%s] Full contents of %s:", t.Name(), label)
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		t.Logf("[%s] %s", t.Name(), line)
	}
}

func TestTerragruntGenerateBlockSkip(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "generate-block", "skip")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase))
	assert.False(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
}

func TestTerragruntGenerateBlockOverwrite(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "generate-block", "overwrite")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase))
	// If the state file was written as foo.tfstate, that means it overwrote the local backend config.
	assert.True(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, fileIsInFolder(t, "bar.tfstate", generateTestCase))
}

func TestTerragruntGenerateAttr(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "generate-attr")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	text := "test-terragrunt-generate-attr-hello-world"

	stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s -var text=\"%s\"", generateTestCase, text))
	require.NoError(t, err)
	require.Contains(t, stdout, text)
}

func TestTerragruntGenerateBlockOverwriteTerragruntSuccess(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "generate-block", "overwrite_terragrunt")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase))
	// If the state file was written as foo.tfstate, that means it overwrote the local backend config.
	assert.True(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, fileIsInFolder(t, "bar.tfstate", generateTestCase))
}

func TestTerragruntGenerateBlockOverwriteTerragruntFail(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "generate-block", "overwrite_terragrunt_error")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase), &stdout, &stderr)
	require.Error(t, err)
	_, ok := errors.Unwrap(err).(codegen.GenerateFileExistsError)
	assert.True(t, ok)
}

func TestTerragruntGenerateBlockNestedInherit(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "generate-block", "nested", "child_inherit")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase))
	// If the state file was written as foo.tfstate, that means it inherited the config
	assert.True(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, fileIsInFolder(t, "bar.tfstate", generateTestCase))
	// Also check to make sure the child config generate block was included
	assert.True(t, fileIsInFolder(t, "random_file.txt", generateTestCase))
}

func TestTerragruntGenerateBlockNestedOverwrite(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "generate-block", "nested", "child_overwrite")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase))
	// If the state file was written as bar.tfstate, that means it overwrite the parent config
	assert.False(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.True(t, fileIsInFolder(t, "bar.tfstate", generateTestCase))
	// Also check to make sure the child config generate block was included
	assert.True(t, fileIsInFolder(t, "random_file.txt", generateTestCase))
}

func TestTerragruntGenerateBlockDisableSignature(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "generate-block", "disable-signature")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase))

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))

	assert.Equal(t, outputs["text"].Value, "Hello, World!")
}

func TestTerragruntRemoteStateCodegenGeneratesBackendBlock(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "remote-state", "base")

	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase))
	// If the state file was written as foo.tfstate, that means it wrote out the local backend config.
	assert.True(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
}

func TestTerragruntRemoteStateCodegenOverwrites(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "remote-state", "overwrite")

	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase))
	// If the state file was written as foo.tfstate, that means it overwrote the local backend config.
	assert.True(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, fileIsInFolder(t, "bar.tfstate", generateTestCase))
}

func TestTerragruntRemoteStateCodegenGeneratesBackendBlockS3(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "remote-state", "s3")

	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, generateTestCase, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, generateTestCase))
}

func TestTerragruntRemoteStateCodegenErrorsIfExists(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "remote-state", "error")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase), &stdout, &stderr)
	require.Error(t, err)
	_, ok := errors.Unwrap(err).(codegen.GenerateFileExistsError)
	assert.True(t, ok)
}

func TestTerragruntRemoteStateCodegenDoesNotGenerateWithSkip(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "remote-state", "skip")

	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", generateTestCase))
	assert.False(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
}

func TestTerragruntValidateAllWithVersionChecks(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, "fixture-version-check")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntVersionCommand(t, "v0.23.21", fmt.Sprintf("terragrunt validate-all --terragrunt-non-interactive --terragrunt-working-dir %s", tmpEnvPath), &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

func TestTerragruntVersionConstraints(t *testing.T) {
	testCases := []struct {
		name                 string
		terragruntVersion    string
		terragruntConstraint string
		shouldSucceed        bool
	}{
		{
			"version meets constraint equal",
			"v0.23.18",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			true,
		},
		{
			"version meets constriant greater patch",
			"v0.23.19",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			true,
		},
		{
			"version meets constriant greater major",
			"v1.0.0",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			true,
		},
		{
			"version meets constriant less patch",
			"v0.23.17",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			false,
		},
		{
			"version meets constriant less major",
			"v0.22.18",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			false,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {

			tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_READ_CONFIG)
			rootPath := filepath.Join(tmpEnvPath, TEST_FIXTURE_READ_CONFIG, "with_constraints")

			tmpTerragruntConfigPath := createTmpTerragruntConfigContent(t, testCase.terragruntConstraint, config.DefaultTerragruntConfigPath)

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}

			err := runTerragruntVersionCommand(t, testCase.terragruntVersion, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, rootPath), &stdout, &stderr)
			logBufferContentsLineByLine(t, stdout, "stdout")
			logBufferContentsLineByLine(t, stderr, "stderr")

			if testCase.shouldSucceed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestTerragruntVersionConstraintsPartialParse(t *testing.T) {
	fixturePath := "fixture-partial-parse/terragrunt-version-constraint"
	cleanupTerragruntFolder(t, fixturePath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntVersionCommand(t, "0.21.23", fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", fixturePath), &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")

	assert.Error(t, err)

	_, isTgVersionError := errors.Unwrap(err).(cli.InvalidTerragruntVersion)
	assert.True(t, isTgVersionError)
}

func cleanupTerraformFolder(t *testing.T, templatesPath string) {
	removeFile(t, util.JoinPath(templatesPath, TERRAFORM_STATE))
	removeFile(t, util.JoinPath(templatesPath, TERRAFORM_STATE_BACKUP))
	removeFile(t, util.JoinPath(templatesPath, TERRAGRUNT_DEBUG_FILE))
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
	return runTerragruntVersionCommand(t, "TEST", command, writer, errwriter)
}

func runTerragruntVersionCommand(t *testing.T, version string, command string, writer io.Writer, errwriter io.Writer) error {
	args := strings.Split(command, " ")

	app := cli.CreateTerragruntCli(version, writer, errwriter)
	return app.Run(args)
}

func runTerragrunt(t *testing.T, command string) {
	runTerragruntRedirectOutput(t, command, os.Stdout, os.Stderr)
}

func runTerragruntCommandWithOutput(t *testing.T, command string) (string, string, error) {
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, command, &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")
	return stdout.String(), stderr.String(), err
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

func createTmpTerragruntConfigContent(t *testing.T, contents string, configFileName string) string {
	tmpFolder, err := ioutil.TempDir("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp folder due to error: %v", err)
	}

	tmpTerragruntConfigFile := util.JoinPath(tmpFolder, configFileName)

	if err := ioutil.WriteFile(tmpTerragruntConfigFile, []byte(contents), 0444); err != nil {
		t.Fatalf("Error writing temp Terragrunt config to %s: %v", tmpTerragruntConfigFile, err)
	}

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
	contents = strings.Replace(contents, "__FILL_IN_LOGS_BUCKET_NAME__", s3BucketName+"-tf-state-logs", -1)

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
	assert.True(t, remote.DoesS3BucketExist(s3Client, &remoteStateConfig.Bucket), "Terragrunt failed to create remote state S3 bucket %s", bucketName)

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

// deleteS3BucketWithRetry will attempt to delete the specified S3 bucket, retrying up to 3 times if there are errors to
// handle eventual consistency issues.
func deleteS3BucketWithRetry(t *testing.T, awsRegion string, bucketName string) {
	for i := 0; i < 3; i++ {
		err := deleteS3BucketE(t, awsRegion, bucketName)
		if err == nil {
			return
		}

		t.Logf("Error deleting s3 bucket %s. Sleeping for 10 seconds before retrying.", bucketName)
		time.Sleep(10 * time.Second)
	}
	t.Fatalf("Max retries attempting to delete s3 bucket %s in region %s", bucketName, awsRegion)
}

// Delete the specified S3 bucket to clean up after a test
func deleteS3Bucket(t *testing.T, awsRegion string, bucketName string) {
	require.NoError(t, deleteS3BucketE(t, awsRegion, bucketName))
}
func deleteS3BucketE(t *testing.T, awsRegion string, bucketName string) error {
	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		t.Logf("Error creating mockOptions: %v", err)
		return err
	}

	sessionConfig := &aws_helper.AwsSessionConfig{
		Region: awsRegion,
	}

	s3Client, err := remote.CreateS3Client(sessionConfig, mockOptions)
	if err != nil {
		t.Logf("Error creating S3 client: %v", err)
		return err
	}

	t.Logf("Deleting test s3 bucket %s", bucketName)

	out, err := s3Client.ListObjectVersions(&s3.ListObjectVersionsInput{Bucket: aws.String(bucketName)})
	if err != nil {
		t.Logf("Failed to list object versions in s3 bucket %s: %v", bucketName, err)
		return err
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
			t.Logf("Error deleting all versions of all objects in bucket %s: %v", bucketName, err)
			return err
		}
	}

	if _, err := s3Client.DeleteBucket(&s3.DeleteBucketInput{Bucket: aws.String(bucketName)}); err != nil {
		t.Logf("Failed to delete S3 bucket %s: %v", bucketName, err)
		return err
	}
	return nil
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
	assert.NoError(t, err)
}

// Check that the GCS Bucket of the given name and location exists. Terragrunt should create this bucket during the test.
// Also check if bucket got labeled properly.
func validateGCSBucketExistsAndIsLabeled(t *testing.T, location string, bucketName string, expectedLabels map[string]string) {
	remoteStateConfig := remote.RemoteStateConfigGCS{Bucket: bucketName}

	gcsClient, err := remote.CreateGCSClient(remoteStateConfig)
	if err != nil {
		t.Fatalf("Error creating GCS client: %v", err)
	}

	// verify the bucket exists
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
	var gcsConfig remote.RemoteStateConfigGCS
	gcsClient, err := remote.CreateGCSClient(gcsConfig)
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
	var gcsConfig remote.RemoteStateConfigGCS
	gcsClient, err := remote.CreateGCSClient(gcsConfig)
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

func fileIsInFolder(t *testing.T, name string, path string) bool {
	found := false
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		assert.NoError(t, err)
		if filepath.Base(path) == name {
			found = true
		}
		return nil
	})
	require.NoError(t, err)
	return found
}

func runValidateAllWithIncludeAndGetIncludedModules(t *testing.T, rootModulePath string, includeModulePaths []string, strictInclude bool) []string {
	cmd_parts := []string{
		"terragrunt", "run-all", "validate",
		"--terragrunt-non-interactive",
		"--terragrunt-log-level", "debug",
		"--terragrunt-working-dir", rootModulePath,
	}

	for _, module := range includeModulePaths {
		cmd_parts = append(cmd_parts, "--terragrunt-include-dir", module)
	}

	if strictInclude {
		cmd_parts = append(cmd_parts, "--terragrunt-strict-include")
	}

	cmd := strings.Join(cmd_parts, " ")

	validateAllStdout := bytes.Buffer{}
	validateAllStderr := bytes.Buffer{}
	err := runTerragruntCommand(
		t,
		cmd,
		&validateAllStdout,
		&validateAllStderr,
	)
	logBufferContentsLineByLine(t, validateAllStdout, "validate-all stdout")
	logBufferContentsLineByLine(t, validateAllStderr, "validate-all stderr")
	require.NoError(t, err)

	currentDir, err := os.Getwd()
	require.NoError(t, err)

	includedModulesRegexp, err := regexp.Compile(
		fmt.Sprintf(
			`=> Module %s/%s/(.+) \(excluded: (true|false)`,
			currentDir,
			rootModulePath,
		),
	)
	require.NoError(t, err)
	matches := includedModulesRegexp.FindAllStringSubmatch(string(validateAllStderr.Bytes()), -1)
	includedModules := []string{}
	for _, match := range matches {
		if match[2] == "false" {
			includedModules = append(includedModules, match[1])
		}
	}
	sort.Strings(includedModules)
	return includedModules
}

// sops decrypting for inputs
func TestSopsDecryptedCorrectly(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_SOPS)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_SOPS)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_SOPS)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))

	assert.Equal(t, outputs["json_bool_array"].Value, []interface{}{true, false})
	assert.Equal(t, outputs["json_string_array"].Value, []interface{}{"example_value1", "example_value2"})
	assert.Equal(t, outputs["json_number"].Value, 1234.56789)
	assert.Equal(t, outputs["json_string"].Value, "example_value")
	assert.Equal(t, outputs["json_hello"].Value, "Welcome to SOPS! Edit this file as you please!")
	assert.Equal(t, outputs["yaml_bool_array"].Value, []interface{}{true, false})
	assert.Equal(t, outputs["yaml_string_array"].Value, []interface{}{"example_value1", "example_value2"})
	assert.Equal(t, outputs["yaml_number"].Value, 1234.5679)
	assert.Equal(t, outputs["yaml_string"].Value, "example_value")
	assert.Equal(t, outputs["yaml_hello"].Value, "Welcome to SOPS! Edit this file as you please!")
}

func TestTerragruntRunAllCommandPrompt(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_OUTPUT_ALL)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt run-all apply --terragrunt-working-dir %s", environmentPath), &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")
	assert.Contains(t, stderr.String(), "Are you sure you want to run 'terragrunt apply' in each folder of the stack described above? (y/n)")
	assert.Error(t, err)
}
