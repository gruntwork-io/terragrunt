//go:build aws

package test_test

import (
	"bytes"
	"fmt"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/util"
	terraws "github.com/gruntwork-io/terratest/modules/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	terraformRemoteStateS3Region = "us-west-2"

	testFixtureAwsProviderPatch     = "fixture-aws-provider-patch"
	testFixtureAwsGetCallerIdentity = "fixture-get-aws-caller-identity"
	testFixtureS3Errors             = "fixture-s3-errors/"
)

func TestTerragruntInitHookNoSourceWithBackend(t *testing.T) {
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

func TestTerragruntInitHookWithSourceWithBackend(t *testing.T) {
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

func TestTerragruntBeforeAfterAndErrorMergeHook(t *testing.T) {
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

func TestTerragruntWorksWithLocalTerraformVersion(t *testing.T) {
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
func TestTerragruntSetsAccessLoggingForTfSTateS3BuckeToADifferentBucketWithGivenTargetPrefix(t *testing.T) {
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
func TestTerragruntSetsAccessLoggingForTfSTateS3BuckeToADifferentBucketWithDefaultTargetPrefix(t *testing.T) {
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

func TestTerragruntRunAllCommand(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := copyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	runTerragrunt(t, "terragrunt run-all init --terragrunt-non-interactive --terragrunt-working-dir "+environmentPath)
}

func TestTerragruntOutputAllCommand(t *testing.T) {
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

func TestTerragruntOutputFromDependency(t *testing.T) {
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

func TestTerragruntValidateAllCommand(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())
	defer deleteS3Bucket(t, terraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := copyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	runTerragrunt(t, "terragrunt validate-all --terragrunt-non-interactive --terragrunt-working-dir "+environmentPath)
}

func TestTerragruntOutputAllCommandSpecificVariableIgnoreDependencyErrors(t *testing.T) {
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

func TestTerragruntStackCommands(t *testing.T) { //nolint paralleltest
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
