package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/go-commons/version"
	terraws "github.com/gruntwork-io/terratest/modules/aws"
	"github.com/gruntwork-io/terratest/modules/git"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/cli/commands"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	terragruntDynamoDb "github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
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
	TEST_FIXTURE_HCLVALIDATE                                                 = "fixtures/hclvalidate"
	TEST_FIXTURE_EXCLUDES_FILE                                               = "fixutre-excludes-file"
	TEST_FIXTURE_INIT_ONCE                                                   = "fixtures/init-once"
	TEST_FIXTURE_PROVIDER_CACHE_MULTIPLE_PLATFORMS                           = "fixtures/provider-cache/multiple-platforms"
	TEST_FIXTURE_PROVIDER_CACHE_DIRECT                                       = "fixtures/provider-cache/direct"
	TEST_FIXTURE_PROVIDER_CACHE_NETWORK_MIRROR                               = "fixtures/provider-cache/network-mirror"
	TEST_FIXTURE_PROVIDER_CACHE_FILESYSTEM_MIRROR                            = "fixtures/provider-cache/filesystem-mirror"
	TEST_FIXTURE_DESTROY_ORDER                                               = "fixtures/destroy-order"
	TEST_FIXTURE_CODEGEN_PATH                                                = "fixtures/codegen"
	TEST_FIXTURE_GCS_PATH                                                    = "fixtures/gcs/"
	TEST_FIXTURE_GCS_BYO_BUCKET_PATH                                         = "fixtures/gcs-byo-bucket/"
	TEST_FIXTURE_STACK                                                       = "fixtures/stack/"
	TEST_FIXTURE_GRAPH_DEPENDENCIES                                          = "fixtures/graph-dependencies"
	TEST_FIXTURE_OUTPUT_ALL                                                  = "fixtures/output-all"
	TEST_FIXTURE_OUTPUT_FROM_REMOTE_STATE                                    = "fixtures/output-from-remote-state"
	TEST_FIXTURE_OUTPUT_FROM_DEPENDENCY                                      = "fixtures/output-from-dependency"
	TEST_FIXTURE_INPUTS_FROM_DEPENDENCY                                      = "fixtures/inputs-from-dependency"
	TEST_FIXTURE_STDOUT                                                      = "fixtures/download/stdout-test"
	TEST_FIXTURE_EXTRA_ARGS_PATH                                             = "fixtures/extra-args/"
	TEST_FIXTURE_ENV_VARS_BLOCK_PATH                                         = "fixtures/env-vars-block/"
	TEST_FIXTURE_SKIP                                                        = "fixtures/skip/"
	TEST_FIXTURE_CONFIG_SINGLE_JSON_PATH                                     = "fixtures/config-files/single-json-config"
	TEST_FIXTURE_CONFIG_WITH_NON_DEFAULT_NAMES                               = "fixtures/config-files/with-non-default-names"
	TEST_FIXTURE_PREVENT_DESTROY_OVERRIDE                                    = "fixtures/prevent-destroy-override/child"
	TEST_FIXTURE_PREVENT_DESTROY_NOT_SET                                     = "fixtures/prevent-destroy-not-set/child"
	TEST_FIXTURE_LOCAL_PREVENT_DESTROY                                       = "fixtures/download/local-with-prevent-destroy"
	TEST_FIXTURE_LOCAL_PREVENT_DESTROY_DEPENDENCIES                          = "fixtures/download/local-with-prevent-destroy-dependencies"
	TEST_FIXTURE_LOCAL_INCLUDE_PREVENT_DESTROY_DEPENDENCIES                  = "fixtures/download/local-include-with-prevent-destroy-dependencies"
	TEST_FIXTURE_NOT_EXISTING_SOURCE                                         = "fixtures/download/invalid-path"
	TEST_FIXTURE_EXTERNAL_DEPENDENCE                                         = "fixtures/external-dependencies"
	TEST_FIXTURE_MISSING_DEPENDENCE                                          = "fixtures/missing-dependencies/main"
	TEST_FIXTURE_GET_OUTPUT                                                  = "fixtures/get-output"
	TEST_FIXTURE_HOOKS_BEFORE_ONLY_PATH                                      = "fixtures/hooks/before-only"
	TEST_FIXTURE_HOOKS_ALL_PATH                                              = "fixtures/hooks/all"
	TEST_FIXTURE_HOOKS_AFTER_ONLY_PATH                                       = "fixtures/hooks/after-only"
	TEST_FIXTURE_HOOKS_BEFORE_AND_AFTER_PATH                                 = "fixtures/hooks/before-and-after"
	TEST_FIXTURE_HOOKS_BEFORE_AFTER_AND_ERROR_MERGE_PATH                     = "fixtures/hooks/before-after-and-error-merge"
	TEST_FIXTURE_HOOKS_SKIP_ON_ERROR_PATH                                    = "fixtures/hooks/skip-on-error"
	TEST_FIXTURE_ERROR_HOOKS_PATH                                            = "fixtures/hooks/error-hooks"
	TEST_FIXTURE_HOOKS_ONE_ARG_ACTION_PATH                                   = "fixtures/hooks/one-arg-action"
	TEST_FIXTURE_HOOKS_EMPTY_STRING_COMMAND_PATH                             = "fixtures/hooks/bad-arg-action/empty-string-command"
	TEST_FIXTURE_HOOKS_EMPTY_COMMAND_LIST_PATH                               = "fixtures/hooks/bad-arg-action/empty-command-list"
	TEST_FIXTURE_HOOKS_INTERPOLATIONS_PATH                                   = "fixtures/hooks/interpolations"
	TEST_FIXTURE_HOOKS_INIT_ONCE_NO_SOURCE_NO_BACKEND                        = "fixtures/hooks/init-once/no-source-no-backend"
	TEST_FIXTURE_HOOKS_INIT_ONCE_NO_SOURCE_WITH_BACKEND                      = "fixtures/hooks/init-once/no-source-with-backend"
	TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND                      = "fixtures/hooks/init-once/with-source-no-backend"
	TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_NO_BACKEND_SUPPRESS_HOOK_STDOUT = "fixtures/hooks/init-once/with-source-no-backend-suppress-hook-stdout"
	TEST_FIXTURE_HOOKS_INIT_ONCE_WITH_SOURCE_WITH_BACKEND                    = "fixtures/hooks/init-once/with-source-with-backend"
	TEST_FIXTURE_FAILED_TERRAFORM                                            = "fixtures/failure"
	TEST_FIXTURE_EXIT_CODE                                                   = "fixtures/exit-code"
	TEST_FIXTURE_AUTO_RETRY_RERUN                                            = "fixtures/auto-retry/re-run"
	TEST_FIXTURE_AUTO_RETRY_EXHAUST                                          = "fixtures/auto-retry/exhaust"
	TEST_FIXTURE_AUTO_RETRY_GET_DEFAULT_ERRORS                               = "fixtures/auto-retry/get-default-errors"
	TEST_FIXTURE_AUTO_RETRY_CUSTOM_ERRORS                                    = "fixtures/auto-retry/custom-errors"
	TEST_FIXTURE_AUTO_RETRY_CUSTOM_ERRORS_NOT_SET                            = "fixtures/auto-retry/custom-errors-not-set"
	TEST_FIXTURE_AUTO_RETRY_APPLY_ALL_RETRIES                                = "fixtures/auto-retry/apply-all"
	TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES                             = "fixtures/auto-retry/configurable-retries"
	TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES_ERROR_1                     = "fixtures/auto-retry/configurable-retries-incorrect-retry-attempts"
	TEST_FIXTURE_AUTO_RETRY_CONFIGURABLE_RETRIES_ERROR_2                     = "fixtures/auto-retry/configurable-retries-incorrect-sleep-interval"
	TEST_FIXTURE_AWS_PROVIDER_PATCH                                          = "fixtures/aws-provider-patch"
	TEST_FIXTURE_INPUTS                                                      = "fixtures/inputs"
	TEST_FIXTURE_LOCALS_ERROR_UNDEFINED_LOCAL                                = "fixtures/locals-errors/undefined-local"
	TEST_FIXTURE_LOCALS_ERROR_UNDEFINED_LOCAL_BUT_INPUT                      = "fixtures/locals-errors/undefined-local-but-input"
	TEST_FIXTURE_LOCALS_CANONICAL                                            = "fixtures/locals/canonical"
	TEST_FIXTURE_LOCALS_IN_INCLUDE                                           = "fixtures/locals/local-in-include"
	TEST_FIXTURE_LOCAL_RUN_ONCE                                              = "fixtures/locals/run-once"
	TEST_FIXTURE_LOCAL_RUN_MULTIPLE                                          = "fixtures/locals/run-multiple"
	TEST_FIXTURE_LOCALS_IN_INCLUDE_CHILD_REL_PATH                            = "qa/my-app"
	TEST_FIXTURE_NO_COLOR                                                    = "fixtures/no-color"
	TEST_FIXTURE_READ_CONFIG                                                 = "fixtures/read-config"
	TEST_FIXTURE_READ_IAM_ROLE                                               = "fixtures/read-config/iam_role_in_file"
	TEST_FIXTURE_IAM_ROLES_MULTIPLE_MODULES                                  = "fixtures/read-config/iam_roles_multiple_modules"
	TEST_FIXTURE_RELATIVE_INCLUDE_CMD                                        = "fixtures/relative-include-cmd"
	TEST_FIXTURE_AWS_GET_CALLER_IDENTITY                                     = "fixtures/get-aws-caller-identity"
	TEST_FIXTURE_GET_REPO_ROOT                                               = "fixtures/get-repo-root"
	TEST_FIXTURE_GET_WORKING_DIR                                             = "fixtures/get-working-dir"
	TEST_FIXTURE_PATH_RELATIVE_FROM_INCLUDE                                  = "fixtures/get-path/fixtures/path_relative_from_include"
	TEST_FIXTURE_GET_PATH_FROM_REPO_ROOT                                     = "fixtures/get-path/fixtures/get-path-from-repo-root"
	TEST_FIXTURE_GET_PATH_TO_REPO_ROOT                                       = "fixtures/get-path/fixtures/get-path-to-repo-root"
	TEST_FIXTURE_GET_PLATFORM                                                = "fixtures/get-platform"
	TEST_FIXTURE_GET_TERRAGRUNT_SOURCE_HCL                                   = "fixtures/get-terragrunt-source-hcl"
	TEST_FIXTURE_GET_TERRAGRUNT_SOURCE_CLI                                   = "fixtures/get-terragrunt-source-cli"
	TEST_FIXTURE_REGRESSIONS                                                 = "fixtures/regressions"
	TEST_FIXTURE_PLANFILE_ORDER                                              = "fixtures/planfile-order-test"
	TEST_FIXTURE_DIRS_PATH                                                   = "fixtures/dirs"
	TEST_FIXTURE_PARALLELISM                                                 = "fixtures/parallelism"
	TEST_FIXTURE_SOPS                                                        = "fixtures/sops"
	TEST_FIXTURE_DESTROY_WARNING                                             = "fixtures/destroy-warning"
	TEST_FIXTURE_INCLUDE_PARENT                                              = "fixtures/include-parent"
	TEST_FIXTURE_AUTO_INIT                                                   = "fixtures/download/init-on-source-change"
	TEST_FIXTURE_DISJOINT                                                    = "fixtures/stack/disjoint"
	TEST_FIXTURE_BROKEN_LOCALS                                               = "fixtures/broken-locals"
	TEST_FIXTURE_BROKEN_DEPENDENCY                                           = "fixtures/broken-dependency"
	TEST_FIXTURE_RENDER_JSON_METADATA                                        = "fixtures/render-json-metadata"
	TEST_FIXTURE_RENDER_JSON_MOCK_OUTPUTS                                    = "fixtures/render-json-mock-outputs"
	TEST_FIXTURE_RENDER_JSON_INPUTS                                          = "fixtures/render-json-inputs"
	TEST_FIXTURE_OUTPUT_MODULE_GROUPS                                        = "fixtures/output-module-groups"
	TEST_FIXTURE_STARTSWITH                                                  = "fixtures/startswith"
	TEST_FIXTURE_TIMECMP                                                     = "fixtures/timecmp"
	TEST_FIXTURE_TIMECMP_INVALID_TIMESTAMP                                   = "fixtures/timecmp-errors/invalid-timestamp"
	TEST_FIXTURE_ENDSWITH                                                    = "fixtures/endswith"
	TEST_FIXTURE_TFLINT_NO_ISSUES_FOUND                                      = "fixtures/tflint/no-issues-found"
	TEST_FIXTURE_TFLINT_ISSUES_FOUND                                         = "fixtures/tflint/issues-found"
	TEST_FIXTURE_TFLINT_NO_CONFIG_FILE                                       = "fixtures/tflint/no-config-file"
	TEST_FIXTURE_TFLINT_MODULE_FOUND                                         = "fixtures/tflint/module-found"
	TEST_FIXTURE_TFLINT_NO_TF_SOURCE_PATH                                    = "fixtures/tflint/no-tf-source"
	TEST_FIXTURE_TFLINT_EXTERNAL_TFLINT                                      = "fixtures/tflint/external-tflint"
	TEST_FIXTURE_TFLINT_TFVAR_PASSING                                        = "fixtures/tflint/tfvar-passing"
	TEST_FIXTURE_TFLINT_ARGS                                                 = "fixtures/tflint/tflint-args"
	TEST_FIXTURE_TFLINT_CUSTOM_CONFIG                                        = "fixtures/tflint/custom-tflint-config"
	TEST_FIXTURE_PARALLEL_RUN                                                = "fixtures/parallel-run"
	TEST_FIXTURE_INIT_ERROR                                                  = "fixtures/init-error"
	TEST_FIXTURE_MODULE_PATH_ERROR                                           = "fixtures/module-path-in-error"
	TEST_FIXTURE_HCLFMT_DIFF                                                 = "fixtures/hclfmt-diff"
	TEST_FIXTURE_DESTROY_DEPENDENT_MODULE                                    = "fixtures/destroy-dependent-module"
	TEST_FIXTURE_REF_SOURCE                                                  = "fixtures/download/remote-ref"
	TEST_FIXTURE_SOURCE_MAP_SLASHES                                          = "fixtures/source-map/slashes-in-ref"
	TEST_FIXTURE_STRCONTAINS                                                 = "fixtures/strcontains"
	TEST_FIXTURE_INIT_CACHE                                                  = "fixtures/init-cache"
	TEST_FIXTURE_NULL_VALUE                                                  = "fixtures/null-values"
	TEST_FIXTURE_GCS_IMPERSONATE_PATH                                        = "fixtures/gcs-impersonate/"
	TEST_FIXTURE_S3_ERRORS                                                   = "fixtures/s3-errors/"
	TEST_FIXTURE_GCS_NO_BUCKET                                               = "fixtures/gcs-no-bucket/"
	TEST_FIXTURE_GCS_NO_PREFIX                                               = "fixtures/gcs-no-prefix/"
	TEST_FIXTURE_DISABLED_PATH                                               = "fixtures/disabled-path/"
	TEST_FIXTURE_NO_SUBMODULES                                               = "fixtures/no-submodules/"
	TEST_FIXTURE_DISABLED_MODULE                                             = "fixtures/disabled/"
	TEST_FIXTURE_EMPTY_STATE                                                 = "fixtures/empty-state/"
	TEST_FIXTURE_EXTERNAL_DEPENDENCY                                         = "fixtures/external-dependency/"
	TEST_FIXTURE_TF_TEST                                                     = "fixtures/tftest/"
	TEST_COMMANDS_THAT_NEED_INPUT                                            = "fixtures/commands-that-need-input"
	TEST_FIXTURE_PARALLEL_STATE_INIT                                         = "fixtures/parallel-state-init"
	TEST_FIXTURE_GCS_PARALLEL_STATE_INIT                                     = "fixtures/gcs-parallel-state-init"
	TEST_FIXTURE_ASSUME_ROLE                                                 = "fixtures/assume-role/external-id"
	TEST_FIXTURE_ASSUME_ROLE_DURATION                                        = "fixtures/assume-role/duration"
	TEST_FIXTURE_ASSUME_ROLE_WEB_IDENTITY_ENV                                = "fixtures/assume-role-web-identity/env-var"
	TEST_FIXTURE_ASSUME_ROLE_WEB_IDENTITY_FILE                               = "fixtures/assume-role-web-identity/file-path"
	TEST_FIXTURE_GRAPH                                                       = "fixtures/graph"
	TEST_FIXTURE_SKIP_DEPENDENCIES                                           = "fixtures/skip-dependencies"
	TEST_FIXTURE_INFO_ERROR                                                  = "fixtures/terragrunt-info-error"
	TEST_FIXTURE_DEPENDENCY_OUTPUT                                           = "fixtures/dependency-output"
	TEST_FIXTURE_OUT_DIR                                                     = "fixtures/out-dir"
	TEST_FIXTURE_SOPS_ERRORS                                                 = "fixtures/sops-errors"
	TEST_FIXTURE_AUTH_PROVIDER_CMD                                           = "fixtures/auth-provider-cmd"
	TERRAFORM_BINARY                                                         = "terraform"
	TOFU_BINARY                                                              = "tofu"
	TERRAFORM_FOLDER                                                         = ".terraform"
	TERRAFORM_STATE                                                          = "terraform.tfstate"
	TERRAFORM_STATE_BACKUP                                                   = "terraform.tfstate.backup"
	TERRAGRUNT_CACHE                                                         = ".terragrunt-cache"

	qaMyAppRelPath  = "qa/my-app"
	fixtureDownload = "fixtures/download"
)

const (
	terragruntDebugFile = "terragrunt-debug.tfvars.json"

	fixtureMultiIncludeDependency = "fixtures/multiinclude-dependency"
	fixtureRenderJSON             = "fixtures/render-json"
	fixtureRenderJSONRegression   = "fixtures/render-json-regression"
)

var (
	fixtureRenderJSONMainModulePath = filepath.Join(fixtureRenderJSON, "main")
	fixtureRenderJSONDepModulePath  = filepath.Join(fixtureRenderJSON, "dep")
)

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
	args := strings.Split(command, " ")
	t.Log(args)

	app := cli.NewApp(writer, errwriter)
	return app.Run(args)
}

func runTerragruntVersionCommand(t *testing.T, ver string, command string, writer io.Writer, errwriter io.Writer) error {
	version.Version = ver
	return runTerragruntCommand(t, command, writer, errwriter)
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

func logBufferContentsLineByLine(t *testing.T, out bytes.Buffer, label string) {
	t.Helper()
	t.Logf("[%s] Full contents of %s:", t.Name(), label)
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		t.Logf("[%s] %s", t.Name(), line)
	}
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

func copyEnvironment(t *testing.T, environmentPath string, includeInCopy ...string) string {
	tmpDir, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	t.Logf("Copying %s to %s", environmentPath, tmpDir)

	require.NoError(t, util.CopyFolderContents(environmentPath, util.JoinPath(tmpDir, environmentPath), ".terragrunt-test", includeInCopy))

	return tmpDir
}

func createTmpTerragruntConfigWithParentAndChild(t *testing.T, parentPath string, childRelPath string, s3BucketName string, parentConfigFileName string, childConfigFileName string) string {
	tmpDir, err := os.MkdirTemp("", "terragrunt-parent-child-test")
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
	tmpFolder, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp folder due to error: %v", err)
	}

	tmpTerragruntConfigFile := util.JoinPath(tmpFolder, configFileName)
	originalTerragruntConfigPath := util.JoinPath(templatesPath, configFileName)
	copyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "not-used")

	return tmpTerragruntConfigFile
}

func createTmpTerragruntConfigContent(t *testing.T, contents string, configFileName string) string {
	tmpFolder, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp folder due to error: %v", err)
	}

	tmpTerragruntConfigFile := util.JoinPath(tmpFolder, configFileName)

	if err := os.WriteFile(tmpTerragruntConfigFile, []byte(contents), 0444); err != nil {
		t.Fatalf("Error writing temp Terragrunt config to %s: %v", tmpTerragruntConfigFile, err)
	}

	return tmpTerragruntConfigFile
}

func createTmpTerragruntGCSConfig(t *testing.T, templatesPath string, project string, location string, gcsBucketName string, configFileName string) string {
	tmpFolder, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp folder due to error: %v", err)
	}

	tmpTerragruntConfigFile := util.JoinPath(tmpFolder, configFileName)
	originalTerragruntConfigPath := util.JoinPath(templatesPath, configFileName)
	copyTerragruntGCSConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, project, location, gcsBucketName)

	return tmpTerragruntConfigFile
}

func copyTerragruntConfigAndFillPlaceholders(t *testing.T, configSrcPath string, configDestPath string, s3BucketName string, lockTableName string, region string) {
	copyAndFillMapPlaceholders(t, configSrcPath, configDestPath, map[string]string{
		"__FILL_IN_BUCKET_NAME__":      s3BucketName,
		"__FILL_IN_LOCK_TABLE_NAME__":  lockTableName,
		"__FILL_IN_REGION__":           region,
		"__FILL_IN_LOGS_BUCKET_NAME__": s3BucketName + "-tf-state-logs",
	})
}

func copyAndFillMapPlaceholders(t *testing.T, srcPath string, destPath string, placeholders map[string]string) {
	contents, err := util.ReadFileAsString(srcPath)
	if err != nil {
		t.Fatalf("Error reading file at %s: %v", srcPath, err)
	}

	// iterate over placeholders and replace placeholders
	for k, v := range placeholders {
		contents = strings.ReplaceAll(contents, k, v)
	}
	if err := os.WriteFile(destPath, []byte(contents), 0444); err != nil {
		t.Fatalf("Error writing temp file to %s: %v", destPath, err)
	}
}

func copyTerragruntGCSConfigAndFillPlaceholders(t *testing.T, configSrcPath string, configDestPath string, project string, location string, gcsBucketName string) {
	email := os.Getenv("GOOGLE_IDENTITY_EMAIL")

	copyAndFillMapPlaceholders(t, configSrcPath, configDestPath, map[string]string{
		"__FILL_IN_PROJECT__":     project,
		"__FILL_IN_LOCATION__":    location,
		"__FILL_IN_BUCKET_NAME__": gcsBucketName,
		"__FILL_IN_GCP_EMAIL__":   email,
	})
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
