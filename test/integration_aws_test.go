//go:build aws

package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/options"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/util"
	terraws "github.com/gruntwork-io/terratest/modules/aws"
	"github.com/gruntwork-io/terratest/modules/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureAwsProviderPatch     = "fixture-aws-provider-patch"
	testFixtureAwsGetCallerIdentity = "fixture-get-aws-caller-identity"
	testFixtureS3Errors             = "fixture-s3-errors/"
)

func TestAwsInitHookNoSourceWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())

	cleanupTerraformFolder(t, testFixtureHooksInitOnceNoSourceWithBackend)
	tmpEnvPath := copyEnvironment(t, "fixture-hooks/init-once")
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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())

	cleanupTerraformFolder(t, testFixtureHooksInitOnceWithSourceWithBackend)
	tmpEnvPath := copyEnvironment(t, "fixture-hooks/init-once")
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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueId())

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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
	s3BucketLogsName := s3BucketName + "-tf-state-logs"
	s3BucketLogsTargetPrefix := "logs/"
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueId())

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

	policyInBucket, err := aws_helper.UnmarshalPolicy(*policy.Policy)
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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
	s3BucketLogsName := s3BucketName + "-tf-state-logs"
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueId())

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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := copyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	runTerragrunt(t, "terragrunt run-all init --terragrunt-non-interactive --terragrunt-working-dir "+environmentPath)
}

func TestAwsOutputAllCommand(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := copyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	runTerragrunt(t, "terragrunt validate-all --terragrunt-non-interactive --terragrunt-working-dir "+environmentPath)
}

func TestAwsOutputAllCommandSpecificVariableIgnoreDependencyErrors(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueId())

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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
	lockTableName := "terragrunt-lock-table-" + strings.ToLower(uniqueId())

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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
	lockTableName := "terragrunt-lock-table-" + strings.ToLower(uniqueId())

	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, terraformRemoteStateS3Region)

	tmpEnvPath := copyEnvironment(t, "fixture-download")
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
	generatedUniqueId := uniqueId()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "nested-optimization-disable")
	rootTerragruntConfigPath := filepath.Join(rootPath, config.DefaultTerragruntConfigPath)
	livePath := filepath.Join(rootPath, "live")
	deepDepPath := filepath.Join(rootPath, "deepdep")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(generatedUniqueId)
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(generatedUniqueId)
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

	stderr := new(bytes.Buffer)
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

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt aws-provider-patch --terragrunt-override-attr region=\"eu-west-1\" --terragrunt-override-attr allowed_account_ids=[\"00000000000\"] --terragrunt-working-dir %s --terragrunt-log-level debug", modulePath), os.Stdout, stderr),
	)
	t.Log(stderr.String())

	assert.Regexp(t, "Patching AWS provider in .+test/fixture-aws-provider-patch/example-module/main.tf", stderr.String())

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
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueId())

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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueId())

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

func dependencyOutputOptimizationTest(t *testing.T, moduleName string, forceInit bool, expectedOutputLogs []string) {
	t.Helper()

	expectedOutput := `They said, "No, The answer is 42"`
	generatedUniqueId := uniqueId()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, moduleName)
	rootTerragruntConfigPath := filepath.Join(rootPath, config.DefaultTerragruntConfigPath)
	livePath := filepath.Join(rootPath, "live")
	deepDepPath := filepath.Join(rootPath, "deepdep")
	depPath := filepath.Join(rootPath, "dep")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(generatedUniqueId)
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(generatedUniqueId)
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

// Check that the S3 Bucket of the given name and region exists. Terragrunt should create this bucket during the test.
// Also check if bucket got tagged properly and that public access is disabled completely.
func validateS3BucketExistsAndIsTagged(t *testing.T, awsRegion string, bucketName string, expectedTags map[string]string) {
	t.Helper()

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
		// TODO: Remove this lint suppression
		return nil, nil //nolint:nilerr
	}

	return output, nil
}
