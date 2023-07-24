package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
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
	TERRAFORM_REMOTE_STATE_S3_REGION                                         = "us-west-2"
	TERRAFORM_REMOTE_STATE_GCP_REGION                                        = "eu"
	TEST_FIXTURE_PATH                                                        = "fixture/"
	TEST_FIXTURE_CODEGEN_PATH                                                = "fixture-codegen"
	TEST_FIXTURE_GCS_PATH                                                    = "fixture-gcs/"
	TEST_FIXTURE_GCS_BYO_BUCKET_PATH                                         = "fixture-gcs-byo-bucket/"
	TEST_FIXTURE_STACK                                                       = "fixture-stack/"
	TEST_FIXTURE_GRAPH_DEPENDENCIES                                          = "fixture-graph-dependencies"
	TEST_FIXTURE_OUTPUT_ALL                                                  = "fixture-output-all"
	TEST_FIXTURE_OUTPUT_FROM_REMOTE_STATE                                    = "fixture-output-from-remote-state"
	TEST_FIXTURE_OUTPUT_FROM_DEPENDENCY                                      = "fixture-output-from-dependency"
	TEST_FIXTURE_STDOUT                                                      = "fixture-download/stdout-test"
	TEST_FIXTURE_EXTRA_ARGS_PATH                                             = "fixture-extra-args/"
	TEST_FIXTURE_ENV_VARS_BLOCK_PATH                                         = "fixture-env-vars-block/"
	TEST_FIXTURE_SKIP                                                        = "fixture-skip/"
	TEST_FIXTURE_CONFIG_SINGLE_JSON_PATH                                     = "fixture-config-files/single-json-config"
	TEST_FIXTURE_PREVENT_DESTROY_OVERRIDE                                    = "fixture-prevent-destroy-override/child"
	TEST_FIXTURE_PREVENT_DESTROY_NOT_SET                                     = "fixture-prevent-destroy-not-set/child"
	TEST_FIXTURE_LOCAL_PREVENT_DESTROY                                       = "fixture-download/local-with-prevent-destroy"
	TEST_FIXTURE_LOCAL_PREVENT_DESTROY_DEPENDENCIES                          = "fixture-download/local-with-prevent-destroy-dependencies"
	TEST_FIXTURE_LOCAL_INCLUDE_PREVENT_DESTROY_DEPENDENCIES                  = "fixture-download/local-include-with-prevent-destroy-dependencies"
	TEST_FIXTURE_NOT_EXISTING_SOURCE                                         = "fixture-download/invalid-path"
	TEST_FIXTURE_EXTERNAL_DEPENDENCIE                                        = "fixture-external-dependencies"
	TEST_FIXTURE_MISSING_DEPENDENCIE                                         = "fixture-missing-dependencies/main"
	TEST_FIXTURE_GET_OUTPUT                                                  = "fixture-get-output"
	TEST_FIXTURE_HOOKS_BEFORE_ONLY_PATH                                      = "fixture-hooks/before-only"
	TEST_FIXTURE_HOOKS_ALL_PATH                                              = "fixture-hooks/all"
	TEST_FIXTURE_HOOKS_AFTER_ONLY_PATH                                       = "fixture-hooks/after-only"
	TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_PATH                                 = "fixture-hooks/before-and-after"
	TEST_FIXTURE_HOOKS_BEFORE_AFTER_AND_ERROR_MERGE_PATH                     = "fixture-hooks/before-after-and-error-merge"
	TEST_FIXTURE_HOOKS_SKIP_ON_ERROR_PATH                                    = "fixture-hooks/skip-on-error"
	TEST_FIXTURE_ERROR_HOOKS_PATH                                            = "fixture-hooks/error-hooks"
	TEST_FIXTURE_HOOKS_ONE_ARG_ACTION_PATH                                   = "fixture-hooks/one-arg-action"
	TEST_FIXTURE_HOOKS_EMPTY_STRING_COMMAND_PATH                             = "fixture-hooks/bad-arg-action/empty-string-command"
	TEST_FIXTURE_HOOKS_EMPTY_COMMAND_LIST_PATH                               = "fixture-hooks/bad-arg-action/empty-command-list"
	TEST_FIXTURE_HOOKS_INTERPOLATIONS_PATH                                   = "fixture-hooks/interpolations"
	TEST_FIXTURE_HOOKS_INIT_ONCE_NO_SOURCE_NO_BACKEND                        = "fixture-hooks/init-once/no-source-no-backend"
	TEST_FIXTURE_HOOKS_INIT_ONCE_NO_SOURCE_WITH_BACKEND                      = "fixture-hooks/init-once/no-source-with-backend"
	TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND                      = "fixture-hooks/init-once/with-source-no-backend"
	TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND_SUPPRESS_HOOK_STDOUT = "fixture-hooks/init-once/with-source-no-backend-suppress-hook-stdout"
	TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_WITH_BACKEND                    = "fixture-hooks/init-once/with-source-with-backend"
	TEST_FIXTURE_FAILED_TERRAFORM                                            = "fixture-failure"
	TEST_FIXTURE_EXIT_CODE                                                   = "fixture-exit-code"
	TEST_FIXTURE_AUTO_RETRY_RERUN                                            = "fixture-auto-retry/re-run"
	TEST_FIXTURE_AUTO_RETRY_EXHAUST                                          = "fixture-auto-retry/exhaust"
	TEST_FIXTURE_AUTO_RETRY_CUSTOM_ERRORS                                    = "fixture-auto-retry/custom-errors"
	TEST_FIXTURE_AUTO_RETRY_CUSTOM_ERRORS_NOT_SET                            = "fixture-auto-retry/custom-errors-not-set"
	TEST_FIXTURE_AUTO_RETRY_APPLY_ALL_RETRIES                                = "fixture-auto-retry/apply-all"
	TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES                             = "fixture-auto-retry/configurable-retries"
	TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES_ERROR_1                     = "fixture-auto-retry/configurable-retries-incorrect-retry-attempts"
	TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES_ERROR_2                     = "fixture-auto-retry/configurable-retries-incorrect-sleep-interval"
	TEST_FIXTURE_AWS_PROVIDER_PATCH                                          = "fixture-aws-provider-patch"
	TEST_FIXTURE_INPUTS                                                      = "fixture-inputs"
	TEST_FIXTURE_LOCALS_ERROR_UNDEFINED_LOCAL                                = "fixture-locals-errors/undefined-local"
	TEST_FIXTURE_LOCALS_ERROR_UNDEFINED_LOCAL_BUT_INPUT                      = "fixture-locals-errors/undefined-local-but-input"
	TEST_FIXTURE_LOCALS_CANONICAL                                            = "fixture-locals/canonical"
	TEST_FIXTURE_LOCALS_IN_INCLUDE                                           = "fixture-locals/local-in-include"
	TEST_FIXTURE_LOCAL_RUN_ONCE                                              = "fixture-locals/run-once"
	TEST_FIXTURE_LOCAL_RUN_MULTIPLE                                          = "fixture-locals/run-multiple"
	TEST_FIXTURE_LOCALS_IN_INCLUDE_CHILD_REL_PATH                            = "qa/my-app"
	TEST_FIXTURE_READ_CONFIG                                                 = "fixture-read-config"
	TEST_FIXTURE_READ_IAM_ROLE                                               = "fixture-read-config/iam_role_in_file"
	TEST_FIXTURE_IAM_ROLES_MULTIPLE_MODULES                                  = "fixture-read-config/iam_roles_multiple_modules"
	TEST_FIXTURE_RELATIVE_INCLUDE_CMD                                        = "fixture-relative-include-cmd"
	TEST_FIXTURE_AWS_GET_CALLER_IDENTITY                                     = "fixture-get-aws-caller-identity"
	TEST_FIXTURE_GET_REPO_ROOT                                               = "fixture-get-repo-root"
	TEST_FIXTURE_PATH_RELATIVE_FROM_INCLUDE                                  = "fixture-get-path/fixture-path_relative_from_include"
	TEST_FIXTURE_GET_PATH_FROM_REPO_ROOT                                     = "fixture-get-path/fixture-get-path-from-repo-root"
	TEST_FIXTURE_GET_PATH_TO_REPO_ROOT                                       = "fixture-get-path/fixture-get-path-to-repo-root"
	TEST_FIXTURE_GET_PLATFORM                                                = "fixture-get-platform"
	TEST_FIXTURE_GET_TERRAGRUNT_SOURCE_HCL                                   = "fixture-get-terragrunt-source-hcl"
	TEST_FIXTURE_GET_TERRAGRUNT_SOURCE_CLI                                   = "fixture-get-terragrunt-source-cli"
	TEST_FIXTURE_REGRESSIONS                                                 = "fixture-regressions"
	TEST_FIXTURE_PLANFILE_ORDER                                              = "fixture-planfile-order-test"
	TEST_FIXTURE_DIRS_PATH                                                   = "fixture-dirs"
	TEST_FIXTURE_PARALLELISM                                                 = "fixture-parallelism"
	TEST_FIXTURE_SOPS                                                        = "fixture-sops"
	TEST_FIXTURE_DESTROY_WARNING                                             = "fixture-destroy-warning"
	TEST_FIXTURE_INCLUDE_PARENT                                              = "fixture-include-parent"
	TEST_FIXTURE_AUTO_INIT                                                   = "fixture-download/init-on-source-change"
	TEST_FIXTURE_DISJOINT                                                    = "fixture-stack/disjoint"
	TEST_FIXTURE_BROKEN_LOCALS                                               = "fixture-broken-locals"
	TEST_FIXTURE_BROKEN_DEPENDENCY                                           = "fixture-broken-dependency"
	TEST_FIXTURE_RENDER_JSON_METADATA                                        = "fixture-render-json-metadata"
	TEST_FIXTURE_RENDER_JSON_MOCK_OUTPUTS                                    = "fixture-render-json-mock-outputs"
	TEST_FIXTURE_RENDER_JSON_INPUTS                                          = "fixture-render-json-inputs"
	TEST_FIXTURE_STARTSWITH                                                  = "fixture-startswith"
	TEST_FIXTURE_TIMECMP                                                     = "fixture-timecmp"
	TEST_FIXTURE_TIMECMP_INVALID_TIMESTAMP                                   = "fixture-timecmp-errors/invalid-timestamp"
	TEST_FIXTURE_ENDSWITH                                                    = "fixture-endswith"
	TEST_FIXTURE_TFLINT_NO_ISSUES_FOUND                                      = "fixture-tflint/no-issues-found"
	TEST_FIXTURE_TFLINT_ISSUES_FOUND                                         = "fixture-tflint/issues-found"
	TEST_FIXTURE_TFLINT_NO_CONFIG_FILE                                       = "fixture-tflint/no-config-file"
	TEST_FIXTURE_TFLINT_MODULE_FOUND                                         = "fixture-tflint/module-found"
	TEST_FIXTURE_TFLINT_NO_TF_SOURCE_PATH                                    = "fixture-tflint/no-tf-source"
	TEST_FIXTURE_PARALLEL_RUN                                                = "fixture-parallel-run"
	TEST_FIXTURE_INIT_ERROR                                                  = "fixture-init-error"
	TEST_FIXTURE_MODULE_PATH_ERROR                                           = "fixture-module-path-in-error"
	TEST_FIXTURE_HCLFMT_DIFF                                                 = "fixture-hclfmt-diff"
	TEST_FIXTURE_DESTROY_DEPENDENT_MODULE                                    = "fixture-destroy-dependent-module"
	TEST_FIXTURE_REF_SOURCE                                                  = "fixture-download/remote-ref"
	TEST_FIXTURE_SOURCE_MAP_SLASHES                                          = "fixture-source-map/slashes-in-ref"
	TEST_FIXTURE_STRCONTAINS                                                 = "fixture-strcontains"
	TEST_FIXTURE_INIT_CACHE                                                  = "fixture-init-cache"
	TERRAFORM_BINARY                                                         = "terraform"
	TERRAFORM_FOLDER                                                         = ".terraform"
	TERRAFORM_STATE                                                          = "terraform.tfstate"
	TERRAFORM_STATE_BACKUP                                                   = "terraform.tfstate.backup"
	TERRAGRUNT_CACHE                                                         = ".terragrunt-cache"

	qaMyAppRelPath  = "qa/my-app"
	fixtureDownload = "fixture-download"
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
	output := stdout.String()

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
	output := stdout.String()
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
	output := stdout.String()

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
	output := stdout.String()

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

	output := stdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(t, 0, strings.Count(output, "BEFORE_TERRAGRUNT_READ_CONFIG"), "terragrunt-read-config before_hook should not be triggered")
	t.Logf("output: %s", output)

	assert.Equal(t, 1, strings.Count(output, "AFTER_TERRAGRUNT_READ_CONFIG"), "Hooks on terragrunt-read-config command executed more than once")

	assert.NoError(t, beforeException)
	assert.NoError(t, afterException)
}

func TestTerragruntBeforeAfterAndErrorMergeHook(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(TEST_FIXTURE_HOOKS_BEFORE_AFTER_AND_ERROR_MERGE_PATH, qaMyAppRelPath)
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	t.Logf("bucketName: %s", s3BucketName)
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, TEST_FIXTURE_HOOKS_BEFORE_AFTER_AND_ERROR_MERGE_PATH, qaMyAppRelPath, s3BucketName, config.DefaultTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath), &stdout, &stderr)
	assert.ErrorContains(t, err, "executable file not found in $PATH")

	_, beforeException := ioutil.ReadFile(childPath + "/before.out")
	_, beforeChildException := ioutil.ReadFile(childPath + "/before-child.out")
	_, beforeOverriddenParentException := ioutil.ReadFile(childPath + "/before-parent.out")
	_, afterException := ioutil.ReadFile(childPath + "/after.out")
	_, afterParentException := ioutil.ReadFile(childPath + "/after-parent.out")
	_, errorHookParentException := ioutil.ReadFile(childPath + "/error-hook-parent.out")
	_, errorHookChildException := ioutil.ReadFile(childPath + "/error-hook-child.out")
	_, errorHookOverridenParentException := ioutil.ReadFile(childPath + "/error-hook-merge-parent.out")

	assert.NoError(t, beforeException)
	assert.NoError(t, beforeChildException)
	assert.NoError(t, afterException)
	assert.NoError(t, afterParentException)
	assert.NoError(t, errorHookParentException)
	assert.NoError(t, errorHookChildException)

	// PathError because no file found
	assert.Error(t, beforeOverriddenParentException)
	assert.Error(t, errorHookOverridenParentException)
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

	assert.Error(t, err)

	output := stdout.String()

	assert.Contains(t, output, "BEFORE_SHOULD_DISPLAY")
	assert.NotContains(t, output, "BEFORE_NODISPLAY")

	assert.Contains(t, output, "AFTER_SHOULD_DISPLAY")
	assert.NotContains(t, output, "AFTER_NODISPLAY")

	assert.Contains(t, output, "ERROR_HOOK_EXECUTED")
	assert.NotContains(t, output, "NOT_MATCHING_ERROR_HOOK")
	assert.Contains(t, output, "PATTERN_MATCHING_ERROR_HOOK")
}

func TestTerragruntCatchErrorsInTerraformExecution(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_ERROR_HOOKS_PATH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_ERROR_HOOKS_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_ERROR_HOOKS_PATH)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)

	assert.Error(t, err)

	output := stderr.String()

	assert.Contains(t, output, "pattern_matching_hook")
	assert.Contains(t, output, "catch_all_matching_hook")
	assert.NotContains(t, output, "not_matching_hook")

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
	output := stdout.String()

	homePath := os.Getenv("HOME")
	if homePath == "" {
		homePath = "HelloWorld"
	}

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Contains(t, output, homePath)

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

	encryptionConfig, err := bucketEncryption(t, TERRAFORM_REMOTE_STATE_S3_REGION, targetLoggingBucket)
	assert.NoError(t, err)
	assert.NotNil(t, encryptionConfig)
	assert.NotNil(t, encryptionConfig.ServerSideEncryptionConfiguration)
	for _, rule := range encryptionConfig.ServerSideEncryptionConfiguration.Rules {
		if rule.ApplyServerSideEncryptionByDefault != nil {
			if rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm != nil {
				assert.Equal(t, s3.ServerSideEncryptionAes256, *rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm)
			}
		}
	}

	policy, err := bucketPolicy(t, TERRAFORM_REMOTE_STATE_S3_REGION, targetLoggingBucket)
	assert.NoError(t, err)
	assert.NotNil(t, policy.Policy)

	policyInBucket, err := aws_helper.UnmarshalPolicy(*policy.Policy)
	assert.NoError(t, err)
	enforceSSE := false
	for _, statement := range policyInBucket.Statement {
		if statement.Sid == remote.SidEnforcedTLSPolicy {
			enforceSSE = true
		}
	}
	assert.True(t, enforceSSE)
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

	encryptionConfig, err := bucketEncryption(t, TERRAFORM_REMOTE_STATE_S3_REGION, targetLoggingBucket)
	assert.NoError(t, err)
	assert.NotNil(t, encryptionConfig)
	assert.NotNil(t, encryptionConfig.ServerSideEncryptionConfiguration)
	for _, rule := range encryptionConfig.ServerSideEncryptionConfiguration.Rules {
		if rule.ApplyServerSideEncryptionByDefault != nil {
			if rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm != nil {
				assert.Equal(t, s3.ServerSideEncryptionAes256, *rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm)
			}
		}
	}

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

func TestTerragruntOutputFromRemoteState(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_OUTPUT_FROM_REMOTE_STATE)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_OUTPUT_FROM_REMOTE_STATE, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, TEST_FIXTURE_OUTPUT_FROM_REMOTE_STATE)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-fetch-dependency-output-from-state --auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s/app1", environmentPath))
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-fetch-dependency-output-from-state --auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s/app3", environmentPath))
	// Now delete dependencies cached state
	config.ClearOutputCache()
	require.NoError(t, os.Remove(filepath.Join(environmentPath, "/app1/.terraform/terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(environmentPath, "/app1/.terraform")))
	require.NoError(t, os.Remove(filepath.Join(environmentPath, "/app3/.terraform/terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(environmentPath, "/app3/.terraform")))
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-fetch-dependency-output-from-state --auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s/app2", environmentPath))
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt run-all output --terragrunt-fetch-dependency-output-from-state --terragrunt-non-interactive --terragrunt-working-dir %s", environmentPath), &stdout, &stderr)
	output := stdout.String()

	assert.True(t, strings.Contains(output, "app1 output"))
	assert.True(t, strings.Contains(output, "app2 output"))
	assert.True(t, strings.Contains(output, "app3 output"))

	assert.True(t, (strings.Index(output, "app3 output") < strings.Index(output, "app1 output")) && (strings.Index(output, "app1 output") < strings.Index(output, "app2 output")))
}

func TestTerragruntOutputFromDependency(t *testing.T) {
	// t.Parallel() cannot be used together with t.Setenv()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_OUTPUT_FROM_DEPENDENCY)

	rootTerragruntPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_OUTPUT_FROM_DEPENDENCY)
	depTerragruntConfigPath := util.JoinPath(rootTerragruntPath, "dependency", config.DefaultTerragruntConfigPath)

	copyTerragruntConfigAndFillPlaceholders(t, depTerragruntConfigPath, depTerragruntConfigPath, s3BucketName, "not-used", TERRAFORM_REMOTE_STATE_S3_REGION)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	t.Setenv("AWS_CSM_ENABLED", "true")

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level debug", rootTerragruntPath), &stdout, &stderr)
	assert.NoError(t, err)

	output := stderr.String()
	assert.NotContains(t, output, "invalid character")
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
		err := util.CopyFolderContents(TEST_FIXTURE_PARALLELISM, tmpEnvPath, ".terragrunt-test", nil)
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

	tmpEnvPath, err := filepath.EvalSymlinks(copyEnvironment(t, TEST_FIXTURE_DISJOINT))
	assert.NoError(t, err)
	disjointEnvironmentPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_DISJOINT)

	cleanupTerraformFolder(t, disjointEnvironmentPath)
	runTerragrunt(t, fmt.Sprintf("terragrunt plan-all -out=plan.tfplan --terragrunt-log-level info --terragrunt-non-interactive --terragrunt-working-dir %s", disjointEnvironmentPath))
	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all plan.tfplan --terragrunt-log-level info --terragrunt-non-interactive --terragrunt-working-dir %s", disjointEnvironmentPath))
}

func TestInvalidSource(t *testing.T) {
	t.Parallel()

	generateTestCase := TEST_FIXTURE_NOT_EXISTING_SOURCE
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt init --terragrunt-working-dir %s", generateTestCase), &stdout, &stderr)
	require.Error(t, err)
	_, ok := errors.Unwrap(err).(cli.WorkingDirNotFound)
	assert.True(t, ok)
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

func TestPlanfileOrder(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, TEST_FIXTURE_PLANFILE_ORDER)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_PLANFILE_ORDER)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-working-dir %s", modulePath), os.Stdout, os.Stderr)
	assert.Nil(t, err)

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-working-dir %s", modulePath), os.Stdout, os.Stderr)
	assert.Nil(t, err)
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
	branchName := git.GetCurrentBranchName(t)
	// https://www.terraform.io/docs/language/modules/sources.html#modules-in-package-sub-directories
	// https://github.com/gruntwork-io/terragrunt/issues/1778
	branchName = url.QueryEscape(branchName)
	mainContents = strings.Replace(mainContents, "__BRANCH_NAME__", branchName, -1)
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

	tmpEnvPath := copyEnvironment(t, "fixture-download")
	fixtureRoot := util.JoinPath(tmpEnvPath, TEST_FIXTURE_LOCAL_PREVENT_DESTROY)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", fixtureRoot))

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt destroy -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", fixtureRoot), os.Stdout, os.Stderr)

	if assert.Error(t, err) {
		underlying := errors.Unwrap(err)
		assert.IsType(t, cli.ModuleIsProtected{}, underlying)
	}
}

func TestPreventDestroyApply(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, "fixture-download")

	fixtureRoot := util.JoinPath(tmpEnvPath, TEST_FIXTURE_LOCAL_PREVENT_DESTROY)
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", fixtureRoot))

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -destroy -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", fixtureRoot), os.Stdout, os.Stderr)

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

func TestTerragruntMissingDependenciesFail(t *testing.T) {
	t.Parallel()

	generateTestCase := TEST_FIXTURE_MISSING_DEPENDENCIE
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt init --terragrunt-working-dir %s", generateTestCase), &stdout, &stderr)
	require.Error(t, err)
	parsedError, ok := errors.Unwrap(err).(config.DependencyDirNotFound)
	assert.True(t, ok)
	assert.True(t, len(parsedError.Dir) == 1)
	assert.Contains(t, parsedError.Dir[0], "hl3-release")
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

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND_SUPPRESS_HOOK_STDOUT)
	tmpEnvPath := copyEnvironment(t, "fixture-hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND_SUPPRESS_HOOK_STDOUT)

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

// Test default behavior when mock_outputs_merge_with_state is not set. It should behave, as before this parameter was added
// It will fail on any command if the parent state is not applied, because the state of the parent exists and it alread has an output
// but not the newly added output.
func TestDependencyMockOutputMergeWithStateDefault(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs-merge-with-state", "merge-with-state-default", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", parentPath), &stdout, &stderr)
	assert.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "plan stdout")
	logBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify we have the default behavior if mock_outputs_merge_with_state is not set
	stdout.Reset()
	stderr.Reset()
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	assert.Error(t, err)
	// Verify that we fail because the dependency is not applied yet, and the new attribue is not available and in
	// this case, mocked outputs are not used.
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output2\"")

	logBufferContentsLineByLine(t, stdout, "plan stdout")
	logBufferContentsLineByLine(t, stderr, "plan stderr")

}

// Test when mock_outputs_merge_with_state is explicitly set to false. It should behave, as before this parameter was added
// It will fail on any command if the parent state is not applied, because the state of the parent exists and it alread has an output
// but not the newly added output.
func TestDependencyMockOutputMergeWithStateFalse(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs-merge-with-state", "merge-with-state-false", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", parentPath), &stdout, &stderr)
	assert.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "plan stdout")
	logBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify we have the default behavior if mock_outputs_merge_with_state is set to false
	stdout.Reset()
	stderr.Reset()
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	assert.Error(t, err)
	// Verify that we fail because the dependency is not applied yet, and the new attribue is not available and in
	// this case, mocked outputs are not used.
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output2\"")

	logBufferContentsLineByLine(t, stdout, "plan stdout")
	logBufferContentsLineByLine(t, stderr, "plan stderr")
}

// Test when mock_outputs_merge_with_state is explicitly set to true.
// It will mock the newly added output from the parent as it was not already applied to the state.
func TestDependencyMockOutputMergeWithStateTrue(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs-merge-with-state", "merge-with-state-true", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", parentPath), &stdout, &stderr)
	assert.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "plan stdout")
	logBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify mocked outputs are used if mock_outputs_merge_with_state is set to true and some output in the parent are not applied yet.
	stdout.Reset()
	stderr.Reset()
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	assert.NoError(t, err)

	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")
	// Now check the outputs to make sure they are as expected
	stdout.Reset()
	stderr.Reset()

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))

	assert.Equal(t, outputs["test_output1_from_parent"].Value, "value1")
	assert.Equal(t, outputs["test_output2_from_parent"].Value, "fake-data2")

	logBufferContentsLineByLine(t, stdout, "output stdout")
	logBufferContentsLineByLine(t, stderr, "output stderr")
}

// Test when mock_outputs_merge_with_state is explicitly set to true, but using an unallowed command. It should ignore
// the mock output.
func TestDependencyMockOutputMergeWithStateTrueNotAllowed(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs-merge-with-state", "merge-with-state-true-validate-only", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", parentPath), &stdout, &stderr)
	assert.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "plan stdout")
	logBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify mocked outputs are used if mock_outputs_merge_with_state is set to true with an allowed command and some
	// output in the parent are not applied yet.
	stdout.Reset()
	stderr.Reset()
	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr),
	)

	// ... but not when an unallowed command is used
	require.Error(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr),
	)
}

// Test when mock_outputs_merge_with_state is explicitly set to true.
// Mock should not be used as the parent state was already fully applied.
func TestDependencyMockOutputMergeWithStateNoOverride(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs-merge-with-state", "merge-with-state-no-override", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", parentPath), &stdout, &stderr)
	assert.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "show stdout")
	logBufferContentsLineByLine(t, stderr, "show stderr")

	// Verify mocked outputs are not used if mock_outputs_merge_with_state is set to true and all outputs in the parent have been applied.
	stdout.Reset()
	stderr.Reset()
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	assert.NoError(t, err)

	// Now check the outputs to make sure they are as expected
	stdout.Reset()
	stderr.Reset()

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))

	assert.Equal(t, outputs["test_output1_from_parent"].Value, "value1")
	assert.Equal(t, outputs["test_output2_from_parent"].Value, "value2")

	logBufferContentsLineByLine(t, stdout, "show stdout")
	logBufferContentsLineByLine(t, stderr, "show stderr")
}

// Test when mock_outputs_merge_strategy_with_state or mock_outputs_merge_with_state is not set, the default is no_merge
func TestDependencyMockOutputMergeStrategyWithStateDefault(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-default", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output_list_string\"")
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")
}

// Test when mock_outputs_merge_with_state = "false" that MergeStrategyType is set to no_merge
func TestDependencyMockOutputMergeStrategyWithStateCompatFalse(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-compat-false", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output_list_string\"")
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")
}

// Test when mock_outputs_merge_with_state = "true" that MergeStrategyType is set to shallow
func TestDependencyMockOutputMergeStrategyWithStateCompatTrue(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-compat-true", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	assert.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(t, runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr))
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	logBufferContentsLineByLine(t, stdout, "output stdout")
	logBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "map_root1_sub1_value", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "map_root1", "map_root1_sub1", "value"))
	assert.Equal(t, nil, util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "not_in_state", "abc", "value"))
	assert.Equal(t, "fake-list-data", util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"))
	assert.Equal(t, nil, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test when both mock_outputs_merge_with_state and mock_outputs_merge_strategy_with_state are set, mock_outputs_merge_strategy_with_state is used
func TestDependencyMockOutputMergeStrategyWithStateCompatConflict(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-compat-true", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	assert.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(t, runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr))
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	logBufferContentsLineByLine(t, stdout, "output stdout")
	logBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "map_root1_sub1_value", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "map_root1", "map_root1_sub1", "value"))
	assert.Equal(t, nil, util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "not_in_state", "abc", "value"))
	assert.Equal(t, "fake-list-data", util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"))
	assert.Equal(t, nil, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test when mock_outputs_merge_strategy_with_state = "no_merge" that mocks are not merged into the current state
func TestDependencyMockOutputMergeStrategyWithStateNoMerge(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-no-merge", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output_list_string\"")
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")
}

// Test when mock_outputs_merge_strategy_with_state = "shallow" that only top level outputs are merged.
// Lists or keys in existing maps will not be merged
func TestDependencyMockOutputMergeStrategyWithStateShallow(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-shallow", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	assert.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(t, runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr))
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	logBufferContentsLineByLine(t, stdout, "output stdout")
	logBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "map_root1_sub1_value", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "map_root1", "map_root1_sub1", "value"))
	assert.Equal(t, nil, util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "not_in_state", "abc", "value"))
	assert.Equal(t, "fake-list-data", util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"))
	assert.Equal(t, nil, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test when mock_outputs_merge_strategy_with_state = "deep" that the existing state is deeply merged into the mocks
// so that the existing state overwrites the mocks. This allows child modules to use new dependency outputs before the
// dependency has been applied
func TestDependencyMockOutputMergeStrategyWithStateDeepMapOnly(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-deep-map-only", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	assert.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(t, runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", childPath), &stdout, &stderr))
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	logBufferContentsLineByLine(t, stdout, "output stdout")
	logBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "fake-abc", outputs["test_output2_from_parent"].Value)
	assert.Equal(t, "map_root1_sub1_value", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "map_root1", "map_root1_sub1", "value"))
	assert.Equal(t, "fake-abc", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "not_in_state", "abc", "value"))
	assert.Equal(t, "a", util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"))
	assert.Equal(t, nil, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
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

func TestGetRepoRoot(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_REPO_ROOT)
	tmpEnvPath, _ := filepath.EvalSymlinks(copyEnvironment(t, TEST_FIXTURE_GET_REPO_ROOT))
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_REPO_ROOT)

	output, err := exec.Command("git", "init", rootPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(output))
	}
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

	repoRoot, ok := outputs["repo_root"]

	require.True(t, ok)
	require.Regexp(t, "/tmp/terragrunt-.*/fixture-get-repo-root", repoRoot.Value)
}

func TestPathRelativeFromInclude(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_PATH_RELATIVE_FROM_INCLUDE)
	tmpEnvPath, _ := filepath.EvalSymlinks(copyEnvironment(t, TEST_FIXTURE_PATH_RELATIVE_FROM_INCLUDE))
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_PATH_RELATIVE_FROM_INCLUDE, "lives/dev")
	basePath := util.JoinPath(rootPath, "base")
	clusterPath := util.JoinPath(rootPath, "cluster")

	output, err := exec.Command("git", "init", tmpEnvPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(output))
	}

	runTerragrunt(t, fmt.Sprintf("terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", clusterPath), &stdout, &stderr)
	assert.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))

	val, hasVal := outputs["some_output"]
	require.True(t, hasVal)
	require.Equal(t, "something else", val.Value)

	// try to destroy module and check if warning is printed in output, also test `get_parent_terragrunt_dir()` func in the parent terragrunt config.
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt destroy -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", basePath), &stdout, &stderr)
	assert.NoError(t, err)

	assert.Contains(t, stderr.String(), fmt.Sprintf("Detected dependent modules:\n%s", clusterPath))
}

func TestGetPathFromRepoRoot(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_PATH_FROM_REPO_ROOT)
	tmpEnvPath, _ := filepath.EvalSymlinks(copyEnvironment(t, TEST_FIXTURE_GET_PATH_FROM_REPO_ROOT))
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_PATH_FROM_REPO_ROOT)

	output, err := exec.Command("git", "init", tmpEnvPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(output))
	}

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

	pathFromRoot, hasPathFromRoot := outputs["path_from_root"]

	require.True(t, hasPathFromRoot)
	require.Equal(t, TEST_FIXTURE_GET_PATH_FROM_REPO_ROOT, pathFromRoot.Value)
}

func TestGetPathToRepoRoot(t *testing.T) {
	t.Parallel()

	tmpEnvPath, _ := filepath.EvalSymlinks(copyEnvironment(t, TEST_FIXTURE_GET_PATH_TO_REPO_ROOT))
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_PATH_TO_REPO_ROOT)
	cleanupTerraformFolder(t, rootPath)

	output, err := exec.Command("git", "init", tmpEnvPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(output))
	}
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

	pathToRoot, hasPathToRoot := outputs["path_to_root"]

	require.True(t, hasPathToRoot)
	require.Equal(t, pathToRoot.Value, "../../")
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
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Initializing provider plugins")

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	require.NoError(t, err)
	assert.NotContains(t, stdout.String(), "Initializing provider plugins")
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
			"paths": []interface{}{"../../fixture"},
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
				"disable":           false,
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
			"source":          "./delorean",
			"include_in_copy": []interface{}{"time_machine.*"},
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
					"name":            "before_hook_1",
					"commands":        []interface{}{"apply", "plan"},
					"execute":         []interface{}{"touch", "before.out"},
					"working_dir":     nil,
					"run_on_error":    true,
					"suppress_stdout": nil,
				},
			},
			"after_hook": map[string]interface{}{
				"after_hook_1": map[string]interface{}{
					"name":            "after_hook_1",
					"commands":        []interface{}{"apply", "plan"},
					"execute":         []interface{}{"touch", "after.out"},
					"working_dir":     nil,
					"run_on_error":    true,
					"suppress_stdout": nil,
				},
			},
			"error_hook": map[string]interface{}{},
		},
	)
}

func logBufferContentsLineByLine(t *testing.T, out bytes.Buffer, label string) {
	t.Helper()
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

func TestTerragruntGenerateBlockSameNameFail(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "generate-block", "same_name_error")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt init --terragrunt-working-dir %s", generateTestCase), &stdout, &stderr)
	require.Error(t, err)
	parsedError, ok := errors.Unwrap(err).(config.DuplicatedGenerateBlocks)
	assert.True(t, ok)
	assert.True(t, len(parsedError.BlockName) == 1)
	assert.Contains(t, parsedError.BlockName, "backend")
}

func TestTerragruntGenerateBlockMultipleSameNameFail(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "generate-block", "same_name_pair_error")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt init --terragrunt-working-dir %s", generateTestCase), &stdout, &stderr)
	require.Error(t, err)
	parsedError, ok := errors.Unwrap(err).(config.DuplicatedGenerateBlocks)
	assert.True(t, ok)
	assert.True(t, len(parsedError.BlockName) == 2)
	assert.Contains(t, parsedError.BlockName, "backend")
	assert.Contains(t, parsedError.BlockName, "backend2")
}

func TestTerragruntGenerateBlockDisable(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "generate-block", "disable")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt init --terragrunt-working-dir %s", generateTestCase), &stdout, &stderr)
	require.NoError(t, err)
	assert.False(t, fileIsInFolder(t, "data.txt", generateTestCase))
}

func TestTerragruntGenerateBlockEnable(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(TEST_FIXTURE_CODEGEN_PATH, "generate-block", "enable")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt init --terragrunt-working-dir %s", generateTestCase), &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, fileIsInFolder(t, "data.txt", generateTestCase))
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

func TestTerragruntIncludeParentHclFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_INCLUDE_PARENT)
	tmpEnvPath = path.Join(tmpEnvPath, TEST_FIXTURE_INCLUDE_PARENT)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt run-all apply --terragrunt-modules-that-include parent.hcl --terragrunt-modules-that-include common.hcl --terragrunt-non-interactive --terragrunt-working-dir %s", tmpEnvPath), &stdout, &stderr)
	require.NoError(t, err)

	out := stdout.String()
	assert.Equal(t, 1, strings.Count(out, "parent_hcl_file"))
	assert.Equal(t, 1, strings.Count(out, "dependency_hcl"))
	assert.Equal(t, 1, strings.Count(out, "common_hcl"))
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

func TestReadTerragruntConfigIamRole(t *testing.T) {
	t.Parallel()

	identityArn, err := aws_helper.GetAWSIdentityArn(nil, &options.TerragruntOptions{})
	assert.NoError(t, err)

	cleanupTerraformFolder(t, TEST_FIXTURE_READ_IAM_ROLE)

	// Execution outputs to be verified
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	// Invoke terragrunt and verify used IAM role
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt init --terragrunt-working-dir %s", TEST_FIXTURE_READ_IAM_ROLE), &stdout, &stderr)

	// Since are used not existing AWS accounts, for validation are used success and error outputs
	output := fmt.Sprintf("%v %v %v", string(stderr.Bytes()), string(stdout.Bytes()), err.Error())

	// Check that output contains value defined in IAM role
	assert.Equal(t, 1, strings.Count(output, "666666666666"))
	// Ensure that state file wasn't created with default IAM value
	assert.True(t, util.FileNotExists(util.JoinPath(TEST_FIXTURE_READ_IAM_ROLE, identityArn+".txt")))
}

func TestIamRolesLoadingFromDifferentModules(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_IAM_ROLES_MULTIPLE_MODULES)

	// Execution outputs to be verified
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	// Invoke terragrunt and verify used IAM roles for each dependency
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt init --terragrunt-log-level debug --terragrunt-debugreset --terragrunt-working-dir %s", TEST_FIXTURE_IAM_ROLES_MULTIPLE_MODULES), &stdout, &stderr)

	// Taking all outputs in one string
	output := fmt.Sprintf("%v %v %v", string(stderr.Bytes()), string(stdout.Bytes()), err.Error())

	component1 := ""
	component2 := ""

	// scan each output line and get lines for component1 and component2
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "Assuming IAM role arn:aws:iam::component1:role/terragrunt") {
			component1 = line
			continue
		}
		if strings.Contains(line, "Assuming IAM role arn:aws:iam::component2:role/terragrunt") {
			component2 = line
			continue
		}
	}
	assert.NotEmptyf(t, component1, "Missing role for component 1")
	assert.NotEmptyf(t, component2, "Missing role for component 2")

	assert.Contains(t, component1, "iam_roles_multiple_modules/component")
	assert.Contains(t, component2, "iam_roles_multiple_modules/component2")
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

func TestLogFailedLocalsEvaluation(t *testing.T) {
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level debug", TEST_FIXTURE_BROKEN_LOCALS), &stdout, &stderr)
	output := stderr.String()

	assert.Error(t, err)
	assert.Contains(t, output, "Encountered error while evaluating locals in file fixture-broken-locals/terragrunt.hcl")
}

func TestLogFailingDependencies(t *testing.T) {
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	path := filepath.Join(TEST_FIXTURE_BROKEN_DEPENDENCY, "app")

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level debug", path), &stdout, &stderr)
	output := stderr.String()

	assert.Error(t, err)
	assert.Contains(t, output, "Terraform invocation failed in fixture-broken-dependency/dependency")
}

func cleanupTerraformFolder(t *testing.T, templatesPath string) {
	removeFile(t, util.JoinPath(templatesPath, TERRAFORM_STATE))
	removeFile(t, util.JoinPath(templatesPath, TERRAFORM_STATE_BACKUP))
	removeFile(t, util.JoinPath(templatesPath, terragruntDebugFile))
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

	fmt.Println("runTerragruntVersionCommand after split")
	fmt.Println(args)
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

		t.Fatalf("Failed to run Terragrunt command '%s' due to error: %s\n\nStdout: %s\n\nStderr: %s", command, errors.PrintErrorWithStackTrace(err), stdout, stderr)
	}
}

func copyEnvironment(t *testing.T, environmentPath string) string {
	tmpDir, err := ioutil.TempDir("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	t.Logf("Copying %s to %s", environmentPath, tmpDir)

	require.NoError(t, util.CopyFolderContents(environmentPath, util.JoinPath(tmpDir, environmentPath), ".terragrunt-test", nil))

	return tmpDir
}

func copyEnvironmentWithTflint(t *testing.T, environmentPath string) string {
	tmpDir, err := ioutil.TempDir("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	t.Logf("Copying %s to %s", environmentPath, tmpDir)

	require.NoError(t, util.CopyFolderContents(environmentPath, util.JoinPath(tmpDir, environmentPath), ".terragrunt-test", []string{".tflint.hcl"}))

	return tmpDir
}

func copyEnvironmentToPath(t *testing.T, environmentPath, targetPath string) {
	if err := os.MkdirAll(targetPath, 0777); err != nil {
		t.Fatalf("Failed to create temp dir %s due to error %v", targetPath, err)
	}

	copyErr := util.CopyFolderContents(environmentPath, util.JoinPath(targetPath, environmentPath), ".terragrunt-test", nil)
	require.NoError(t, copyErr)
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

// createS3BucketE create test S3 bucket.
func createS3BucketE(t *testing.T, awsRegion string, bucketName string) error {
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

	t.Logf("Creating test s3 bucket %s", bucketName)
	if _, err := s3Client.CreateBucket(&s3.CreateBucketInput{Bucket: aws.String(bucketName)}); err != nil {
		t.Logf("Failed to create S3 bucket %s: %v", bucketName, err)
		return err
	}
	return nil
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

func bucketEncryption(t *testing.T, awsRegion string, bucketName string) (*s3.GetBucketEncryptionOutput, error) {
	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		t.Logf("Error creating mockOptions: %v", err)
		return nil, err
	}

	sessionConfig := &aws_helper.AwsSessionConfig{
		Region: awsRegion,
	}

	s3Client, err := remote.CreateS3Client(sessionConfig, mockOptions)
	if err != nil {
		t.Logf("Error creating S3 client: %v", err)
		return nil, err
	}

	input := &s3.GetBucketEncryptionInput{Bucket: aws.String(bucketName)}
	output, err := s3Client.GetBucketEncryption(input)
	if err != nil {
		return nil, nil
	}

	return output, nil
}

func bucketPolicy(t *testing.T, awsRegion string, bucketName string) (*s3.GetBucketPolicyOutput, error) {
	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		t.Logf("Error creating mockOptions: %v", err)
		return nil, err
	}

	sessionConfig := &aws_helper.AwsSessionConfig{
		Region: awsRegion,
	}

	s3Client, err := remote.CreateS3Client(sessionConfig, mockOptions)
	if err != nil {
		return nil, err
	}
	policyOutput, err := s3Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return nil, err
	}
	return policyOutput, nil
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

	require.NoError(t, err)

	includedModulesRegexp, err := regexp.Compile(
		fmt.Sprintf(
			`=> Module %s/(.+) \(excluded: (true|false)`,
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
	assert.Equal(t, outputs["text_value"].Value, "Raw Secret Example")
	assert.Contains(t, outputs["env_value"].Value, "DB_PASSWORD=tomato")
	assert.Contains(t, outputs["ini_value"].Value, "password = potato")
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

func TestTerragruntInitOnce(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_RUN_ONCE)
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	runTerragruntCommand(t, fmt.Sprintf("terragrunt init --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_RUN_ONCE), &stdout, &stderr)

	errout := string(stdout.Bytes())

	assert.Equal(t, 1, strings.Count(errout, "foo"))
}

func TestTerragruntInitRunCmd(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_RUN_MULTIPLE)
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	runTerragruntCommand(t, fmt.Sprintf("terragrunt init --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_RUN_MULTIPLE), &stdout, &stderr)

	errout := string(stdout.Bytes())

	// Check for cached values between locals and inputs sections
	assert.Equal(t, 1, strings.Count(errout, "potato"))
	assert.Equal(t, 1, strings.Count(errout, "carrot"))
	assert.Equal(t, 1, strings.Count(errout, "bar"))
	assert.Equal(t, 1, strings.Count(errout, "foo"))

	assert.Equal(t, 1, strings.Count(errout, "input_variable"))

	// Commands executed multiple times because of different arguments
	assert.Equal(t, 4, strings.Count(errout, "uuid"))
	assert.Equal(t, 6, strings.Count(errout, "random_arg"))
	assert.Equal(t, 4, strings.Count(errout, "another_arg"))
}

func TestShowWarningWithDependentModulesBeforeDestroy(t *testing.T) {

	rootPath := copyEnvironment(t, TEST_FIXTURE_DESTROY_WARNING)

	rootPath = util.JoinPath(rootPath, TEST_FIXTURE_DESTROY_WARNING)
	vpcPath := util.JoinPath(rootPath, "vpc")
	appPath := util.JoinPath(rootPath, "app")

	cleanupTerraformFolder(t, rootPath)
	cleanupTerraformFolder(t, vpcPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt run-all init --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	assert.NoError(t, err)
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt run-all apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	assert.NoError(t, err)

	// try to destroy vpc module and check if warning is printed in output
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt destroy --terragrunt-non-interactive --terragrunt-working-dir %s", vpcPath), &stdout, &stderr)
	assert.NoError(t, err)

	output := string(stderr.Bytes())
	assert.Equal(t, 1, strings.Count(output, appPath))
}

func TestShowErrorWhenRunAllInvokedWithoutArguments(t *testing.T) {
	t.Parallel()

	appPath := TEST_FIXTURE_STACK

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt run-all --terragrunt-non-interactive --terragrunt-working-dir %s", appPath), &stdout, &stderr)
	require.Error(t, err)
	_, ok := errors.Unwrap(err).(cli.MissingCommand)
	assert.True(t, ok)
}

func TestPathRelativeToIncludeInvokedInCorrectPathFromChild(t *testing.T) {
	t.Parallel()

	appPath := path.Join(TEST_FIXTURE_RELATIVE_INCLUDE_CMD, "app")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt version --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir %s", appPath), &stdout, &stderr)
	require.NoError(t, err)
	output := string(stdout.Bytes())
	assert.Equal(t, 1, strings.Count(output, "path_relative_to_inclue: app\n"))
	assert.Equal(t, 0, strings.Count(output, "path_relative_to_inclue: .\n"))
}

func TestTerragruntInitConfirmation(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_OUTPUT_ALL)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_OUTPUT_ALL, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt run-all init --terragrunt-working-dir %s", tmpEnvPath), &stdout, &stderr)
	require.Error(t, err)
	errout := string(stderr.Bytes())
	assert.Equal(t, 1, strings.Count(errout, "does not exist or you don't have permissions to access it. Would you like Terragrunt to create it? (y/n)"))
}

func TestNoMultipleInitsWithoutSourceChange(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, fixtureDownload)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_STDOUT)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", testPath), &stdout, &stderr)
	require.NoError(t, err)
	// providers initialization during first plan
	assert.Equal(t, 1, strings.Count(stdout.String(), "Terraform has been successfully initialized!"))

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", testPath), &stdout, &stderr)
	require.NoError(t, err)
	// no initialization expected for second plan run
	// https://github.com/gruntwork-io/terragrunt/issues/1921
	assert.Equal(t, 0, strings.Count(stdout.String(), "Terraform has been successfully initialized!"))
}

func TestAutoInitWhenSourceIsChanged(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, fixtureDownload)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_AUTO_INIT)

	terragruntHcl := util.JoinPath(testPath, "terragrunt.hcl")
	contents, err := util.ReadFileAsString(terragruntHcl)
	if err != nil {
		require.NoError(t, err)
	}
	updatedHcl := strings.Replace(contents, "__TAG_VALUE__", "v0.35.1", -1)
	require.NoError(t, ioutil.WriteFile(terragruntHcl, []byte(updatedHcl), 0444))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", testPath), &stdout, &stderr)
	require.NoError(t, err)
	// providers initialization during first plan
	assert.Equal(t, 1, strings.Count(stdout.String(), "Terraform has been successfully initialized!"))

	updatedHcl = strings.Replace(contents, "__TAG_VALUE__", "v0.35.2", -1)
	require.NoError(t, ioutil.WriteFile(terragruntHcl, []byte(updatedHcl), 0444))

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", testPath), &stdout, &stderr)
	require.NoError(t, err)
	// auto initialization when source is changed
	assert.Equal(t, 1, strings.Count(stdout.String(), "Terraform has been successfully initialized!"))
}

func TestRenderJsonAttributesMetadata(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_RENDER_JSON_METADATA)
	cleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "attributes")

	terragruntHcl := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "attributes", "terragrunt.hcl")

	var expectedMetadata = map[string]interface{}{
		"found_in_file": terragruntHcl,
	}

	jsonOut := filepath.Join(tmpDir, "terragrunt_rendered.json")

	runTerragrunt(t, fmt.Sprintf("terragrunt render-json --with-metadata --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s  --terragrunt-json-out %s", tmpDir, jsonOut))

	jsonBytes, err := ioutil.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJson = map[string]interface{}{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJson))

	var inputs = renderedJson[config.MetadataInputs]
	var expectedInputs = map[string]interface{}{
		"name": map[string]interface{}{
			"metadata": expectedMetadata,
			"value":    "us-east-1-bucket",
		},
		"region": map[string]interface{}{
			"metadata": expectedMetadata,
			"value":    "us-east-1",
		},
	}
	assert.True(t, reflect.DeepEqual(expectedInputs, inputs))

	var locals = renderedJson[config.MetadataLocals]
	var expectedLocals = map[string]interface{}{
		"aws_region": map[string]interface{}{
			"metadata": expectedMetadata,
			"value":    "us-east-1",
		},
	}
	assert.True(t, reflect.DeepEqual(expectedLocals, locals))

	var downloadDir = renderedJson[config.MetadataDownloadDir]
	var expecteDownloadDir = map[string]interface{}{
		"metadata": expectedMetadata,
		"value":    "/tmp",
	}
	assert.True(t, reflect.DeepEqual(expecteDownloadDir, downloadDir))

	var iamAssumeRoleDuration = renderedJson[config.MetadataIamAssumeRoleDuration]
	expectedIamAssumeRoleDuration := map[string]interface{}{
		"metadata": expectedMetadata,
		"value":    float64(666),
	}
	assert.True(t, reflect.DeepEqual(expectedIamAssumeRoleDuration, iamAssumeRoleDuration))

	var iamAssumeRoleName = renderedJson[config.MetadataIamAssumeRoleSessionName]
	expectedIamAssumeRoleName := map[string]interface{}{
		"metadata": expectedMetadata,
		"value":    "qwe",
	}
	assert.True(t, reflect.DeepEqual(expectedIamAssumeRoleName, iamAssumeRoleName))

	var iamRole = renderedJson[config.MetadataIamRole]
	expectedIamRole := map[string]interface{}{
		"metadata": expectedMetadata,
		"value":    "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME",
	}
	assert.True(t, reflect.DeepEqual(expectedIamRole, iamRole))

	var preventDestroy = renderedJson[config.MetadataPreventDestroy]
	expectedPreventDestroy := map[string]interface{}{
		"metadata": expectedMetadata,
		"value":    true,
	}
	assert.True(t, reflect.DeepEqual(expectedPreventDestroy, preventDestroy))

	var skip = renderedJson[config.MetadataSkip]
	expectedSkip := map[string]interface{}{
		"metadata": expectedMetadata,
		"value":    true,
	}
	assert.True(t, reflect.DeepEqual(expectedSkip, skip))

	var terraformBinary = renderedJson[config.MetadataTerraformBinary]
	expectedTerraformBinary := map[string]interface{}{
		"metadata": expectedMetadata,
		"value":    "terraform",
	}
	assert.True(t, reflect.DeepEqual(expectedTerraformBinary, terraformBinary))

	var terraformVersionConstraint = renderedJson[config.MetadataTerraformVersionConstraint]
	expectedTerraformVersionConstraint := map[string]interface{}{
		"metadata": expectedMetadata,
		"value":    ">= 0.11",
	}
	assert.True(t, reflect.DeepEqual(expectedTerraformVersionConstraint, terraformVersionConstraint))
}

func TestRenderJsonMetadataDependencies(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_RENDER_JSON_METADATA)
	cleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "dependencies", "app")

	terragruntHcl := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "dependencies", "app", "terragrunt.hcl")
	includeHcl := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "dependencies", "app", "include.hcl")

	var includeMetadata = map[string]interface{}{
		"found_in_file": includeHcl,
	}

	var terragruntMetadata = map[string]interface{}{
		"found_in_file": terragruntHcl,
	}

	jsonOut := filepath.Join(tmpDir, "terragrunt_rendered.json")

	runTerragrunt(t, fmt.Sprintf("terragrunt render-json --with-metadata --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s  --terragrunt-json-out %s", tmpDir, jsonOut))

	jsonBytes, err := ioutil.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJson = map[string]interface{}{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJson))

	var inputs = renderedJson[config.MetadataInputs]
	var expectedInputs = map[string]interface{}{
		"test_input": map[string]interface{}{
			"metadata": includeMetadata,
			"value":    "test_value",
		},
	}
	assert.True(t, reflect.DeepEqual(expectedInputs, inputs))

	var dependencies = renderedJson[config.MetadataDependencies]
	var expectedDependencies = []interface{}{
		map[string]interface{}{
			"metadata": includeMetadata,
			"value":    "../dependency2",
		},
		map[string]interface{}{
			"metadata": terragruntMetadata,
			"value":    "../dependency1",
		},
	}
	assert.True(t, reflect.DeepEqual(expectedDependencies, dependencies))
}

func TestRenderJsonWithMockOutputs(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_RENDER_JSON_MOCK_OUTPUTS)
	cleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_MOCK_OUTPUTS, "app")

	var expectedMetadata = map[string]interface{}{
		"found_in_file": util.JoinPath(tmpDir, "terragrunt.hcl"),
	}

	jsonOut := filepath.Join(tmpDir, "terragrunt_rendered.json")

	runTerragrunt(t, fmt.Sprintf("terragrunt render-json --with-metadata --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s  --terragrunt-json-out %s", tmpDir, jsonOut))

	jsonBytes, err := ioutil.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJson = map[string]interface{}{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJson))

	dependency := renderedJson[config.MetadataDependency]

	var expectedDependency = map[string]interface{}{
		"module": map[string]interface{}{
			"metadata": expectedMetadata,
			"value": map[string]interface{}{
				"config_path": "../dependency",
				"mock_outputs": map[string]interface{}{
					"bastion_host_security_group_id": "123",
					"security_group_id":              "sg-abcd1234",
				},
				"mock_outputs_allowed_terraform_commands": [1]string{"validate"},
				"mock_outputs_merge_strategy_with_state":  nil,
				"mock_outputs_merge_with_state":           nil,
				"name":                                    "module",
				"outputs":                                 nil,
				"skip":                                    nil,
			},
		},
	}
	serializedDependency, err := json.Marshal(dependency)
	assert.NoError(t, err)

	serializedExpectedDependency, err := json.Marshal(expectedDependency)
	assert.NoError(t, err)
	assert.Equal(t, string(serializedExpectedDependency), string(serializedDependency))
}

func TestRenderJsonMetadataIncludes(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_RENDER_JSON_METADATA)
	cleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "includes", "app")

	terragruntHcl := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "includes", "app", "terragrunt.hcl")
	localsHcl := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "includes", "app", "locals.hcl")
	inputHcl := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "includes", "app", "inputs.hcl")
	generateHcl := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "includes", "app", "generate.hcl")
	commonHcl := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "includes", "common", "common.hcl")

	var terragruntMetadata = map[string]interface{}{
		"found_in_file": terragruntHcl,
	}
	var localsMetadata = map[string]interface{}{
		"found_in_file": localsHcl,
	}
	var inputMetadata = map[string]interface{}{
		"found_in_file": inputHcl,
	}
	var generateMetadata = map[string]interface{}{
		"found_in_file": generateHcl,
	}
	var commonMetadata = map[string]interface{}{
		"found_in_file": commonHcl,
	}

	jsonOut := filepath.Join(tmpDir, "terragrunt_rendered.json")

	runTerragrunt(t, fmt.Sprintf("terragrunt render-json --with-metadata --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s  --terragrunt-json-out %s", tmpDir, jsonOut))

	jsonBytes, err := ioutil.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJson = map[string]interface{}{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJson))

	var inputs = renderedJson[config.MetadataInputs]
	var expectedInputs = map[string]interface{}{
		"content": map[string]interface{}{
			"metadata": localsMetadata,
			"value":    "test",
		},
		"qwe": map[string]interface{}{
			"metadata": inputMetadata,
			"value":    "123",
		},
	}
	assert.True(t, reflect.DeepEqual(expectedInputs, inputs))

	var locals = renderedJson[config.MetadataLocals]
	var expectedLocals = map[string]interface{}{
		"abc": map[string]interface{}{
			"metadata": terragruntMetadata,
			"value":    "xyz",
		},
	}
	assert.True(t, reflect.DeepEqual(expectedLocals, locals))

	var generate = renderedJson[config.MetadataGenerateConfigs]
	var expectedGenerate = map[string]interface{}{
		"provider": map[string]interface{}{
			"metadata": generateMetadata,
			"value": map[string]interface{}{
				"comment_prefix":    "# ",
				"contents":          "# test\n",
				"disable_signature": false,
				"disable":           false,
				"if_exists":         "overwrite",
				"path":              "provider.tf",
			},
		},
	}

	// compare fields by serialization in json since map from "value" field is not deterministic
	serializedGenerate, err := json.Marshal(generate)
	assert.NoError(t, err)

	serializedExpectedGenerate, err := json.Marshal(expectedGenerate)
	assert.NoError(t, err)

	assert.Equal(t, string(serializedExpectedGenerate), string(serializedGenerate))

	var remoteState = renderedJson[config.MetadataRemoteState]
	var expectedRemoteState = map[string]interface{}{
		"metadata": commonMetadata,
		"value": map[string]interface{}{
			"backend":                         "s3",
			"disable_dependency_optimization": false,
			"disable_init":                    false,
			"generate":                        nil,
			"config": map[string]interface{}{
				"bucket": "mybucket",
				"key":    "path/to/my/key",
				"region": "us-east-1",
			},
		},
	}

	// compare fields by serialization in json since map from "value" field is not deterministic
	serializedRemoteState, err := json.Marshal(remoteState)
	assert.NoError(t, err)

	serializedExpectedRemoteState, err := json.Marshal(expectedRemoteState)
	assert.NoError(t, err)

	assert.Equal(t, string(serializedExpectedRemoteState), string(serializedRemoteState))
}

func TestRenderJsonMetadataDepenency(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_RENDER_JSON_METADATA)
	cleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "dependency", "app")

	terragruntHcl := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "dependency", "app", "terragrunt.hcl")

	var terragruntMetadata = map[string]interface{}{
		"found_in_file": terragruntHcl,
	}

	jsonOut := filepath.Join(tmpDir, "terragrunt_rendered.json")

	runTerragrunt(t, fmt.Sprintf("terragrunt render-json --with-metadata --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s  --terragrunt-json-out %s", tmpDir, jsonOut))

	jsonBytes, err := ioutil.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJson = map[string]interface{}{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJson))

	var dependency = renderedJson[config.MetadataDependency]

	var expectedDependency = map[string]interface{}{
		"dep": map[string]interface{}{
			"metadata": terragruntMetadata,
			"value": map[string]interface{}{
				"config_path": "../dependency",
				"mock_outputs": map[string]interface{}{
					"test": "value",
				},
				"mock_outputs_allowed_terraform_commands": nil,
				"mock_outputs_merge_strategy_with_state":  nil,
				"mock_outputs_merge_with_state":           nil,
				"name":                                    "dep",
				"outputs":                                 nil,
				"skip":                                    nil,
			},
		},
		"dep2": map[string]interface{}{
			"metadata": terragruntMetadata,
			"value": map[string]interface{}{
				"config_path": "../dependency2",
				"mock_outputs": map[string]interface{}{
					"test2": "value2",
				},
				"mock_outputs_allowed_terraform_commands": nil,
				"mock_outputs_merge_strategy_with_state":  nil,
				"mock_outputs_merge_with_state":           nil,
				"name":                                    "dep2",
				"outputs":                                 nil,
				"skip":                                    nil,
			},
		},
	}

	// compare fields by serialization in json since map from "value" field is not deterministic
	serializedDependency, err := json.Marshal(dependency)
	assert.NoError(t, err)

	serializedExpectedDependency, err := json.Marshal(expectedDependency)
	assert.NoError(t, err)

	assert.Equal(t, string(serializedExpectedDependency), string(serializedDependency))
}

func TestRenderJsonMetadataTerraform(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_RENDER_JSON_METADATA)
	cleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "terraform-remote-state", "app")

	commonHcl := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "terraform-remote-state", "common", "terraform.hcl")
	remoteStateHcl := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "terraform-remote-state", "common", "remote_state.hcl")
	var terragruntMetadata = map[string]interface{}{
		"found_in_file": commonHcl,
	}
	var remoteMetadata = map[string]interface{}{
		"found_in_file": remoteStateHcl,
	}

	jsonOut := filepath.Join(tmpDir, "terragrunt_rendered.json")

	runTerragrunt(t, fmt.Sprintf("terragrunt render-json --with-metadata --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s  --terragrunt-json-out %s", tmpDir, jsonOut))

	jsonBytes, err := ioutil.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJson = map[string]interface{}{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJson))

	var terraform = renderedJson[config.MetadataTerraform]
	var expectedTerraform = map[string]interface{}{
		"metadata": terragruntMetadata,
		"value": map[string]interface{}{
			"after_hook":      map[string]interface{}{},
			"before_hook":     map[string]interface{}{},
			"error_hook":      map[string]interface{}{},
			"extra_arguments": map[string]interface{}{},
			"include_in_copy": nil,
			"source":          "../terraform",
		},
	}

	// compare fields by serialization in json since map from "value" field is not deterministic
	serializedTerraform, err := json.Marshal(terraform)
	assert.NoError(t, err)

	serializedExpectedTerraform, err := json.Marshal(expectedTerraform)
	assert.NoError(t, err)

	assert.Equal(t, string(serializedExpectedTerraform), string(serializedTerraform))

	var remoteState = renderedJson[config.MetadataRemoteState]
	var expectedRemoteState = map[string]interface{}{
		"metadata": remoteMetadata,
		"value": map[string]interface{}{
			"backend": "s3",
			"config": map[string]interface{}{
				"bucket": "mybucket",
				"key":    "path/to/my/key",
				"region": "us-east-1",
			},
			"disable_dependency_optimization": false,
			"disable_init":                    false,
			"generate":                        nil,
		},
	}

	// compare fields by serialization in json since map from "value" field is not deterministic
	serializedRemoteState, err := json.Marshal(remoteState)
	assert.NoError(t, err)

	serializedExpectedRemoteState, err := json.Marshal(expectedRemoteState)
	assert.NoError(t, err)

	assert.Equal(t, string(serializedExpectedRemoteState), string(serializedRemoteState))
}

func TestTerragruntRenderJsonHelp(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND)
	tmpEnvPath := copyEnvironment(t, "fixture-hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND)

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt render-json --help --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &showStdout, &showStderr)
	assert.Nil(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")

	output := showStdout.String()

	assert.Contains(t, output, "Usage: terragrunt render-json [OPTIONS]")
	assert.Contains(t, output, "--with-metadata")
}

func TestStartsWith(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_STARTSWITH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_STARTSWITH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_STARTSWITH)

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

	validateOutput(t, outputs, "startswith1", true)
	validateOutput(t, outputs, "startswith2", false)
	validateOutput(t, outputs, "startswith3", true)
	validateOutput(t, outputs, "startswith4", false)
	validateOutput(t, outputs, "startswith5", true)
	validateOutput(t, outputs, "startswith6", false)
	validateOutput(t, outputs, "startswith7", true)
	validateOutput(t, outputs, "startswith8", false)
	validateOutput(t, outputs, "startswith9", false)
}

func TestTimeCmp(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_TIMECMP)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_TIMECMP)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_TIMECMP)

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

	validateOutput(t, outputs, "timecmp1", float64(0))
	validateOutput(t, outputs, "timecmp2", float64(0))
	validateOutput(t, outputs, "timecmp3", float64(1))
	validateOutput(t, outputs, "timecmp4", float64(-1))
	validateOutput(t, outputs, "timecmp5", float64(-1))
	validateOutput(t, outputs, "timecmp6", float64(1))
}

func TestTimeCmpInvalidTimestamp(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_TIMECMP_INVALID_TIMESTAMP)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_TIMECMP_INVALID_TIMESTAMP)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_TIMECMP_INVALID_TIMESTAMP)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)

	expectedError := `not a valid RFC3339 timestamp: missing required time introducer 'T'`
	require.ErrorContains(t, err, expectedError)
}

func TestEndsWith(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_ENDSWITH)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_ENDSWITH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_ENDSWITH)

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

	validateOutput(t, outputs, "endswith1", true)
	validateOutput(t, outputs, "endswith2", false)
	validateOutput(t, outputs, "endswith3", true)
	validateOutput(t, outputs, "endswith4", false)
	validateOutput(t, outputs, "endswith5", true)
	validateOutput(t, outputs, "endswith6", false)
	validateOutput(t, outputs, "endswith7", true)
	validateOutput(t, outputs, "endswith8", false)
	validateOutput(t, outputs, "endswith9", false)
}

func TestMockOutputsMergeWithState(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_REGRESSIONS)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_REGRESSIONS)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_REGRESSIONS, "mocks-merge-with-state")

	modulePath := util.JoinPath(rootPath, "module")
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-log-level debug --terragrunt-non-interactive -auto-approve --terragrunt-working-dir %s", modulePath), &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "module-executed")
	require.NoError(t, err)

	deepMapPath := util.JoinPath(rootPath, "deep-map")
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-log-level debug --terragrunt-non-interactive -auto-approve --terragrunt-working-dir %s", deepMapPath), &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "deep-map-executed")
	require.NoError(t, err)

	shallowPath := util.JoinPath(rootPath, "shallow")
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-log-level debug --terragrunt-non-interactive -auto-approve --terragrunt-working-dir %s", shallowPath), &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "shallow-map-executed")
	require.NoError(t, err)
}

func TestRenderJsonMetadataDepenencyModulePrefix(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_RENDER_JSON_METADATA)
	cleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_METADATA, "dependency", "app")

	runTerragrunt(t, fmt.Sprintf("terragrunt run-all render-json --terragrunt-include-module-prefix --with-metadata --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", tmpDir))
}

func TestTerragruntValidateModulePrefix(t *testing.T) {
	t.Parallel()

	fixturePath := TEST_FIXTURE_INCLUDE_PARENT
	cleanupTerraformFolder(t, fixturePath)
	tmpEnvPath := copyEnvironment(t, fixturePath)
	rootPath := util.JoinPath(tmpEnvPath, fixturePath)

	runTerragrunt(t, fmt.Sprintf("terragrunt run-all validate --terragrunt-include-module-prefix --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))
}

func TestInitFailureModulePrefix(t *testing.T) {
	t.Parallel()

	initTestCase := TEST_FIXTURE_INIT_ERROR
	cleanupTerraformFolder(t, initTestCase)
	cleanupTerragruntFolder(t, initTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.Error(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt init -no-color --terragrunt-include-module-prefix --terragrunt-non-interactive --terragrunt-working-dir %s", initTestCase), &stdout, &stderr),
	)
	logBufferContentsLineByLine(t, stderr, "init")
	assert.Contains(t, stderr.String(), "[fixture-init-error] Error")
}

func TestDependencyOutputModulePrefix(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_OUTPUT)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_OUTPUT)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_OUTPUT, "integration")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply-all --terragrunt-include-module-prefix --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// verify expected output 42
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	app3Path := util.JoinPath(rootPath, "app3")
	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-include-module-prefix --terragrunt-non-interactive --terragrunt-working-dir %s", app3Path), &stdout, &stderr),
	)
	// validate that output is valid json
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	assert.Equal(t, int(outputs["z"].Value.(float64)), 42)
}

func TestErrorExplaining(t *testing.T) {
	t.Parallel()

	initTestCase := TEST_FIXTURE_INIT_ERROR
	cleanupTerraformFolder(t, initTestCase)
	cleanupTerragruntFolder(t, initTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt init -no-color --terragrunt-include-module-prefix --terragrunt-non-interactive --terragrunt-working-dir %s", initTestCase), &stdout, &stderr)
	explanation := shell.ExplainError(err)
	assert.Contains(t, explanation, "Check your credentials and permissions")
}

func TestModulePathInPlanErrorMessage(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_MODULE_PATH_ERROR)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_MODULE_PATH_ERROR, "app")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan -no-color --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("[%s] exit status 1", util.JoinPath(tmpEnvPath, TEST_FIXTURE_MODULE_PATH_ERROR, "d1")))
}

func TestModulePathInRunAllPlanErrorMessage(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_MODULE_PATH_ERROR)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_MODULE_PATH_ERROR)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt run-all plan -no-color --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("[%s] exit status 1", util.JoinPath(tmpEnvPath, TEST_FIXTURE_MODULE_PATH_ERROR, "d1")))
}

func TestHclFmtDiff(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_HCLFMT_DIFF)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_HCLFMT_DIFF)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_HCLFMT_DIFF)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt hclfmt --terragrunt-diff --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

	output := stdout.String()

	expectedDiff, err := ioutil.ReadFile(util.JoinPath(rootPath, "expected.diff"))
	assert.NoError(t, err)

	logBufferContentsLineByLine(t, stdout, "output")
	assert.Contains(t, output, string(expectedDiff))
}

func TestDestroyDependentModule(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_DESTROY_DEPENDENT_MODULE)
	tmpEnvPath, _ := filepath.EvalSymlinks(copyEnvironment(t, TEST_FIXTURE_DESTROY_DEPENDENT_MODULE))
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_DESTROY_DEPENDENT_MODULE)

	output, err := exec.Command("git", "init", rootPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(output))
	}
	// apply each module in order
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", util.JoinPath(rootPath, "a")))
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", util.JoinPath(rootPath, "b")))
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", util.JoinPath(rootPath, "c")))

	config.ClearOutputCache()

	// destroy module which have outputs from other modules
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt destroy -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", util.JoinPath(rootPath, "c")), &stdout, &stderr)
	assert.NoError(t, err)

	assert.True(t, strings.Contains(stderr.String(), util.JoinPath(rootPath, "b", "terragrunt.hcl")))
	assert.True(t, strings.Contains(stderr.String(), util.JoinPath(rootPath, "a", "terragrunt.hcl")))

	assert.True(t, strings.Contains(stderr.String(), "\"value\": \"module-b.txt\""))
	assert.True(t, strings.Contains(stderr.String(), "\"value\": \"module-a.txt\""))
}

func TestDownloadSourceWithRef(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_REF_SOURCE)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_REF_SOURCE)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s", testPath), &stdout, &stderr)
	require.NoError(t, err)
}

func TestSourceMapWithSlashInRef(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_SOURCE_MAP_SLASHES)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_SOURCE_MAP_SLASHES)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-source-map git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=git::git@github.com:gruntwork-io/terragrunt.git?ref=fixture/test --terragrunt-working-dir %s", testPath), &stdout, &stderr)
	require.NoError(t, err)
}

func TestStrContains(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_STRCONTAINS)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_STRCONTAINS)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_STRCONTAINS)

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

	validateOutput(t, outputs, "o1", true)
	validateOutput(t, outputs, "o2", false)
}

func TestInitSkipCache(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_INIT_CACHE)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_INIT_CACHE)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_INIT_CACHE, "app")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

	// verify that init was invoked
	assert.Contains(t, stdout.String(), "Terraform has been successfully initialized!")
	assert.Contains(t, stderr.String(), "Running command: terraform init")

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

	// verify that init wasn't invoked second time since cache directories are ignored
	assert.NotContains(t, stdout.String(), "Terraform has been successfully initialized!")
	assert.NotContains(t, stderr.String(), "Running command: terraform init")

	// verify that after adding new file, init is executed
	tfFile := util.JoinPath(tmpEnvPath, TEST_FIXTURE_INIT_CACHE, "app", "project.tf")
	if err := ioutil.WriteFile(tfFile, []byte(""), 0644); err != nil {
		t.Fatalf("Error writing new Terraform file to %s: %v", tfFile, err)
	}

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

	// verify that init was invoked
	assert.Contains(t, stdout.String(), "Terraform has been successfully initialized!")
	assert.Contains(t, stderr.String(), "Running command: terraform init")
}

func TestRenderJsonWithInputsNotExistingOutput(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_RENDER_JSON_INPUTS)
	cleanupTerraformFolder(t, tmpEnvPath)
	dependencyPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_INPUTS, "dependency")
	appPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_RENDER_JSON_INPUTS, "app")

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", dependencyPath))
	runTerragrunt(t, fmt.Sprintf("terragrunt render-json --with-metadata --terragrunt-non-interactive --terragrunt-working-dir %s", appPath))

	jsonOut := filepath.Join(appPath, "terragrunt_rendered.json")

	jsonBytes, err := ioutil.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJson = map[string]interface{}{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJson))

	var includeMetadata = map[string]interface{}{
		"found_in_file": util.JoinPath(appPath, "terragrunt.hcl"),
	}

	var inputs = renderedJson[config.MetadataInputs]
	var expectedInputs = map[string]interface{}{
		"static_value": map[string]interface{}{
			"metadata": includeMetadata,
			"value":    "static_value",
		},
		"value": map[string]interface{}{
			"metadata": includeMetadata,
			"value":    "output_value",
		},
		"not_existing_value": map[string]interface{}{
			"metadata": includeMetadata,
			"value":    "",
		},
	}
	assert.True(t, reflect.DeepEqual(expectedInputs, inputs))
}

func TestTerragruntFailIfBucketCreationIsRequired(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_PATH)
	cleanupTerraformFolder(t, rootPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, rootPath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-fail-on-state-bucket-creation --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, rootPath), &stdout, &stderr)
	assert.Error(t, err)
}

func TestTerragruntDisableBucketUpdate(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_PATH)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_PATH)
	cleanupTerraformFolder(t, rootPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	err := createS3BucketE(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	assert.NoError(t, err)

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, rootPath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-disable-bucket-update --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, rootPath))

	_, err = bucketPolicy(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	// validate that bucket policy is not updated, because of --terragrunt-disable-bucket-update
	assert.Error(t, err)
}

func validateOutput(t *testing.T, outputs map[string]TerraformOutput, key string, value interface{}) {
	t.Helper()
	output, hasPlatform := outputs[key]
	require.Truef(t, hasPlatform, "Expected output %s to be defined", key)
	require.Equalf(t, output.Value, value, "Expected output %s to be %t", key, value)
}
