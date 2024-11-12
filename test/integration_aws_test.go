//go:build aws

package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/awshelper"
	"github.com/gruntwork-io/terragrunt/config"
	terragruntDynamoDb "github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/util"
	terraws "github.com/gruntwork-io/terratest/modules/aws"
	"github.com/gruntwork-io/terratest/modules/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureAwsProviderPatch          = "fixtures/aws-provider-patch"
	testFixtureAwsGetCallerIdentity      = "fixtures/get-aws-caller-identity"
	testFixtureS3Errors                  = "fixtures/s3-errors/"
	testFixtureAssumeRole                = "fixtures/assume-role/external-id"
	testFixtureAssumeRoleDuration        = "fixtures/assume-role/duration"
	testFixtureAssumeRoleWebIdentityEnv  = "fixtures/assume-role-web-identity/env-var"
	testFixtureAssumeRoleWebIdentityFile = "fixtures/assume-role-web-identity/file-path"
	testFixtureReadIamRole               = "fixtures/read-config/iam_role_in_file"
	testFixtureOutputFromRemoteState     = "fixtures/output-from-remote-state"
	testFixtureOutputFromDependency      = "fixtures/output-from-dependency"

	qaMyAppRelPath = "qa/my-app"
)

func TestAwsInitHookNoSourceWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())

	cleanupTerraformFolder(t, testFixtureHooksInitOnceNoSourceWithBackend)
	tmpEnvPath := copyEnvironment(t, "fixtures/hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInitOnceNoSourceWithBackend)

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", terraformRemoteStateS3Region)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
	output := stdout.String()
	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_ONLY_ONCE"), "Hooks on init command executed more than once")
	// With no source, `init-from-module` should not execute
	assert.NotContains(t, output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE", "Hooks on init-from-module command executed when no source was specified")
}

func TestAwsInitHookWithSourceWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())

	cleanupTerraformFolder(t, testFixtureHooksInitOnceWithSourceWithBackend)
	tmpEnvPath := copyEnvironment(t, "fixtures/hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInitOnceWithSourceWithBackend)

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", terraformRemoteStateS3Region)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
	output := stdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	// `init` hook should execute only once
	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_ONLY_ONCE"), "Hooks on init command executed more than once")
	// `init-from-module` hook should execute only once
	assert.Equal(t, 1, strings.Count(output, "AFTER_INIT_FROM_MODULE_ONLY_ONCE"), "Hooks on init-from-module command executed more than once")
}

func TestAwsBeforeAfterAndErrorMergeHook(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(testFixtureHooksBeforeAfterAndErrorMergePath, qaMyAppRelPath)
	cleanupTerraformFolder(t, childPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	t.Logf("bucketName: %s", s3BucketName)
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, testFixtureHooksBeforeAfterAndErrorMergePath, qaMyAppRelPath, s3BucketName, config.DefaultTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath), &stdout, &stderr)
	require.ErrorContains(t, err, "executable file not found in $PATH")

	_, beforeException := os.ReadFile(childPath + "/before.out")
	_, beforeChildException := os.ReadFile(childPath + "/before-child.out")
	_, beforeOverriddenParentException := os.ReadFile(childPath + "/before-parent.out")
	_, afterException := os.ReadFile(childPath + "/after.out")
	_, afterParentException := os.ReadFile(childPath + "/after-parent.out")
	_, errorHookParentException := os.ReadFile(childPath + "/error-hook-parent.out")
	_, errorHookChildException := os.ReadFile(childPath + "/error-hook-child.out")
	_, errorHookOverridenParentException := os.ReadFile(childPath + "/error-hook-merge-parent.out")

	require.NoError(t, beforeException)
	require.NoError(t, beforeChildException)
	require.NoError(t, afterException)
	require.NoError(t, afterParentException)
	require.NoError(t, errorHookParentException)
	require.NoError(t, errorHookChildException)

	// PathError because no file found
	require.Error(t, beforeOverriddenParentException)
	require.Error(t, errorHookOverridenParentException)
}

func TestAwsWorksWithLocalTerraformVersion(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixturePath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueID())

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, terraformRemoteStateS3Region)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, testFixturePath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, testFixturePath))

	var expectedS3Tags = map[string]string{
		"owner": "terragrunt integration test",
		"name":  "Terraform state storage"}
	validateS3BucketExistsAndIsTagged(t, terraformRemoteStateS3Region, s3BucketName, expectedS3Tags)

	var expectedDynamoDBTableTags = map[string]string{
		"owner": "terragrunt integration test",
		"name":  "Terraform lock table"}
	validateDynamoDBTableExistsAndIsTagged(t, terraformRemoteStateS3Region, lockTableName, expectedDynamoDBTableTags)
}

// Regression test to ensure that `accesslogging_bucket_name` and `accesslogging_target_prefix` are taken into account
// & the TargetLogs bucket is set to a new S3 bucket, different from the origin S3 bucket
// & the logs objects are prefixed with the `accesslogging_target_prefix` value
func TestAwsSetsAccessLoggingForTfSTateS3BuckeToADifferentBucketWithGivenTargetPrefix(t *testing.T) {
	t.Parallel()

	examplePath := filepath.Join(testFixtureRegressions, "accesslogging-bucket/with-target-prefix-input")
	cleanupTerraformFolder(t, examplePath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	s3BucketLogsName := s3BucketName + "-tf-state-logs"
	s3BucketLogsTargetPrefix := "logs/"
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueID())

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, terraformRemoteStateS3Region)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(
		t,
		examplePath,
		s3BucketName,
		lockTableName,
		"remote_terragrunt.hcl",
	)

	runTerragrunt(t, fmt.Sprintf("terragrunt validate --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, examplePath))

	targetLoggingBucket := terraws.GetS3BucketLoggingTarget(t, terraformRemoteStateS3Region, s3BucketName)
	targetLoggingBucketPrefix := terraws.GetS3BucketLoggingTargetPrefix(t, terraformRemoteStateS3Region, s3BucketName)

	assert.Equal(t, s3BucketLogsName, targetLoggingBucket)
	assert.Equal(t, s3BucketLogsTargetPrefix, targetLoggingBucketPrefix)

	encryptionConfig, err := bucketEncryption(t, terraformRemoteStateS3Region, targetLoggingBucket)
	require.NoError(t, err)
	assert.NotNil(t, encryptionConfig)
	assert.NotNil(t, encryptionConfig.ServerSideEncryptionConfiguration)
	for _, rule := range encryptionConfig.ServerSideEncryptionConfiguration.Rules {
		if rule.ApplyServerSideEncryptionByDefault != nil {
			if rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm != nil {
				assert.Equal(t, s3.ServerSideEncryptionAes256, *rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm)
			}
		}
	}

	policy, err := bucketPolicy(t, terraformRemoteStateS3Region, targetLoggingBucket)
	require.NoError(t, err)
	assert.NotNil(t, policy.Policy)

	policyInBucket, err := awshelper.UnmarshalPolicy(*policy.Policy)
	require.NoError(t, err)
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
func TestAwsSetsAccessLoggingForTfSTateS3BuckeToADifferentBucketWithDefaultTargetPrefix(t *testing.T) {
	t.Parallel()

	examplePath := filepath.Join(testFixtureRegressions, "accesslogging-bucket/no-target-prefix-input")
	cleanupTerraformFolder(t, examplePath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	s3BucketLogsName := s3BucketName + "-tf-state-logs"
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueID())

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, terraformRemoteStateS3Region)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(
		t,
		examplePath,
		s3BucketName,
		lockTableName,
		"remote_terragrunt.hcl",
	)

	runTerragrunt(t, fmt.Sprintf("terragrunt validate --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, examplePath))

	targetLoggingBucket := terraws.GetS3BucketLoggingTarget(t, terraformRemoteStateS3Region, s3BucketName)
	targetLoggingBucketPrefix := terraws.GetS3BucketLoggingTargetPrefix(t, terraformRemoteStateS3Region, s3BucketName)

	encryptionConfig, err := bucketEncryption(t, terraformRemoteStateS3Region, targetLoggingBucket)
	require.NoError(t, err)
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

func TestAwsRunAllCommand(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := copyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	runTerragrunt(t, "terragrunt run-all init --terragrunt-non-interactive --terragrunt-working-dir "+environmentPath)
}

func TestAwsOutputAllCommand(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := copyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+environmentPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	runTerragruntRedirectOutput(t, "terragrunt output-all --terragrunt-non-interactive --terragrunt-working-dir "+environmentPath, &stdout, &stderr)
	output := stdout.String()

	assert.True(t, strings.Contains(output, "app1 output"))
	assert.True(t, strings.Contains(output, "app2 output"))
	assert.True(t, strings.Contains(output, "app3 output"))

	assert.True(t, (strings.Index(output, "app3 output") < strings.Index(output, "app1 output")) && (strings.Index(output, "app1 output") < strings.Index(output, "app2 output")))
}

func TestAwsOutputFromDependency(t *testing.T) {
	// t.Parallel() cannot be used together with t.Setenv()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := copyEnvironment(t, testFixtureOutputFromDependency)

	rootTerragruntPath := util.JoinPath(tmpEnvPath, testFixtureOutputFromDependency)
	depTerragruntConfigPath := util.JoinPath(rootTerragruntPath, "dependency", config.DefaultTerragruntConfigPath)

	copyTerragruntConfigAndFillPlaceholders(t, depTerragruntConfigPath, depTerragruntConfigPath, s3BucketName, "not-used", terraformRemoteStateS3Region)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	t.Setenv("AWS_CSM_ENABLED", "true")

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level debug", rootTerragruntPath), &stdout, &stderr)
	require.NoError(t, err)

	output := stderr.String()
	assert.NotContains(t, output, "invalid character")
}

func TestAwsValidateAllCommand(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := copyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	runTerragrunt(t, "terragrunt validate-all --terragrunt-non-interactive --terragrunt-working-dir "+environmentPath)
}

func TestAwsOutputAllCommandSpecificVariableIgnoreDependencyErrors(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := copyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+environmentPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	// Call runTerragruntCommand directly because this command contains failures (which causes runTerragruntRedirectOutput to abort) but we don't care.
	runTerragruntCommand(t, "terragrunt output-all app2_text --terragrunt-ignore-dependency-errors --terragrunt-non-interactive --terragrunt-working-dir "+environmentPath, &stdout, &stderr)
	output := stdout.String()

	logBufferContentsLineByLine(t, stdout, "output-all stdout")
	logBufferContentsLineByLine(t, stderr, "output-all stderr")

	// Without --terragrunt-ignore-dependency-errors, app2 never runs because its dependencies have "errors" since they don't have the output "app2_text".
	assert.True(t, strings.Contains(output, "app2 output"))
}

func TestAwsStackCommands(t *testing.T) { //nolint paralleltest
	// It seems that disabling parallel test execution helps avoid the CircleCi error: “NoSuchBucket Policy: The bucket policy does not exist.”
	// t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueID())

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, terraformRemoteStateS3Region)

	cleanupTerraformFolder(t, testFixtureStack)
	cleanupTerragruntFolder(t, testFixtureStack)

	tmpEnvPath := copyEnvironment(t, testFixtureStack)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureStack, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	mgmtEnvironmentPath := util.JoinPath(tmpEnvPath, testFixtureStack, "mgmt")
	stageEnvironmentPath := util.JoinPath(tmpEnvPath, testFixtureStack, "stage")

	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+mgmtEnvironmentPath)
	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+stageEnvironmentPath)

	runTerragrunt(t, "terragrunt output-all --terragrunt-non-interactive --terragrunt-working-dir "+mgmtEnvironmentPath)
	runTerragrunt(t, "terragrunt output-all --terragrunt-non-interactive --terragrunt-working-dir "+stageEnvironmentPath)

	runTerragrunt(t, "terragrunt destroy-all --terragrunt-non-interactive --terragrunt-working-dir "+stageEnvironmentPath)
	runTerragrunt(t, "terragrunt destroy-all --terragrunt-non-interactive --terragrunt-working-dir "+mgmtEnvironmentPath)
}

func TestAwsRemoteWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	lockTableName := "terragrunt-lock-table-" + strings.ToLower(uniqueID())

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, terraformRemoteStateS3Region)

	tmpEnvPath := copyEnvironment(t, testFixtureRemoteWithBackend)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRemoteWithBackend)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
}

func TestAwsLocalWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	lockTableName := "terragrunt-lock-table-" + strings.ToLower(uniqueID())

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, terraformRemoteStateS3Region)

	tmpEnvPath := copyEnvironment(t, "fixtures/download")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLocalWithBackend)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
}

func TestAwsGetCallerIdentityFunctions(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureAwsGetCallerIdentity)
	tmpEnvPath := copyEnvironment(t, testFixtureAwsGetCallerIdentity)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAwsGetCallerIdentity)

	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
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
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, outputs["account"].Value, *identity.Account)
	assert.Equal(t, outputs["arn"].Value, *identity.Arn)
	assert.Equal(t, outputs["user_id"].Value, *identity.UserId)
}

// We test the path with remote_state blocks by:
// - Applying all modules initially
// - Deleting the local state of the nested deep dependency
// - Running apply on the root module
// If output optimization is working, we should still get the same correct output even though the state of the upmost
// module has been destroyed.
func TestAwsDependencyOutputOptimization(t *testing.T) {
	t.Parallel()

	expectOutputLogs := []string{
		`prefix=../dep .+Running command: ` + wrappedBinary() + ` init -get=false`,
	}
	dependencyOutputOptimizationTest(t, "nested-optimization", true, expectOutputLogs)
}

func TestAwsDependencyOutputOptimizationSkipInit(t *testing.T) {
	t.Parallel()

	expectOutputLogs := []string{
		`prefix=../dep .+Detected module ../dep/terragrunt.hcl is already init-ed. Retrieving outputs directly from working directory.`,
	}
	dependencyOutputOptimizationTest(t, "nested-optimization", false, expectOutputLogs)
}

func TestAwsDependencyOutputOptimizationNoGenerate(t *testing.T) {
	t.Parallel()

	expectOutputLogs := []string{
		`prefix=../dep .+Running command: ` + wrappedBinary() + ` init -get=false`,
	}
	dependencyOutputOptimizationTest(t, "nested-optimization-nogen", true, expectOutputLogs)
}

func TestAwsDependencyOutputOptimizationDisableTest(t *testing.T) {
	t.Parallel()

	expectedOutput := `They said, "No, The answer is 42"`
	generatedUniqueID := uniqueID()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "nested-optimization-disable")
	rootTerragruntConfigPath := filepath.Join(rootPath, config.DefaultTerragruntConfigPath)
	livePath := filepath.Join(rootPath, "live")
	deepDepPath := filepath.Join(rootPath, "deepdep")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(generatedUniqueID)
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(generatedUniqueID)
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, terraformRemoteStateS3Region)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, terraformRemoteStateS3Region)

	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// verify expected output
	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+livePath)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))
	assert.Equal(t, expectedOutput, outputs["output"].Value)

	// Now delete the deepdep state and verify it no longer works, because it tries to fetch the deepdep dependency
	config.ClearOutputCache()
	require.NoError(t, os.Remove(filepath.Join(deepDepPath, "terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(deepDepPath, ".terraform")))
	_, _, err = runTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+livePath)
	require.Error(t, err)
}

func TestAwsProviderPatch(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, testFixtureAwsProviderPatch)
	modulePath := util.JoinPath(rootPath, testFixtureAwsProviderPatch)
	mainTFFile := filepath.Join(modulePath, "main.tf")

	// fill in branch so we can test against updates to the test case file
	mainContents, err := util.ReadFileAsString(mainTFFile)
	require.NoError(t, err)
	branchName := git.GetCurrentBranchName(t)
	// https://www.terraform.io/docs/language/modules/sources.html#modules-in-package-sub-directories
	// https://github.com/gruntwork-io/terragrunt/issues/1778
	branchName = url.QueryEscape(branchName)
	mainContents = strings.ReplaceAll(mainContents, "__BRANCH_NAME__", branchName)
	require.NoError(t, os.WriteFile(mainTFFile, []byte(mainContents), 0444))

	_, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt aws-provider-patch --terragrunt-override-attr region=\"eu-west-1\" --terragrunt-override-attr allowed_account_ids=[\"00000000000\"] --terragrunt-working-dir %s --terragrunt-log-level debug", modulePath))
	require.NoError(t, err)

	assert.Regexp(t, "Patching AWS provider in .+test/fixtures/aws-provider-patch/example-module/main.tf", stderr)

	// Make sure the resulting terraform code is still valid
	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt validate --terragrunt-working-dir "+modulePath, os.Stdout, os.Stderr),
	)
}

func TestAwsPrintAwsErrors(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureS3Errors)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureS3Errors)
	cleanupTerraformFolder(t, rootPath)

	s3BucketName := "test-tg-2023-02"
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueID())

	tmpTerragruntConfigFile := util.JoinPath(rootPath, "terragrunt.hcl")
	originalTerragruntConfigPath := util.JoinPath(rootPath, "terragrunt.hcl")
	copyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "us-east-2")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigFile, rootPath), &stdout, &stderr)
	require.Error(t, err)
	message := err.Error()
	assert.True(t, strings.Contains(message, "AllAccessDisabled: All access to this object has been disabled") || strings.Contains(message, "BucketRegionError: incorrect region"))
	assert.Contains(t, message, s3BucketName)
}

func TestAwsErrorWhenStateBucketIsInDifferentRegion(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureS3Errors)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureS3Errors)
	cleanupTerraformFolder(t, rootPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueID())

	originalTerragruntConfigPath := util.JoinPath(testFixtureS3Errors, "terragrunt.hcl")
	tmpTerragruntConfigFile := util.JoinPath(rootPath, "terragrunt.hcl")
	copyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "us-east-1")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigFile, rootPath), &stdout, &stderr)
	require.NoError(t, err)

	copyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "us-west-2")

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigFile, rootPath), &stdout, &stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BucketRegionError: incorrect region")
}

func TestAwsDisableBucketUpdate(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)
	cleanupTerraformFolder(t, rootPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueID())

	createS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	createDynamoDBTable(t, terraformRemoteStateS3Region, lockTableName)

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, terraformRemoteStateS3Region)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, rootPath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-disable-bucket-update --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, rootPath))

	_, err := bucketPolicy(t, terraformRemoteStateS3Region, s3BucketName)
	// validate that bucket policy is not updated, because of --terragrunt-disable-bucket-update
	require.Error(t, err)
}

func TestAwsUpdatePolicy(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)
	cleanupTerraformFolder(t, rootPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueID())

	createS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, terraformRemoteStateS3Region)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, rootPath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	// check that there is no policy on created bucket
	_, err := bucketPolicy(t, terraformRemoteStateS3Region, s3BucketName)
	require.Error(t, err)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, rootPath))

	// check that policy is created
	_, err = bucketPolicy(t, terraformRemoteStateS3Region, s3BucketName)
	require.NoError(t, err)
}

func TestAwsAssumeRoleDuration(t *testing.T) {
	t.Parallel()
	if isTerraform() {
		t.Skip("New assume role duration config not supported by Terraform 1.5.x")
		return
	}

	tmpEnvPath := copyEnvironment(t, testFixtureAssumeRoleDuration)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAssumeRoleDuration)

	originalTerragruntConfigPath := util.JoinPath(testFixtureAssumeRoleDuration, "terragrunt.hcl")
	tmpTerragruntConfigFile := util.JoinPath(testPath, "terragrunt.hcl")
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	assumeRole := os.Getenv("AWS_TEST_S3_ASSUME_ROLE")

	copyAndFillMapPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, map[string]string{
		"__FILL_IN_BUCKET_NAME__":      s3BucketName,
		"__FILL_IN_REGION__":           terraformRemoteStateS3Region,
		"__FILL_IN_LOGS_BUCKET_NAME__": s3BucketName + "-tf-state-logs",
		"__FILL_IN_ASSUME_ROLE__":      assumeRole,
	})

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply  -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	output := fmt.Sprintf("%s %s", stderr.String(), stdout.String())
	assert.Contains(t, output, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")
	// run one more time to check that no init is performed
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = runTerragruntCommand(t, "terragrunt apply  -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	output = fmt.Sprintf("%s %s", stderr.String(), stdout.String())
	assert.NotContains(t, output, "Initializing the backend...")
	assert.NotContains(t, output, "has been successfully initialized!")
	assert.Contains(t, output, "no changes are needed.")
}

func TestAwsAssumeRoleWebIdentityEnv(t *testing.T) {
	t.Parallel()

	assumeRole := os.Getenv("AWS_TEST_S3_ASSUME_ROLE")
	tokenEnvVar := os.Getenv("AWS_TEST_S3_IDENTITY_TOKEN_VAR")
	if tokenEnvVar == "" {
		t.Skip("Missing required env var AWS_TEST_S3_IDENTITY_TOKEN_VAR")
		return
	}

	tmpEnvPath := copyEnvironment(t, testFixtureAssumeRoleWebIdentityEnv)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAssumeRoleWebIdentityEnv)

	originalTerragruntConfigPath := util.JoinPath(testFixtureAssumeRoleWebIdentityEnv, "terragrunt.hcl")
	tmpTerragruntConfigFile := util.JoinPath(testPath, "terragrunt.hcl")
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName, options.WithIAMRoleARN(assumeRole), options.WithIAMWebIdentityToken(os.Getenv(tokenEnvVar)))

	copyAndFillMapPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, map[string]string{
		"__FILL_IN_BUCKET_NAME__":            s3BucketName,
		"__FILL_IN_REGION__":                 terraformRemoteStateS3Region,
		"__FILL_IN_ASSUME_ROLE__":            assumeRole,
		"__FILL_IN_IDENTITY_TOKEN_ENV_VAR__": tokenEnvVar,
	})

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply  -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	output := fmt.Sprintf("%s %s", stderr.String(), stdout.String())
	assert.Contains(t, output, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")
}

func TestAwsAssumeRoleWebIdentityFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureAssumeRoleWebIdentityFile)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAssumeRoleWebIdentityFile)

	originalTerragruntConfigPath := util.JoinPath(testFixtureAssumeRoleWebIdentityFile, "terragrunt.hcl")
	tmpTerragruntConfigFile := util.JoinPath(testPath, "terragrunt.hcl")
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())

	assumeRole := os.Getenv("AWS_TEST_S3_ASSUME_ROLE")
	tokenFilePath := os.Getenv("AWS_TEST_S3_IDENTITY_TOKEN_FILE_PATH")

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName, options.WithIAMRoleARN(assumeRole), options.WithIAMWebIdentityToken(tokenFilePath))

	copyAndFillMapPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, map[string]string{
		"__FILL_IN_BUCKET_NAME__":              s3BucketName,
		"__FILL_IN_REGION__":                   terraformRemoteStateS3Region,
		"__FILL_IN_ASSUME_ROLE__":              assumeRole,
		"__FILL_IN_IDENTITY_TOKEN_FILE_PATH__": tokenFilePath,
	})

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply  -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	output := fmt.Sprintf("%s %s", stderr.String(), stdout.String())
	assert.Contains(t, output, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")
}

// Regression testing for https://github.com/gruntwork-io/terragrunt/issues/906
func TestAwsDependencyOutputSameOutputConcurrencyRegression(t *testing.T) {
	t.Parallel()

	// Use func to isolate each test run to a single s3 bucket that is deleted. We run the test multiple times
	// because the underlying error we are trying to test against is nondeterministic, and thus may not always work
	// the first time.
	tt := func() {
		cleanupTerraformFolder(t, testFixtureGetOutput)
		tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
		rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "regression-906")

		// Make sure to fill in the s3 bucket to the config. Also ensure the bucket is deleted before the next for
		// loop call.
		s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s%s", strings.ToLower(uniqueID()), strings.ToLower(uniqueID()))
		defer deleteS3BucketWithRetry(t, terraformRemoteStateS3Region, s3BucketName)
		commonDepConfigPath := util.JoinPath(rootPath, "common-dep", "terragrunt.hcl")
		copyTerragruntConfigAndFillPlaceholders(t, commonDepConfigPath, commonDepConfigPath, s3BucketName, "not-used", "not-used")

		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}
		err := runTerragruntCommand(
			t,
			"terragrunt apply-all --terragrunt-source-update --terragrunt-non-interactive --terragrunt-working-dir "+rootPath,
			&stdout,
			&stderr,
		)
		logBufferContentsLineByLine(t, stdout, "stdout")
		logBufferContentsLineByLine(t, stderr, "stderr")
		require.NoError(t, err)
	}

	for i := 0; i < 3; i++ {
		tt()
		// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
		// This is only a problem during testing, where the process is shared across terragrunt runs.
		config.ClearOutputCache()
	}
}

func TestAwsRemoteStateCodegenGeneratesBackendBlockS3(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "remote-state", "s3")

	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueID())

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, terraformRemoteStateS3Region)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, generateTestCase, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, generateTestCase))
}

func TestAwsOutputFromRemoteState(t *testing.T) { //nolint: paralleltest
	// NOTE: We can't run this test in parallel because there are other tests that also call `config.ClearOutputCache()`, but this function uses a global variable and sometimes it throws an unexpected error:
	// "fixtures/output-from-remote-state/env1/app2/terragrunt.hcl:23,38-48: Unsupported attribute; This object does not have an attribute named "app3_text"."
	// t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := copyEnvironment(t, testFixtureOutputFromRemoteState)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputFromRemoteState, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputFromRemoteState)

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

	runTerragruntRedirectOutput(t, "terragrunt run-all output --terragrunt-fetch-dependency-output-from-state --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir "+environmentPath, &stdout, &stderr)
	output := stdout.String()

	assert.True(t, strings.Contains(output, "app1 output"))
	assert.True(t, strings.Contains(output, "app2 output"))
	assert.True(t, strings.Contains(output, "app3 output"))
	assert.False(t, strings.Contains(stderr.String(), "terraform output -json"))

	assert.True(t, (strings.Index(output, "app3 output") < strings.Index(output, "app1 output")) && (strings.Index(output, "app1 output") < strings.Index(output, "app2 output")))
}

func TestAwsMockOutputsFromRemoteState(t *testing.T) { //nolint: paralleltest
	// NOTE: We can't run this test in parallel because there are other tests that also call `config.ClearOutputCache()`, but this function uses a global variable and sometimes it throws an unexpected error:
	// "fixtures/output-from-remote-state/env1/app2/terragrunt.hcl:23,38-48: Unsupported attribute; This object does not have an attribute named "app3_text"."
	// t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := copyEnvironment(t, testFixtureOutputFromRemoteState)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputFromRemoteState, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := filepath.Join(tmpEnvPath, testFixtureOutputFromRemoteState, "env1")

	// applying only the app1 dependency, the app3 dependency was purposely not applied and should be mocked when running the app2 module
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-fetch-dependency-output-from-state --auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s/app1", environmentPath))
	// Now delete dependencies cached state
	config.ClearOutputCache()
	require.NoError(t, os.Remove(filepath.Join(environmentPath, "/app1/.terraform/terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(environmentPath, "/app1/.terraform")))

	_, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt init --terragrunt-fetch-dependency-output-from-state --terragrunt-non-interactive --terragrunt-working-dir %s/app2", environmentPath))
	require.NoError(t, err)

	assert.Contains(t, stderr, "Failed to read outputs")
	assert.Contains(t, stderr, "fallback to mock outputs")
}

func TestAwsParallelStateInit(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		require.NoError(t, err)
	}
	for i := 0; i < 20; i++ {
		err := util.CopyFolderContents(createLogger(), testFixtureParallelStateInit, tmpEnvPath, ".terragrunt-test", nil, nil)
		require.NoError(t, err)
		err = os.Rename(
			path.Join(tmpEnvPath, "template"),
			path.Join(tmpEnvPath, "app"+strconv.Itoa(i)))
		require.NoError(t, err)
	}

	originalTerragruntConfigPath := util.JoinPath(testFixtureParallelStateInit, "terragrunt.hcl")
	tmpTerragruntConfigFile := util.JoinPath(tmpEnvPath, "terragrunt.hcl")
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueID())
	copyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "us-east-2")

	runTerragrunt(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+tmpEnvPath)
}

func TestAwsAssumeRole(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureAssumeRole)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAssumeRole)

	originalTerragruntConfigPath := util.JoinPath(testFixtureAssumeRole, "terragrunt.hcl")
	tmpTerragruntConfigFile := util.JoinPath(testPath, "terragrunt.hcl")
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueID())
	copyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "us-east-2")

	runTerragrunt(t, "terragrunt validate-inputs -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testPath)

	// validate generated backend.tf
	backendFile := filepath.Join(testPath, "backend.tf")
	assert.FileExists(t, backendFile)

	content, err := files.ReadFileAsString(backendFile)
	require.NoError(t, err)

	opts, err := options.NewTerragruntOptionsForTest(testPath)
	require.NoError(t, err)

	identityARN, err := awshelper.GetAWSIdentityArn(nil, opts)
	require.NoError(t, err)

	assert.Contains(t, content, "role_arn     = \""+identityARN+"\"")
	assert.Contains(t, content, "external_id  = \"external_id_123\"")
	assert.Contains(t, content, "session_name = \"session_name_example\"")
}

func TestAwsInitConfirmation(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())

	tmpEnvPath := copyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt run-all init --terragrunt-working-dir "+tmpEnvPath, &stdout, &stderr)
	require.Error(t, err)
	errout := stderr.String()
	assert.Equal(t, 1, strings.Count(errout, "does not exist or you don't have permissions to access it. Would you like Terragrunt to create it? (y/n)"))
}

func TestAwsRunAllCommandPrompt(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())

	tmpEnvPath := copyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt run-all apply --terragrunt-working-dir "+environmentPath, &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")
	assert.Contains(t, stderr.String(), "Are you sure you want to run 'terragrunt apply' in each folder of the stack described above? (y/n)")
	require.Error(t, err)
}

func TestAwsReadTerragruntAuthProviderCmd(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := copyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "multiple-apps")
	appPath := util.JoinPath(rootPath, "app1")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	runTerragrunt(t, fmt.Sprintf(`terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-auth-provider-cmd %s`, rootPath, mockAuthCmd))

	stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt output -json --terragrunt-working-dir %s --terragrunt-auth-provider-cmd %s", appPath, mockAuthCmd))
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "app1-bar", outputs["foo-app1"].Value)
	assert.Equal(t, "app2-bar", outputs["foo-app2"].Value)
	assert.Equal(t, "app3-bar", outputs["foo-app3"].Value)
}

func TestAwsReadTerragruntAuthProviderCmdWithSops(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := copyEnvironment(t, testFixtureAuthProviderCmd)
	sopsPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "sops")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	runTerragrunt(t, fmt.Sprintf(`terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-auth-provider-cmd %s`, sopsPath, mockAuthCmd))

	stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt output -json --terragrunt-working-dir %s --terragrunt-auth-provider-cmd %s", sopsPath, mockAuthCmd))
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "Welcome to SOPS! Edit this file as you please!", outputs["hello"].Value)
}

func TestAwsReadTerragruntConfigIamRole(t *testing.T) {
	t.Parallel()

	identityArn, err := awshelper.GetAWSIdentityArn(nil, &options.TerragruntOptions{})
	require.NoError(t, err)

	cleanupTerraformFolder(t, testFixtureReadIamRole)

	// Execution outputs to be verified
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	// Invoke terragrunt and verify used IAM role
	err = runTerragruntCommand(t, "terragrunt init --terragrunt-working-dir "+testFixtureReadIamRole, &stdout, &stderr)

	// Since are used not existing AWS accounts, for validation are used success and error outputs
	output := fmt.Sprintf("%v %v %v", stderr.String(), stdout.String(), err.Error())

	// Check that output contains value defined in IAM role
	assert.Contains(t, output, "666666666666")
	// Ensure that state file wasn't created with default IAM value
	assert.True(t, util.FileNotExists(util.JoinPath(testFixtureReadIamRole, identityArn+".txt")))
}

func dependencyOutputOptimizationTest(t *testing.T, moduleName string, forceInit bool, expectedOutputLogs []string) {
	t.Helper()

	expectedOutput := `They said, "No, The answer is 42"`
	generatedUniqueID := uniqueID()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, moduleName)
	rootTerragruntConfigPath := filepath.Join(rootPath, config.DefaultTerragruntConfigPath)
	livePath := filepath.Join(rootPath, "live")
	deepDepPath := filepath.Join(rootPath, "deepdep")
	depPath := filepath.Join(rootPath, "dep")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(generatedUniqueID)
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(generatedUniqueID)
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, terraformRemoteStateS3Region)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, terraformRemoteStateS3Region)

	runTerragrunt(t, "terragrunt apply-all --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// verify expected output
	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir "+livePath)
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

	fmt.Println("terragrunt output -no-color -json --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir " + livePath)

	reout, reerr, err := runTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir "+livePath)
	require.NoError(t, err)

	require.NoError(t, json.Unmarshal([]byte(reout), &outputs))
	assert.Equal(t, expectedOutput, outputs["output"].Value)

	for _, logRegexp := range expectedOutputLogs {
		assert.Regexp(t, logRegexp, reerr)
	}
}

func assertS3Tags(t *testing.T, expectedTags map[string]string, bucketName string, client *s3.S3) {
	t.Helper()

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

// Check that the DynamoDB table of the given name and region exists. Terragrunt should create this table during the test.
// Also check if table got tagged properly
func validateDynamoDBTableExistsAndIsTagged(t *testing.T, awsRegion string, tableName string, expectedTags map[string]string) {
	t.Helper()

	client := createDynamoDBClientForTest(t, awsRegion)

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

// Check that the S3 Bucket of the given name and region exists. Terragrunt should create this bucket during the test.
// Also check if bucket got tagged properly and that public access is disabled completely.
func validateS3BucketExistsAndIsTagged(t *testing.T, awsRegion string, bucketName string, expectedTags map[string]string) {
	t.Helper()

	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		t.Fatalf("Error creating mockOptions: %v", err)
	}

	sessionConfig := &awshelper.AwsSessionConfig{
		Region: awsRegion,
	}

	s3Client, err := remote.CreateS3Client(sessionConfig, mockOptions)
	if err != nil {
		t.Fatalf("Error creating S3 client: %v", err)
	}

	assert.True(t, remote.DoesS3BucketExist(s3Client, &bucketName), "Terragrunt failed to create remote state S3 bucket %s", bucketName)

	if expectedTags != nil {
		assertS3Tags(t, expectedTags, bucketName, s3Client)
	}

	assertS3PublicAccessBlocks(t, s3Client, bucketName)
}

func assertS3PublicAccessBlocks(t *testing.T, client *s3.S3, bucketName string) {
	t.Helper()

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

func bucketEncryption(t *testing.T, awsRegion string, bucketName string) (*s3.GetBucketEncryptionOutput, error) {
	t.Helper()

	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		t.Logf("Error creating mockOptions: %v", err)
		return nil, err
	}

	sessionConfig := &awshelper.AwsSessionConfig{
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
		// TODO: Remove this lint suppression
		return nil, nil //nolint:nilerr
	}

	return output, nil
}

// deleteS3BucketWithRetry will attempt to delete the specified S3 bucket, retrying up to 3 times if there are errors to
// handle eventual consistency issues.
func deleteS3BucketWithRetry(t *testing.T, awsRegion string, bucketName string) {
	t.Helper()

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

// createS3Bucket creates a test S3 bucket for state.
func createS3Bucket(t *testing.T, awsRegion string, bucketName string) {
	t.Helper()

	err := createS3BucketE(t, awsRegion, bucketName)
	require.NoError(t, err)
}

// createS3BucketE create test S3 bucket.
func createS3BucketE(t *testing.T, awsRegion string, bucketName string) error {
	t.Helper()

	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		t.Logf("Error creating mockOptions: %v", err)
		return err
	}

	sessionConfig := &awshelper.AwsSessionConfig{
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

func cleanupTableForTest(t *testing.T, tableName string, awsRegion string) {
	t.Helper()

	client := createDynamoDBClientForTest(t, awsRegion)
	err := terragruntDynamoDb.DeleteTable(tableName, client)
	require.NoError(t, err)
}

// Create an authenticated client for DynamoDB
func createDynamoDBClient(awsRegion, awsProfile string, iamRoleArn string) (*dynamodb.DynamoDB, error) {
	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		return nil, err
	}

	sessionConfig := &awshelper.AwsSessionConfig{
		Region:  awsRegion,
		Profile: awsProfile,
		RoleArn: iamRoleArn,
	}

	session, err := awshelper.CreateAwsSession(sessionConfig, mockOptions)
	if err != nil {
		return nil, err
	}

	return dynamodb.New(session), nil
}

func bucketPolicy(t *testing.T, awsRegion string, bucketName string) (*s3.GetBucketPolicyOutput, error) {
	t.Helper()

	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	if err != nil {
		t.Logf("Error creating mockOptions: %v", err)
		return nil, err
	}

	sessionConfig := &awshelper.AwsSessionConfig{
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

func createDynamoDBClientForTest(t *testing.T, awsRegion string) *dynamodb.DynamoDB {
	t.Helper()

	client, err := createDynamoDBClient(awsRegion, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// createDynamoDBTableE creates a test DynamoDB table, and returns an error if the table creation fails.
func createDynamoDBTableE(t *testing.T, awsRegion string, tableName string) error {
	t.Helper()

	client := createDynamoDBClientForTest(t, awsRegion)
	_, err := client.CreateTable(&dynamodb.CreateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String("LockID"),
				AttributeType: aws.String("S"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("LockID"),
				KeyType:       aws.String("HASH"),
			},
		},
		TableName: aws.String(tableName),
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
	})
	if err != nil {
		return err
	}
	client.WaitUntilTableExists(&dynamodb.DescribeTableInput{TableName: aws.String(tableName)})
	return nil
}

// createDynamoDBTable creates a test DynamoDB table.
func createDynamoDBTable(t *testing.T, awsRegion string, tableName string) {
	t.Helper()

	err := createDynamoDBTableE(t, awsRegion, tableName)
	require.NoError(t, err)
}
