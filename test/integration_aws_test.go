//go:build aws || awsgcp

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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"

	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/util"
	terraws "github.com/gruntwork-io/terratest/modules/aws"
	"github.com/gruntwork-io/terratest/modules/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
)

const (
	testFixtureAwsProviderPatch                  = "fixtures/aws-provider-patch"
	testFixtureAwsAccountAlias                   = "fixtures/get-aws-account-alias"
	testFixtureAwsGetCallerIdentity              = "fixtures/get-aws-caller-identity"
	testFixtureS3Errors                          = "fixtures/s3-errors/"
	testFixtureAssumeRole                        = "fixtures/assume-role/external-id"
	testFixtureAssumeRoleDuration                = "fixtures/assume-role/duration"
	testFixtureReadIamRole                       = "fixtures/read-config/iam_role_in_file"
	testFixtureOutputFromRemoteState             = "fixtures/output-from-remote-state"
	testFixtureOutputFromDependency              = "fixtures/output-from-dependency"
	testFixtureS3Backend                         = "fixtures/s3-backend"
	testFixtureS3BackendDualLocking              = "fixtures/s3-backend/dual-locking"
	testFixtureS3BackendUseLockfile              = "fixtures/s3-backend/use-lockfile"
	testFixtureAssumeRoleWithExternalIDWithComma = "fixtures/assume-role/external-id-with-comma"

	qaMyAppRelPath = "qa/my-app"
)

func TestAwsBootstrapBackend(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		checkExpectedResultFn func(t *testing.T, output string, s3BucketName, dynamoDBName string)
		name                  string
		args                  string
	}{
		{
			name: "no bootstrap s3 backend without flag",
			args: "run apply",
			checkExpectedResultFn: func(t *testing.T, output string, s3BucketName, dynamoDBName string) {
				t.Helper()

				assert.Regexp(t, "(S3 bucket must have been previously created)|(S3 bucket does not exist)", output)
			},
		},
		{
			name: "bootstrap s3 backend with flag",
			args: "run apply --backend-bootstrap",
			checkExpectedResultFn: func(t *testing.T, output string, s3BucketName, dynamoDBName string) {
				t.Helper()

				validateS3BucketExistsAndIsTaggedAndVersioning(t, helpers.TerraformRemoteStateS3Region, s3BucketName, true, nil)
				validateDynamoDBTableExistsAndIsTaggedAndIsSSEncrypted(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, nil, false)
			},
		},
		{
			name: "bootstrap s3 backend with lock table ssencryption",
			args: "run apply --backend-bootstrap --feature enable_lock_table_ssencryption=true",
			checkExpectedResultFn: func(t *testing.T, output string, s3BucketName, dynamoDBName string) {
				t.Helper()

				validateS3BucketExistsAndIsTaggedAndVersioning(t, helpers.TerraformRemoteStateS3Region, s3BucketName, true, nil)
				validateDynamoDBTableExistsAndIsTaggedAndIsSSEncrypted(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, nil, true)
			},
		},
		{
			name: "bootstrap s3 backend by backend command",
			args: "backend bootstrap",
			checkExpectedResultFn: func(t *testing.T, output string, s3BucketName, dynamoDBName string) {
				t.Helper()

				validateS3BucketExistsAndIsTaggedAndVersioning(t, helpers.TerraformRemoteStateS3Region, s3BucketName, true, nil)
				validateDynamoDBTableExistsAndIsTaggedAndIsSSEncrypted(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, nil, false)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureS3Backend)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3Backend)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureS3Backend)

			testID := strings.ToLower(helpers.UniqueID())

			s3BucketName := "terragrunt-test-bucket-" + testID
			dynamoDBName := "terragrunt-test-dynamodb-" + testID

			if tc.name != "no bootstrap s3 backend without flag" {
				defer func() {
					deleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
					cleanupTableForTest(t, dynamoDBName, helpers.TerraformRemoteStateS3Region)
				}()
			}

			commonConfigPath := util.JoinPath(rootPath, "common.hcl")
			helpers.CopyTerragruntConfigAndFillPlaceholders(t, commonConfigPath, commonConfigPath, s3BucketName, dynamoDBName, helpers.TerraformRemoteStateS3Region)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt "+tc.args+" --all --non-interactive --log-level debug --working-dir "+rootPath)
			require.NoError(t, err)

			tc.checkExpectedResultFn(t, stdout+stderr, s3BucketName, dynamoDBName)
		})
	}
}

func TestAwsDualLockingBackend(t *testing.T) {
	t.Parallel()

	if !helpers.IsNativeS3LockingSupported(t) {
		t.Skip("Wrapped binary does not support native S3 locking")
		return
	}

	helpers.CleanupTerraformFolder(t, testFixtureS3BackendDualLocking)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3BackendDualLocking)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureS3BackendDualLocking)

	testID := strings.ToLower(helpers.UniqueID())

	s3BucketName := "terragrunt-test-bucket-" + testID
	dynamoDBName := "terragrunt-test-dynamodb-" + testID

	defer func() {
		deleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
		cleanupTableForTest(t, dynamoDBName, helpers.TerraformRemoteStateS3Region)
	}()

	terragruntConfigPath := util.JoinPath(rootPath, "terragrunt.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, terragruntConfigPath, terragruntConfigPath, s3BucketName, dynamoDBName, helpers.TerraformRemoteStateS3Region)

	// Test backend bootstrap with dual locking
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run apply --backend-bootstrap --non-interactive --log-level debug --working-dir "+rootPath+" -- -auto-approve")
	require.NoError(t, err)

	// Validate both S3 bucket and DynamoDB table are created
	validateS3BucketExistsAndIsTaggedAndVersioning(t, helpers.TerraformRemoteStateS3Region, s3BucketName, true, nil)
	validateDynamoDBTableExistsAndIsTaggedAndIsSSEncrypted(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, nil, false)

	t.Logf("Dual locking test completed successfully. Output: %s, Errors: %s", stdout, stderr)

	// Test that subsequent runs work with dual locking (both locks should be acquired)
	stdout2, stderr2, err2 := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run plan --non-interactive --log-level debug --working-dir "+rootPath)
	require.NoError(t, err2)

	t.Logf("Dual locking plan test completed successfully. Output: %s, Errors: %s", stdout2, stderr2)
}

func TestAwsNativeS3LockingBackend(t *testing.T) {
	t.Parallel()

	if !helpers.IsNativeS3LockingSupported(t) {
		t.Skip("Wrapped binary does not support native S3 locking")
		return
	}

	helpers.CleanupTerraformFolder(t, testFixtureS3BackendUseLockfile)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3BackendUseLockfile)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureS3BackendUseLockfile)

	testID := strings.ToLower(helpers.UniqueID())

	s3BucketName := "terragrunt-test-bucket-" + testID
	// Note: No DynamoDB table needed for native S3 locking

	defer func() {
		deleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
		// Note: No DynamoDB cleanup needed for S3 native locking
	}()

	terragruntConfigPath := util.JoinPath(rootPath, "terragrunt.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, terragruntConfigPath, terragruntConfigPath, s3BucketName, "unused-dynamodb-name", helpers.TerraformRemoteStateS3Region)

	// Test backend bootstrap with S3 native locking only
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run apply --backend-bootstrap --non-interactive --log-level debug --working-dir "+rootPath+" -- -auto-approve")
	require.NoError(t, err)

	// Validate S3 bucket is created and versioned
	validateS3BucketExistsAndIsTaggedAndVersioning(t, helpers.TerraformRemoteStateS3Region, s3BucketName, true, nil)

	// Note: No DynamoDB table validation - S3 native locking doesn't use DynamoDB

	t.Logf("S3 native locking test completed successfully. Output: %s, Errors: %s", stdout, stderr)

	// Test that subsequent runs work with S3 native locking only
	stdout2, stderr2, err2 := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run plan --non-interactive --log-level debug --working-dir "+rootPath)
	require.NoError(t, err2)

	t.Logf("S3 native locking plan test completed successfully. Output: %s, Errors: %s", stdout2, stderr2)
}

func TestAwsBootstrapBackendWithoutVersioning(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureS3Backend)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3Backend)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureS3Backend)

	testID := strings.ToLower(helpers.UniqueID())

	s3BucketName := "terragrunt-test-bucket-" + testID
	dynamoDBName := "terragrunt-test-dynamodb-" + testID

	defer func() {
		deleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
		cleanupTableForTest(t, dynamoDBName, helpers.TerraformRemoteStateS3Region)
	}()

	commonConfigPath := util.JoinPath(rootPath, "common.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, commonConfigPath, commonConfigPath, s3BucketName, dynamoDBName, helpers.TerraformRemoteStateS3Region)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-level debug --working-dir "+rootPath+" apply --backend-bootstrap --feature disable_versioning=true")
	require.NoError(t, err)

	validateS3BucketExistsAndIsTaggedAndVersioning(t, helpers.TerraformRemoteStateS3Region, s3BucketName, false, nil)
	validateDynamoDBTableExistsAndIsTaggedAndIsSSEncrypted(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, nil, false)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt --non-interactive --log-level debug --working-dir "+rootPath+" backend delete --backend-bootstrap --feature disable_versioning=true --all")
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt --non-interactive --log-level debug --working-dir "+rootPath+" backend delete --backend-bootstrap --feature disable_versioning=true --all --force")
	require.NoError(t, err)
}

func TestAwsBootstrapBackendWithAccessLogging(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureS3Backend)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3Backend)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureS3Backend)

	testID := strings.ToLower(helpers.UniqueID())

	s3BucketName := "terragrunt-test-bucket-" + testID
	s3AccessLogsBucketName := "terragrunt-test-bucket-" + testID + "-access-logs"
	dynamoDBName := "terragrunt-test-dynamodb-" + testID

	defer func() {
		deleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
		deleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3AccessLogsBucketName)
		cleanupTableForTest(t, dynamoDBName, helpers.TerraformRemoteStateS3Region)
	}()

	commonConfigPath := util.JoinPath(rootPath, "common.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, commonConfigPath, commonConfigPath, s3BucketName, dynamoDBName, helpers.TerraformRemoteStateS3Region)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-level debug --working-dir "+rootPath+" --feature access_logging_bucket="+s3AccessLogsBucketName+" apply --backend-bootstrap")
	require.NoError(t, err)

	validateS3BucketExistsAndIsTaggedAndVersioning(t, helpers.TerraformRemoteStateS3Region, s3BucketName, true, nil)
	validateS3BucketExistsAndIsTaggedAndVersioning(t, helpers.TerraformRemoteStateS3Region, s3AccessLogsBucketName, true, nil)
	validateDynamoDBTableExistsAndIsTaggedAndIsSSEncrypted(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, nil, false)
}

func TestAwsMigrateBackendWithoutVersioning(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureS3Backend)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3Backend)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureS3Backend)
	unitPath := util.JoinPath(rootPath, "unit1")

	testID := strings.ToLower(helpers.UniqueID())

	s3BucketName := "terragrunt-test-bucket-" + testID
	dynamoDBName := "terragrunt-test-dynamodb-" + testID

	defer func() {
		deleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
		cleanupTableForTest(t, dynamoDBName, helpers.TerraformRemoteStateS3Region)
	}()

	commonConfigPath := util.JoinPath(rootPath, "common.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, commonConfigPath, commonConfigPath, s3BucketName, dynamoDBName, helpers.TerraformRemoteStateS3Region)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --non-interactive --log-level debug --working-dir "+unitPath+" --feature disable_versioning=true apply --backend-bootstrap -- -auto-approve")
	require.NoError(t, err)

	validateS3BucketExistsAndIsTaggedAndVersioning(t, helpers.TerraformRemoteStateS3Region, s3BucketName, false, nil)
	validateDynamoDBTableExistsAndIsTaggedAndIsSSEncrypted(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, nil, false)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt --non-interactive --log-level debug --working-dir "+rootPath+" backend migrate --backend-bootstrap --feature disable_versioning=true unit1 unit2")
	require.Error(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt --non-interactive --log-level debug --working-dir "+rootPath+" backend migrate --backend-bootstrap --feature disable_versioning=true --force unit1 unit2")
	require.NoError(t, err)
}

func TestAwsDeleteBackend(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureS3Backend)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3Backend)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureS3Backend)

	testID := strings.ToLower(helpers.UniqueID())

	s3BucketName := "terragrunt-test-bucket-" + testID
	dynamoDBName := "terragrunt-test-dynamodb-" + testID

	defer func() {
		deleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
		cleanupTableForTest(t, dynamoDBName, helpers.TerraformRemoteStateS3Region)
	}()

	commonConfigPath := util.JoinPath(rootPath, "common.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, commonConfigPath, commonConfigPath, s3BucketName, dynamoDBName, helpers.TerraformRemoteStateS3Region)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run apply --backend-bootstrap --all --non-interactive --log-level debug --working-dir "+rootPath)
	require.NoError(t, err)

	remoteStateKeys := []string{
		"unit1/tofu.tfstate",
		"unit2/tofu.tfstate",
	}

	for _, key := range remoteStateKeys {
		tableKey := path.Join(s3BucketName, key+"-md5")

		assert.True(t, doesS3BucketKeyExist(t, helpers.TerraformRemoteStateS3Region, s3BucketName, key), "S3 bucket key %s must exist", key)
		assert.True(t, doesDynamoDBTableItemExist(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, tableKey), "DynamoDB table key %s must exist", tableKey)
	}

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt backend delete --all --non-interactive --log-level debug --working-dir "+rootPath)
	require.NoError(t, err)

	for _, key := range remoteStateKeys {
		tableKey := path.Join(s3BucketName, key+"-md5")

		assert.False(t, doesS3BucketKeyExist(t, helpers.TerraformRemoteStateS3Region, s3BucketName, key), "S3 bucket key %s must not exist", key)
		assert.False(t, doesDynamoDBTableItemExist(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, tableKey), "DynamoDB table key %s must not exist", tableKey)
	}
}

func TestAwsMigrateBackend(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureS3Backend)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3Backend)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureS3Backend)

	testID := strings.ToLower(helpers.UniqueID())

	s3BucketName := "terragrunt-test-bucket-" + testID
	dynamoDBName := "terragrunt-test-dynamodb-" + testID

	unit1Path := util.JoinPath(rootPath, "unit1")
	unit2Path := util.JoinPath(rootPath, "unit2")

	unit1BackendKey := "unit1/tofu.tfstate"
	unit2BackendKey := "unit2/tofu.tfstate"

	unit1TableKey := path.Join(s3BucketName, unit1BackendKey+"-md5")
	unit2TableKey := path.Join(s3BucketName, unit2BackendKey+"-md5")

	defer func() {
		deleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
		cleanupTableForTest(t, dynamoDBName, helpers.TerraformRemoteStateS3Region)
	}()

	commonConfigPath := util.JoinPath(rootPath, "common.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, commonConfigPath, commonConfigPath, s3BucketName, dynamoDBName, helpers.TerraformRemoteStateS3Region)

	// Bootstrap backend and create remote state for unit1.

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run apply --backend-bootstrap --non-interactive --log-level debug --working-dir "+unit1Path+" -- -auto-approve")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Changes to Outputs")

	// Check for remote states.

	assert.True(t, doesS3BucketKeyExist(t, helpers.TerraformRemoteStateS3Region, s3BucketName, unit1BackendKey), "S3 bucket key %s must exist", unit1BackendKey)
	assert.True(t, doesDynamoDBTableItemExist(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, unit1TableKey), "DynamoDB table key %s must exist", unit1TableKey)
	assert.False(t, doesS3BucketKeyExist(t, helpers.TerraformRemoteStateS3Region, s3BucketName, unit2BackendKey), "S3 bucket key %s must not exist", unit2BackendKey)
	assert.False(t, doesDynamoDBTableItemExist(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, unit2TableKey), "DynamoDB table key %s must not exist", unit2TableKey)

	// Migrate remote state from unit1 to unit2.
	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt backend migrate --log-level debug --working-dir "+rootPath+" unit1 unit2")
	require.NoError(t, err)

	// Check for remote states after migration.
	assert.False(t, doesS3BucketKeyExist(t, helpers.TerraformRemoteStateS3Region, s3BucketName, unit1BackendKey), "S3 bucket key %s must not exist", unit1BackendKey)
	assert.False(t, doesDynamoDBTableItemExist(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, unit1TableKey), "DynamoDB table key %s must not exist", unit1TableKey)
	assert.True(t, doesS3BucketKeyExist(t, helpers.TerraformRemoteStateS3Region, s3BucketName, unit2BackendKey), "S3 bucket key %s must exist", unit2BackendKey)
	assert.True(t, doesDynamoDBTableItemExist(t, helpers.TerraformRemoteStateS3Region, dynamoDBName, unit2TableKey), "DynamoDB table key %s must exist", unit2TableKey)

	// Run `tofu apply` for unit2 with migrated remote state from unit1.

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run apply --backend-bootstrap --non-interactive --log-level debug --working-dir "+unit2Path+" -- -auto-approve")
	require.NoError(t, err)
	assert.Contains(t, stdout, "No changes")
}

func TestAwsInitHookNoSourceWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	helpers.CleanupTerraformFolder(t, testFixtureHooksInitOnceNoSourceWithBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInitOnceNoSourceWithBackend)

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	rootTerragruntConfigPath := util.JoinPath(rootPath, "terragrunt.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", helpers.TerraformRemoteStateS3Region)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --backend-bootstrap --working-dir "+rootPath, &stdout, &stderr)
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

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	helpers.CleanupTerraformFolder(t, testFixtureHooksInitOnceWithSourceWithBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInitOnceWithSourceWithBackend)

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	rootTerragruntConfigPath := util.JoinPath(rootPath, "terragrunt.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", helpers.TerraformRemoteStateS3Region)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --backend-bootstrap --working-dir "+rootPath, &stdout, &stderr)
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
	helpers.CleanupTerraformFolder(t, childPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	t.Logf("bucketName: %s", s3BucketName)
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfigWithParentAndChild(t, testFixtureHooksBeforeAfterAndErrorMergePath, qaMyAppRelPath, s3BucketName, "root.hcl", config.DefaultTerragruntConfigPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --backend-bootstrap --config %s --working-dir %s", tmpTerragruntConfigPath, childPath), &stdout, &stderr)
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

	helpers.CleanupTerraformFolder(t, testFixturePath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(t, testFixturePath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --backend-bootstrap --config %s --working-dir %s", tmpTerragruntConfigPath, testFixturePath))

	var expectedS3Tags = map[string]string{
		"owner": "terragrunt integration test",
		"name":  "Terraform state storage"}
	validateS3BucketExistsAndIsTaggedAndVersioning(t, helpers.TerraformRemoteStateS3Region, s3BucketName, true, expectedS3Tags)

	var expectedDynamoDBTableTags = map[string]string{
		"owner": "terragrunt integration test",
		"name":  "Terraform lock table"}
	validateDynamoDBTableExistsAndIsTaggedAndIsSSEncrypted(t, helpers.TerraformRemoteStateS3Region, lockTableName, expectedDynamoDBTableTags, true)
}

// Regression test to ensure that `accesslogging_bucket_name` and `accesslogging_target_prefix` are taken into account
// & the TargetLogs bucket is set to a new S3 bucket, different from the origin S3 bucket
// & the logs objects are prefixed with the `accesslogging_target_prefix` value
func TestAwsSetsAccessLoggingForTfSTateS3BuckeToADifferentBucketWithGivenTargetPrefix(t *testing.T) {
	t.Parallel()

	examplePath := filepath.Join(testFixtureRegressions, "accesslogging-bucket/with-target-prefix-input")
	helpers.CleanupTerraformFolder(t, examplePath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	s3BucketLogsName := s3BucketName + "-tf-state-logs"
	s3BucketLogsTargetPrefix := "logs/"
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(
		t,
		examplePath,
		s3BucketName,
		lockTableName,
		"remote_terragrunt.hcl",
	)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt validate --non-interactive --backend-bootstrap --config %s --working-dir %s", tmpTerragruntConfigPath, examplePath))

	targetLoggingBucket := terraws.GetS3BucketLoggingTarget(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	targetLoggingBucketPrefix := terraws.GetS3BucketLoggingTargetPrefix(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	assert.Equal(t, s3BucketLogsName, targetLoggingBucket)
	assert.Equal(t, s3BucketLogsTargetPrefix, targetLoggingBucketPrefix)

	encryptionConfig, err := bucketEncryption(t, helpers.TerraformRemoteStateS3Region, targetLoggingBucket)
	require.NoError(t, err)
	assert.NotNil(t, encryptionConfig)
	assert.NotNil(t, encryptionConfig.ServerSideEncryptionConfiguration)
	for _, rule := range encryptionConfig.ServerSideEncryptionConfiguration.Rules {
		if rule.ApplyServerSideEncryptionByDefault != nil {
			assert.Equal(t, s3types.ServerSideEncryptionAes256, rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm)
		}
	}

	policy, err := bucketPolicy(t, helpers.TerraformRemoteStateS3Region, targetLoggingBucket)
	require.NoError(t, err)
	assert.NotNil(t, policy.Policy)

	policyInBucket, err := awshelper.UnmarshalPolicy(*policy.Policy)
	require.NoError(t, err)
	enforceSSE := false
	if policyInBucket.Statement != nil {
		for _, statement := range policyInBucket.Statement {
			if statement.Sid == s3backend.SidEnforcedTLSPolicy {
				enforceSSE = true
			}
		}
	}
	assert.True(t, enforceSSE)
}

// Regression test to ensure that `accesslogging_bucket_name` is taken into account
// & when no `accesslogging_target_prefix` provided, then **default** value is used for TargetPrefix
func TestAwsSetsAccessLoggingForTfSTateS3BucketToADifferentBucketWithDefaultTargetPrefix(t *testing.T) {
	t.Parallel()

	examplePath := filepath.Join(testFixtureRegressions, "accesslogging-bucket/no-target-prefix-input")
	helpers.CleanupTerraformFolder(t, examplePath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	s3BucketLogsName := s3BucketName + "-tf-state-logs"
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(
		t,
		examplePath,
		s3BucketName,
		lockTableName,
		"remote_terragrunt.hcl",
	)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt validate --non-interactive --backend-bootstrap --config %s --working-dir %s", tmpTerragruntConfigPath, examplePath))

	targetLoggingBucket := terraws.GetS3BucketLoggingTarget(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	targetLoggingBucketPrefix := terraws.GetS3BucketLoggingTargetPrefix(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	encryptionConfig, err := bucketEncryption(t, helpers.TerraformRemoteStateS3Region, targetLoggingBucket)
	require.NoError(t, err)
	assert.NotNil(t, encryptionConfig)
	assert.NotNil(t, encryptionConfig.ServerSideEncryptionConfiguration)
	for _, rule := range encryptionConfig.ServerSideEncryptionConfiguration.Rules {
		if rule.ApplyServerSideEncryptionByDefault != nil {
			assert.Equal(t, s3types.ServerSideEncryptionAes256, rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm)
		}
	}

	assert.Equal(t, s3BucketLogsName, targetLoggingBucket)
	assert.Equal(t, s3backend.DefaultS3BucketAccessLoggingTargetPrefix, targetLoggingBucketPrefix)
}

func TestAwsRunAllCommand(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, "root.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	helpers.RunTerragrunt(t, "terragrunt run --all init --non-interactive --working-dir "+environmentPath)
}

func TestAwsOutputAllCommand(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, "root.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --backend-bootstrap --working-dir "+environmentPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	helpers.RunTerragruntRedirectOutput(t, "terragrunt run --all output --non-interactive --backend-bootstrap --working-dir "+environmentPath, &stdout, &stderr)
	output := stdout.String()

	assert.Contains(t, output, "app1 output")
	assert.Contains(t, output, "app2 output")
	assert.Contains(t, output, "app3 output")

	assert.True(t, (strings.Index(output, "app3 output") < strings.Index(output, "app1 output")) && (strings.Index(output, "app1 output") < strings.Index(output, "app2 output")))
}

func TestAwsOutputFromDependency(t *testing.T) {
	// t.Parallel() cannot be used together with t.Setenv()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputFromDependency)

	rootTerragruntPath := util.JoinPath(tmpEnvPath, testFixtureOutputFromDependency)
	depTerragruntConfigPath := util.JoinPath(rootTerragruntPath, "dependency", config.DefaultTerragruntConfigPath)

	helpers.CopyTerragruntConfigAndFillPlaceholders(t, depTerragruntConfigPath, depTerragruntConfigPath, s3BucketName, "not-used", helpers.TerraformRemoteStateS3Region)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	t.Setenv("AWS_CSM_ENABLED", "true")

	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt run --all --non-interactive --working-dir %s --log-level trace", rootTerragruntPath)+" -- apply -auto-approve", &stdout, &stderr)
	require.NoError(t, err)

	output := stderr.String()
	assert.NotContains(t, output, "invalid character")
}

func TestAwsValidateAllCommand(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, "root.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	helpers.RunTerragrunt(t, "terragrunt run --all validate --non-interactive --working-dir "+environmentPath)
}

func TestAwsOutputAllCommandSpecificVariableIgnoreDependencyErrors(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, "root.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --backend-bootstrap --working-dir "+environmentPath)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	// Call helpers.RunTerragruntCommand directly because this command contains failures (which causes helpers.RunTerragruntRedirectOutput to abort) but we don't care.
	err := helpers.RunTerragruntCommand(t, "terragrunt run --all output app2_text --queue-ignore-errors --non-interactive --backend-bootstrap --working-dir "+environmentPath, &stdout, &stderr)
	require.NoError(t, err)
	output := stdout.String()

	helpers.LogBufferContentsLineByLine(t, stdout, "run --all output stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "run --all output stderr")

	// Without --queue-ignore-errors, app2 never runs because its dependencies have "errors" since they don't have the output "app2_text".
	assert.Contains(t, output, "app2 output")
}

func TestAwsStackCommands(t *testing.T) { //nolint paralleltest
	// It seems that disabling parallel test execution helps avoid the CircleCi error: "NoSuchBucket Policy: The bucket policy does not exist."
	// t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	helpers.CleanupTerraformFolder(t, testFixtureStack)
	helpers.CleanupTerragruntFolder(t, testFixtureStack)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStack)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureStack, "root.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	mgmtEnvironmentPath := util.JoinPath(tmpEnvPath, testFixtureStack, "mgmt")
	stageEnvironmentPath := util.JoinPath(tmpEnvPath, testFixtureStack, "stage")

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+mgmtEnvironmentPath)
	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+stageEnvironmentPath)

	helpers.RunTerragrunt(t, "terragrunt run --all output --non-interactive --working-dir "+mgmtEnvironmentPath)
	helpers.RunTerragrunt(t, "terragrunt run --all output --non-interactive --working-dir "+stageEnvironmentPath)

	helpers.RunTerragrunt(t, "terragrunt run --all destroy --non-interactive --working-dir "+stageEnvironmentPath)
	helpers.RunTerragrunt(t, "terragrunt run --all destroy --non-interactive --working-dir "+mgmtEnvironmentPath)
}

func TestAwsRemoteWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-lock-table-" + strings.ToLower(helpers.UniqueID())

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRemoteWithBackend)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRemoteWithBackend)

	rootTerragruntConfigPath := util.JoinPath(rootPath, "terragrunt.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --backend-bootstrap --non-interactive --working-dir "+rootPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)
}

func TestAwsLocalWithBackend(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-lock-table-" + strings.ToLower(helpers.UniqueID())

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/download")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLocalWithBackend)

	rootTerragruntConfigPath := util.JoinPath(rootPath, "terragrunt.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, "not-used")

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --backend-bootstrap --working-dir "+rootPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --backend-bootstrap --working-dir "+rootPath)
}

func TestAwsGetAccountAliasFunctions(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureAwsAccountAlias)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAwsAccountAlias)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAwsAccountAlias)

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &stdout, &stderr),
	)

	// Get values from STS
	awsCfg, err := awshelper.CreateAwsConfig(t.Context(), createLogger(), nil, nil)
	if err != nil {
		t.Fatalf("Error while creating AWS config: %v", err)
	}

	iamClient := iam.NewFromConfig(awsCfg)
	aliases, err := iamClient.ListAccountAliases(t.Context(), &iam.ListAccountAliasesInput{})
	if err != nil {
		t.Fatalf("Error while getting AWS account aliases: %v", err)
	}

	alias := ""
	if len(aliases.AccountAliases) == 1 {
		alias = aliases.AccountAliases[0]
	}

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, outputs["account_alias"].Value, alias)
}

func TestAwsGetCallerIdentityFunctions(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureAwsGetCallerIdentity)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAwsGetCallerIdentity)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAwsGetCallerIdentity)

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt run --non-interactive --working-dir "+rootPath+" -- output -no-color -json", &stdout, &stderr),
	)

	// Get values from STS
	awsCfg, err := awshelper.CreateAwsConfig(t.Context(), createLogger(), nil, nil)
	if err != nil {
		t.Fatalf("Error while creating AWS config: %v", err)
	}

	stsClient := sts.NewFromConfig(awsCfg)
	identity, err := stsClient.GetCallerIdentity(t.Context(), &sts.GetCallerIdentityInput{})
	if err != nil {
		t.Fatalf("Error while getting AWS caller identity: %v", err)
	}

	outputs := map[string]helpers.TerraformOutput{}
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

	expectedOutput := `They said, "No, The answer is 42"`
	generatedUniqueID := helpers.UniqueID()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "nested-optimization")
	rootTerragruntConfigPath := filepath.Join(rootPath, "root.hcl")
	livePath := filepath.Join(rootPath, "live")
	deepDepPath := filepath.Join(rootPath, "deepdep")
	depPath := filepath.Join(rootPath, "dep")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(generatedUniqueID)
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(generatedUniqueID)
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, helpers.TerraformRemoteStateS3Region)

	helpers.RunTerragrunt(t, "terragrunt apply --all --log-level trace --non-interactive --backend-bootstrap --working-dir "+rootPath)

	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// verify expected output
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --log-level trace --non-interactive --working-dir "+livePath)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))
	assert.Equal(t, expectedOutput, outputs["output"].Value)

	// If we want to force reinit, delete the relevant .terraform directories
	helpers.CleanupTerraformFolder(t, depPath)

	// Now delete the deepdep state and verify still works (note we need to bust the cache again)
	config.ClearOutputCache()
	require.NoError(t, os.Remove(filepath.Join(deepDepPath, "terraform.tfstate")))

	fmt.Println("terragrunt output -no-color -json --log-level trace --non-interactive --working-dir " + livePath)

	reout, reerr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --log-level trace --non-interactive --working-dir "+livePath)
	require.NoError(t, err)

	require.NoError(t, json.Unmarshal([]byte(reout), &outputs))
	assert.Equal(t, expectedOutput, outputs["output"].Value)

	for _, logRegexp := range []string{`prefix=../dep .+Running command: ` + wrappedBinary() + ` init -get=false`} {
		assert.Regexp(t, logRegexp, reerr)
	}
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
	generatedUniqueID := helpers.UniqueID()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "nested-optimization-disable")
	rootTerragruntConfigPath := filepath.Join(rootPath, "root.hcl")
	livePath := filepath.Join(rootPath, "live")
	deepDepPath := filepath.Join(rootPath, "deepdep")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(generatedUniqueID)
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(generatedUniqueID)
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, helpers.TerraformRemoteStateS3Region)

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --backend-bootstrap --working-dir "+rootPath)

	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// verify expected output
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --non-interactive --working-dir "+livePath)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))
	assert.Equal(t, expectedOutput, outputs["output"].Value)

	// Now delete the deepdep state and verify it no longer works, because it tries to fetch the deepdep dependency
	config.ClearOutputCache()
	require.NoError(t, os.Remove(filepath.Join(deepDepPath, "terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(deepDepPath, ".terraform")))
	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --non-interactive --working-dir "+livePath)
	require.Error(t, err)
}

func TestAwsProviderPatch(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixtureAwsProviderPatch)
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

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			`terragrunt aws-provider-patch --override-attr 'region="eu-west-1"' --override-attr allowed_account_ids='["00000000000"]' --working-dir %s --log-level trace`,
			modulePath,
		),
	)
	require.NoError(t, err)

	assert.Regexp(t, "Patching AWS provider in .+test/fixtures/aws-provider-patch/example-module/main.tf", stderr)

	// Make sure the resulting terraform code is still valid
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt validate --working-dir "+modulePath, os.Stdout, os.Stderr),
	)
}

func TestAwsPrintAwsErrors(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3Errors)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureS3Errors)
	helpers.CleanupTerraformFolder(t, rootPath)

	s3BucketName := "test-tg-2023-02"
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	tmpTerragruntConfigFile := util.JoinPath(rootPath, "terragrunt.hcl")
	originalTerragruntConfigPath := util.JoinPath(rootPath, "terragrunt.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "us-east-2")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt apply --non-interactive --backend-bootstrap --config %s --working-dir %s", tmpTerragruntConfigFile, rootPath), &stdout, &stderr)
	require.Error(t, err)
	message := err.Error()
	assert.True(t,
		strings.Contains(
			message,
			"AllAccessDisabled: All access to this object has been disabled",
		) ||
			strings.Contains(message, "BucketRegionError: incorrect region") ||
			strings.Contains(message, "MovedPermanently"),
	)
}

func TestAwsErrorWhenStateBucketIsInDifferentRegion(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3Errors)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureS3Errors)
	helpers.CleanupTerraformFolder(t, rootPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	originalTerragruntConfigPath := util.JoinPath(testFixtureS3Errors, "terragrunt.hcl")
	tmpTerragruntConfigFile := util.JoinPath(rootPath, "terragrunt.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "us-east-1")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt apply --non-interactive --backend-bootstrap --config %s --working-dir %s", tmpTerragruntConfigFile, rootPath), &stdout, &stderr)
	require.NoError(t, err)

	helpers.CopyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "us-west-2")

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	err = helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt apply --non-interactive --backend-bootstrap --config %s --working-dir %s", tmpTerragruntConfigFile, rootPath), &stdout, &stderr)
	require.Error(t, err)

	assert.True(t, strings.Contains(
		err.Error(), "MovedPermanently") || strings.Contains(err.Error(), "BucketRegionError: incorrect region"),
		"Expected error to contain 'MovedPermanently' or 'BucketRegionError: incorrect region', but got: %s", err.Error(),
	)
}

func TestAwsDisableBucketUpdate(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)
	helpers.CleanupTerraformFolder(t, rootPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	createS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	createDynamoDBTable(t, helpers.TerraformRemoteStateS3Region, lockTableName)

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(t, rootPath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --disable-bucket-update --non-interactive --config %s --working-dir %s", tmpTerragruntConfigPath, rootPath))

	_, err := bucketPolicy(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	// validate that bucket policy is not updated, because of --disable-bucket-update
	require.Error(t, err)
}

func TestAwsUpdatePolicy(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)
	helpers.CleanupTerraformFolder(t, rootPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	createS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(t, rootPath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	// check that there is no policy on created bucket
	_, err := bucketPolicy(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	require.Error(t, err)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --backend-bootstrap --non-interactive --config %s --working-dir %s", tmpTerragruntConfigPath, rootPath))

	// check that policy is created
	_, err = bucketPolicy(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	require.NoError(t, err)
}

func TestAwsAssumeRoleDuration(t *testing.T) {
	t.Parallel()
	if isTerraform() {
		t.Skip("New assume role duration config not supported by Terraform 1.5.x")
		return
	}

	assumeRole := os.Getenv("AWS_TEST_S3_ASSUME_ROLE")
	if len(assumeRole) == 0 {
		t.Error("AWS_TEST_S3_ASSUME_ROLE environment variable not set")
		return
	}

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAssumeRoleDuration)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAssumeRoleDuration)

	originalTerragruntConfigPath := util.JoinPath(testFixtureAssumeRoleDuration, "terragrunt.hcl")
	tmpTerragruntConfigFile := util.JoinPath(testPath, "terragrunt.hcl")
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	helpers.CopyAndFillMapPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, map[string]string{
		"__FILL_IN_BUCKET_NAME__":      s3BucketName,
		"__FILL_IN_REGION__":           helpers.TerraformRemoteStateS3Region,
		"__FILL_IN_LOGS_BUCKET_NAME__": s3BucketName + "-tf-state-logs",
		"__FILL_IN_ASSUME_ROLE__":      assumeRole,
	})

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --backend-bootstrap --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	output := fmt.Sprintf("%s %s", stderr.String(), stdout.String())
	assert.Contains(t, output, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")
	// run one more time to check that no init is performed
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --backend-bootstrap --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	output = fmt.Sprintf("%s %s", stderr.String(), stdout.String())
	assert.NotContains(t, output, "Initializing the backend...")
	assert.NotContains(t, output, "has been successfully initialized!")
	assert.Contains(t, output, "no changes are needed.")
}

// Regression testing for https://github.com/gruntwork-io/terragrunt/issues/906
func TestAwsDependencyOutputSameOutputConcurrencyRegression(t *testing.T) {
	t.Parallel()

	// Use func to isolate each test run to a single s3 bucket that is deleted. We run the test multiple times
	// because the underlying error we are trying to test against is nondeterministic, and thus may not always work
	// the first time.
	tt := func() {
		helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
		rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "regression-906")

		// Make sure to fill in the s3 bucket to the config. Also ensure the bucket is deleted before the next for
		// loop call.
		s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s%s", strings.ToLower(helpers.UniqueID()), strings.ToLower(helpers.UniqueID()))
		defer helpers.DeleteS3BucketWithRetry(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
		commonDepConfigPath := util.JoinPath(rootPath, "common-dep", "terragrunt.hcl")
		helpers.CopyTerragruntConfigAndFillPlaceholders(t, commonDepConfigPath, commonDepConfigPath, s3BucketName, "not-used", "not-used")

		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}
		err := helpers.RunTerragruntCommand(
			t,
			"terragrunt run --all apply --source-update --non-interactive --working-dir "+rootPath,
			&stdout,
			&stderr,
		)
		helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
		helpers.LogBufferContentsLineByLine(t, stderr, "stderr")
		require.NoError(t, err)
	}

	for range 3 {
		tt()
		// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
		// This is only a problem during testing, where the process is shared across terragrunt runs.
		config.ClearOutputCache()
	}
}

func TestAwsRemoteStateCodegenGeneratesBackendBlockS3(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "remote-state", "s3")

	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(t, generateTestCase, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --backend-bootstrap --config %s --working-dir %s", tmpTerragruntConfigPath, generateTestCase))
}

func TestAwsOutputFromRemoteState(t *testing.T) { //nolint: paralleltest
	// NOTE: We can't run this test in parallel because there are other tests that also call `config.ClearOutputCache()`, but this function uses a global variable and sometimes it throws an unexpected error:
	// "fixtures/output-from-remote-state/env1/app2/terragrunt.hcl:23,38-48: Unsupported attribute; This object does not have an attribute named "app3_text"."
	// t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputFromRemoteState)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputFromRemoteState, "root.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputFromRemoteState)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply --backend-bootstrap --dependency-fetch-output-from-state --auto-approve --non-interactive --working-dir %s/app1", environmentPath))
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply --backend-bootstrap --dependency-fetch-output-from-state --auto-approve --non-interactive --working-dir %s/app3", environmentPath))
	// Now delete dependencies cached state
	config.ClearOutputCache()
	require.NoError(t, os.Remove(filepath.Join(environmentPath, "/app1/.terraform/terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(environmentPath, "/app1/.terraform")))
	require.NoError(t, os.Remove(filepath.Join(environmentPath, "/app3/.terraform/terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(environmentPath, "/app3/.terraform")))

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply --backend-bootstrap --dependency-fetch-output-from-state --auto-approve --non-interactive --working-dir %s/app2", environmentPath))
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	helpers.RunTerragruntRedirectOutput(t, "terragrunt run --all output --backend-bootstrap --dependency-fetch-output-from-state --non-interactive --log-level trace --working-dir "+environmentPath, &stdout, &stderr)
	output := stdout.String()

	assert.Contains(t, output, "app1 output")
	assert.Contains(t, output, "app2 output")
	assert.Contains(t, output, "app3 output")
	assert.NotContains(t, stderr.String(), "terraform output -json")

	assert.True(t, (strings.Index(output, "app3 output") < strings.Index(output, "app1 output")) && (strings.Index(output, "app1 output") < strings.Index(output, "app2 output")))
}

func TestAwsMockOutputsFromRemoteState(t *testing.T) { //nolint: paralleltest
	// NOTE: We can't run this test in parallel because there are other tests that also call `config.ClearOutputCache()`, but this function uses a global variable and sometimes it throws an unexpected error:
	// "fixtures/output-from-remote-state/env1/app2/terragrunt.hcl:23,38-48: Unsupported attribute; This object does not have an attribute named "app3_text"."
	// t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputFromRemoteState)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputFromRemoteState, "root.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := filepath.Join(tmpEnvPath, testFixtureOutputFromRemoteState, "env1")

	// applying only the app1 dependency, the app3 dependency was purposely not applied and should be mocked when running the app2 module
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply --dependency-fetch-output-from-state --auto-approve --backend-bootstrap --non-interactive --working-dir %s/app1", environmentPath))
	// Now delete dependencies cached state
	config.ClearOutputCache()
	require.NoError(t, os.Remove(filepath.Join(environmentPath, "/app1/.terraform/terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(environmentPath, "/app1/.terraform")))

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt init --dependency-fetch-output-from-state --non-interactive --working-dir %s/app2", environmentPath))
	require.NoError(t, err)

	assert.Contains(t, stderr, "Failed to read outputs")
	assert.Contains(t, stderr, "fallback to mock outputs")
}

func TestAwsParallelStateInit(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()
	for i := range 20 {
		err := util.CopyFolderContents(logger.CreateLogger(), testFixtureParallelStateInit, tmpEnvPath, ".terragrunt-test", nil, nil)
		require.NoError(t, err)
		err = os.Rename(
			path.Join(tmpEnvPath, "template"),
			path.Join(tmpEnvPath, "app"+strconv.Itoa(i)))
		require.NoError(t, err)
	}

	originalTerragruntConfigPath := util.JoinPath(testFixtureParallelStateInit, "root.hcl")
	tmpTerragruntConfigFile := util.JoinPath(tmpEnvPath, "root.hcl")
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "us-east-2")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+tmpEnvPath+" -- apply -auto-approve")
}

func TestAwsAssumeRole(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAssumeRole)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAssumeRole)

	originalTerragruntConfigPath := util.JoinPath(testFixtureAssumeRole, "terragrunt.hcl")
	tmpTerragruntConfigFile := util.JoinPath(testPath, "terragrunt.hcl")
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "us-east-2")

	helpers.RunTerragrunt(t, "terragrunt hcl validate --inputs -auto-approve --non-interactive --working-dir "+testPath)

	// validate generated backend.tf
	backendFile := filepath.Join(testPath, "backend.tf")
	assert.FileExists(t, backendFile)

	content, err := files.ReadFileAsString(backendFile)
	require.NoError(t, err)

	opts, err := options.NewTerragruntOptionsForTest(testPath)
	require.NoError(t, err)

	l := logger.CreateLogger()

	cfg, err := awshelper.CreateAwsConfig(t.Context(), l, nil, opts)
	require.NoError(t, err)

	identityARN, err := awshelper.GetAWSIdentityArn(t.Context(), cfg)
	require.NoError(t, err)

	assert.Contains(t, content, "role_arn     = \""+identityARN+"\"")
	assert.Contains(t, content, "external_id  = \"external_id_123\"")
	assert.Contains(t, content, "session_name = \"session_name_example\"")
}

func TestAwsAssumeRoleWithExternalIDWithComma(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAssumeRoleWithExternalIDWithComma)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAssumeRoleWithExternalIDWithComma)

	originalTerragruntConfigPath := util.JoinPath(testFixtureAssumeRoleWithExternalIDWithComma, "terragrunt.hcl")
	tmpTerragruntConfigFile := util.JoinPath(testPath, "terragrunt.hcl")
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "us-east-2")

	helpers.RunTerragrunt(t, "terragrunt hcl validate --inputs -auto-approve --non-interactive --working-dir "+testPath)

	// validate generated backend.tf
	backendFile := filepath.Join(testPath, "backend.tf")
	assert.FileExists(t, backendFile)

	content, err := files.ReadFileAsString(backendFile)
	require.NoError(t, err)

	opts, err := options.NewTerragruntOptionsForTest(testPath)
	require.NoError(t, err)

	l := logger.CreateLogger()

	cfg, err := awshelper.CreateAwsConfig(t.Context(), l, nil, opts)
	require.NoError(t, err)

	identityARN, err := awshelper.GetAWSIdentityArn(t.Context(), cfg)
	require.NoError(t, err)

	assert.Contains(t, content, "role_arn     = \""+identityARN+"\"")
	assert.Contains(t, content, "external_id  = \"external_id_123,external_id_456\"")
	assert.Contains(t, content, "session_name = \"session_name_example\"")
}

func TestAwsInitConfirmation(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, "root.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt run --backend-bootstrap --all init --working-dir "+tmpEnvPath, &stdout, &stderr)
	require.NoError(t, err)
	errout := stderr.String()
	assert.Equal(t, 1, strings.Count(errout, "does not exist or you don't have permissions to access it. Would you like Terragrunt to create it? (y/n)"))
}

func TestAwsRunAllCommandPrompt(t *testing.T) {
	t.Parallel()

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputAll)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureOutputAll, "root.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputAll)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --working-dir "+environmentPath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "stderr")
	assert.Contains(t, stderr.String(), "Are you sure you want to run 'terragrunt apply' in each unit of the run queue displayed above? (y/n)")
	require.Error(t, err)
}

func TestAwsReadTerragruntAuthProviderCmd(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "multiple-apps")
	appPath := util.JoinPath(rootPath, "app1")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	helpers.RunTerragrunt(t, fmt.Sprintf(`terragrunt run --all --non-interactive --working-dir %s --auth-provider-cmd %s`, rootPath, mockAuthCmd)+" -- apply -auto-approve")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt output -json --working-dir %s --auth-provider-cmd %s", appPath, mockAuthCmd))
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "app1-bar", outputs["foo-app1"].Value)
	assert.Equal(t, "app2-bar", outputs["foo-app2"].Value)
	assert.Equal(t, "app3-bar", outputs["foo-app3"].Value)
}

func TestAwsReadTerragruntAuthProviderCmdWithSops(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	sopsPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "sops")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	helpers.RunTerragrunt(t, fmt.Sprintf(`terragrunt apply -auto-approve --non-interactive --working-dir %s --auth-provider-cmd %s`, sopsPath, mockAuthCmd))

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt output -json --working-dir %s --auth-provider-cmd %s", sopsPath, mockAuthCmd))
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "Welcome to SOPS! Edit this file as you please!", outputs["hello"].Value)
}

func TestAwsReadTerragruntConfigIamRole(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	cfg, err := awshelper.CreateAwsConfig(t.Context(), l, nil, &options.TerragruntOptions{})
	require.NoError(t, err)

	identityArn, err := awshelper.GetAWSIdentityArn(t.Context(), cfg)
	require.NoError(t, err)

	helpers.CleanupTerraformFolder(t, testFixtureReadIamRole)

	// Execution outputs to be verified
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	// Invoke terragrunt and verify used IAM role
	err = helpers.RunTerragruntCommand(t, "terragrunt init --working-dir "+testFixtureReadIamRole, &stdout, &stderr)

	// Since are used not existing AWS accounts, for validation are used success and error outputs
	output := fmt.Sprintf("%v %v %v", stderr.String(), stdout.String(), err.Error())

	// Check that output contains value defined in IAM role
	assert.Contains(t, output, "666666666666")
	// Ensure that state file wasn't created with default IAM value
	assert.True(t, util.FileNotExists(util.JoinPath(testFixtureReadIamRole, identityArn+".txt")))
}

func TestTerragruntWorksWithIncludeShallowMerge(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(includeFixturePath, includeShallowFixturePath)
	helpers.CleanupTerraformFolder(t, childPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfigWithParentAndChild(t, includeFixturePath, includeShallowFixturePath, s3BucketName, "root.hcl", config.DefaultTerragruntConfigPath)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --log-level trace --config %s --working-dir %s", tmpTerragruntConfigPath, childPath))
	validateIncludeRemoteStateReflection(t, s3BucketName, includeShallowFixturePath, tmpTerragruntConfigPath, childPath)
}

func TestTerragruntWorksWithIncludeNoMerge(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(includeFixturePath, includeNoMergeFixturePath)
	helpers.CleanupTerraformFolder(t, childPath)

	// We deliberately pick an s3 bucket name that is invalid, as we don't expect to create this s3 bucket.
	s3BucketName := "__INVALID_NAME__"

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfigWithParentAndChild(t, includeFixturePath, includeNoMergeFixturePath, s3BucketName, "root.hcl", config.DefaultTerragruntConfigPath)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --log-level trace --config %s --working-dir %s", tmpTerragruntConfigPath, childPath))
	validateIncludeRemoteStateReflection(t, s3BucketName, includeNoMergeFixturePath, tmpTerragruntConfigPath, childPath)
}

func TestErrorExplaining(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInitError)
	initTestCase := util.JoinPath(tmpEnvPath, testFixtureInitError)

	helpers.CleanupTerraformFolder(t, initTestCase)
	helpers.CleanupTerragruntFolder(t, initTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt init -no-color --tf-forward-stdout --non-interactive --working-dir "+initTestCase, &stdout, &stderr)
	require.Error(t, err)

	explanation := shell.ExplainError(err)
	assert.Contains(t, explanation, "Check your credentials and permissions")
}

func TestTerragruntInvokeTerraformTests(t *testing.T) {
	t.Parallel()
	if isTerraform() {
		t.Skip("Not compatible with Terraform 1.5.x")
		return
	}

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTfTest)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureTfTest)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt test --non-interactive --tf-forward-stdout --working-dir "+testPath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "1 passed, 0 failed")
}

func dependencyOutputOptimizationTest(t *testing.T, moduleName string, forceInit bool, expectedOutputLogs []string) {
	t.Helper()

	expectedOutput := `They said, "No, The answer is 42"`
	generatedUniqueID := helpers.UniqueID()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, moduleName)
	rootTerragruntConfigPath := filepath.Join(rootPath, "root.hcl")
	livePath := filepath.Join(rootPath, "live")
	deepDepPath := filepath.Join(rootPath, "deepdep")
	depPath := filepath.Join(rootPath, "dep")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(generatedUniqueID)
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(generatedUniqueID)
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName, helpers.TerraformRemoteStateS3Region)

	helpers.RunTerragrunt(t, "terragrunt run --all apply --log-level trace --non-interactive --backend-bootstrap --working-dir "+rootPath)

	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// verify expected output
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --log-level trace --non-interactive --working-dir "+livePath)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))
	assert.Equal(t, expectedOutput, outputs["output"].Value)

	// If we want to force reinit, delete the relevant .terraform directories
	if forceInit {
		helpers.CleanupTerraformFolder(t, depPath)
	}

	// Now delete the deepdep state and verify still works (note we need to bust the cache again)
	config.ClearOutputCache()
	require.NoError(t, os.Remove(filepath.Join(deepDepPath, "terraform.tfstate")))

	fmt.Println("terragrunt output -no-color -json --log-level trace --non-interactive --working-dir " + livePath)

	reout, reerr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --log-level trace --non-interactive --working-dir "+livePath)
	require.NoError(t, err)

	require.NoError(t, json.Unmarshal([]byte(reout), &outputs))
	assert.Equal(t, expectedOutput, outputs["output"].Value)

	for _, logRegexp := range expectedOutputLogs {
		assert.Regexp(t, logRegexp, reerr)
	}
}

func assertS3Tags(t *testing.T, expectedTags map[string]string, bucketName string, client *s3.Client) {
	t.Helper()

	ctx := t.Context()
	var in = s3.GetBucketTaggingInput{}
	in.Bucket = aws.String(bucketName)

	var tags, err2 = client.GetBucketTagging(ctx, &in)

	if err2 != nil {
		t.Fatal(err2)
	}

	var actualTags = make(map[string]string)

	for _, element := range tags.TagSet {
		actualTags[*element.Key] = *element.Value
	}

	assert.Equal(t, expectedTags, actualTags, "Did not find expected tags on s3 bucket.")
}

func assertS3BucketVersioning(t *testing.T, bucketName string, versioning bool, client *s3.Client) {
	t.Helper()

	ctx := t.Context()
	res, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucketName)})
	require.NoError(t, err)
	require.NotNil(t, res)

	if versioning {
		require.NotNil(t, res.Status)
		assert.Equal(t, s3types.BucketVersioningStatusEnabled, res.Status, "Versioning is not enabled for the remote state S3 bucket %s", bucketName)
	} else {
		require.Empty(t, res.Status, "Versioning should be disabled for the remote state S3 bucket %s", bucketName)
	}
}

// Check that the DynamoDB table of the given name and region exists. Terragrunt should create this table during the test.
// Also check if table got tagged properly
func validateDynamoDBTableExistsAndIsTaggedAndIsSSEncrypted(t *testing.T, awsRegion string, tableName string, expectedTags map[string]string, expectedSSEncrypted bool) {
	t.Helper()

	client := helpers.CreateDynamoDBClientForTest(t, awsRegion, "", "")

	ctx := t.Context()
	description, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)})
	require.NoError(t, err, "DynamoDB table %s does not exist", tableName)

	if expectedSSEncrypted {
		require.NotNil(t, description.Table.SSEDescription)
		assert.Equal(t, types.SSEStatusEnabled, description.Table.SSEDescription.Status)
	} else {
		require.Nil(t, description.Table.SSEDescription)
	}

	tags, err := client.ListTagsOfResource(ctx, &dynamodb.ListTagsOfResourceInput{ResourceArn: description.Table.TableArn})
	require.NoError(t, err)

	if expectedTags == nil {
		return
	}

	var actualTags = make(map[string]string)

	for _, element := range tags.Tags {
		actualTags[*element.Key] = *element.Value
	}

	assert.Equal(t, expectedTags, actualTags, "Did not find expected tags on dynamo table.")
}

func doesDynamoDBTableItemExist(t *testing.T, awsRegion string, tableName, key string) bool {
	t.Helper()

	client := helpers.CreateDynamoDBClientForTest(t, awsRegion, "", "")

	ctx := t.Context()
	_, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)})
	require.NoError(t, err, "DynamoDB table %s does not exist", tableName)

	input := &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"LockID": &types.AttributeValueMemberS{
				Value: key,
			},
		},
	}

	res, err := client.GetItem(ctx, input)
	require.NoError(t, err)

	exists := len(res.Item) != 0

	return exists

}

// Check that the S3 Bucket of the given name and region exists. Terragrunt should create this bucket during the test.
// Also check if bucket got tagged properly and that public access is disabled completely.
func validateS3BucketExistsAndIsTaggedAndVersioning(t *testing.T, awsRegion string, bucketName string, versioning bool, expectedTags map[string]string) {
	t.Helper()

	client := helpers.CreateS3ClientForTest(t, awsRegion)

	ctx := t.Context()
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucketName)})
	require.NoError(t, err, "S3 bucket %s does not exist", bucketName)

	if expectedTags != nil {
		assertS3Tags(t, expectedTags, bucketName, client)
	}

	assertS3BucketVersioning(t, bucketName, versioning, client)

	assertS3PublicAccessBlocks(t, client, bucketName)
}

func doesS3BucketKeyExist(t *testing.T, awsRegion string, bucketName, key string) bool {
	t.Helper()

	client := helpers.CreateS3ClientForTest(t, awsRegion)

	ctx := t.Context()
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucketName)})
	require.NoError(t, err, "S3 bucket %s does not exist", bucketName)

	_, err = client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			if apiErr.ErrorCode() == "NotFound" {
				return false
			}
		}
		require.NoError(t, err)
	}

	return true
}

func assertS3PublicAccessBlocks(t *testing.T, client *s3.Client, bucketName string) {
	t.Helper()

	ctx := t.Context()
	resp, err := client.GetPublicAccessBlock(
		ctx, &s3.GetPublicAccessBlockInput{Bucket: aws.String(bucketName)},
	)
	require.NoError(t, err)

	publicAccessBlockConfig := resp.PublicAccessBlockConfiguration
	assert.True(t, aws.ToBool(publicAccessBlockConfig.BlockPublicAcls))
	assert.True(t, aws.ToBool(publicAccessBlockConfig.BlockPublicPolicy))
	assert.True(t, aws.ToBool(publicAccessBlockConfig.IgnorePublicAcls))
	assert.True(t, aws.ToBool(publicAccessBlockConfig.RestrictPublicBuckets))
}

func bucketEncryption(t *testing.T, awsRegion string, bucketName string) (*s3.GetBucketEncryptionOutput, error) {
	t.Helper()

	client := helpers.CreateS3ClientForTest(t, awsRegion)

	ctx := t.Context()
	input := &s3.GetBucketEncryptionInput{Bucket: aws.String(bucketName)}
	output, err := client.GetBucketEncryption(ctx, input)
	if err != nil {
		// TODO: Remove this lint suppression
		return nil, nil //nolint:nilerr
	}

	return output, nil
}

// createS3Bucket create test S3 bucket.
func createS3Bucket(t *testing.T, awsRegion, bucketName string) {
	t.Helper()

	client := helpers.CreateS3ClientForTest(t, awsRegion)

	t.Logf("Creating test s3 bucket %s", bucketName)

	ctx := t.Context()

	input := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
		CreateBucketConfiguration: &s3types.CreateBucketConfiguration{
			LocationConstraint: s3types.BucketLocationConstraint(awsRegion),
		},
	}

	_, err := client.CreateBucket(ctx, input)
	require.NoError(t, err, "Failed to create S3 bucket")
}

func deleteS3Bucket(t *testing.T, awsRegion, bucketName string) {
	t.Helper()

	helpers.DeleteS3Bucket(t, awsRegion, bucketName)
}

func cleanupTableForTest(t *testing.T, tableName string, awsRegion string) {
	t.Helper()

	client := helpers.CreateDynamoDBClientForTest(t, awsRegion, "", "")

	t.Logf("Deleting test DynamoDB table %s", tableName)

	ctx := t.Context()
	_, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "ResourceNotFoundException" {
			t.Logf("DynamoDB table %s does not exist", tableName)
			return
		}

		t.Errorf("Failed to describe DynamoDB table %s: %v", tableName, err)
		return
	}

	if _, err := client.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(tableName)}); err != nil {
		t.Errorf("Failed to delete DynamoDB table %s: %v", tableName, err)
	}
}

func bucketPolicy(t *testing.T, awsRegion string, bucketName string) (*s3.GetBucketPolicyOutput, error) {
	t.Helper()

	client := helpers.CreateS3ClientForTest(t, awsRegion)

	ctx := t.Context()
	policyOutput, err := client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return nil, err
	}
	return policyOutput, nil
}

// createDynamoDBTableE creates a test DynamoDB table, and returns an error if the table creation fails.
func createDynamoDBTableE(t *testing.T, awsRegion string, tableName string) error {
	t.Helper()

	client := helpers.CreateDynamoDBClientForTest(t, awsRegion, "", "")
	ctx := t.Context()
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("LockID"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("LockID"),
				KeyType:       types.KeyTypeHash,
			},
		},
		TableName: aws.String(tableName),
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
	})
	if err != nil {
		return err
	}

	// Wait for table to be created
	waiter := dynamodb.NewTableExistsWaiter(client)
	err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)}, 5*time.Minute)
	return err
}

// createDynamoDBTable creates a test DynamoDB table.
func createDynamoDBTable(t *testing.T, awsRegion string, tableName string) {
	t.Helper()

	err := createDynamoDBTableE(t, awsRegion, tableName)
	require.NoError(t, err)
}

func validateIncludeRemoteStateReflection(t *testing.T, s3BucketName string, keyPath string, configPath string, workingDir string) {
	t.Helper()

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --non-interactive --log-level trace --config %s --working-dir %s", configPath, workingDir), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	remoteStateOut := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(outputs["reflect"].Value.(string)), &remoteStateOut))
	assert.Equal(
		t,
		map[string]any{
			"backend":                         "s3",
			"disable_init":                    false,
			"disable_dependency_optimization": false,
			"generate":                        nil,
			"config": map[string]any{
				"encrypt": true,
				"bucket":  s3BucketName,
				"key":     keyPath + "/terraform.tfstate",
				"region":  "us-west-2",
			},
			"encryption": nil,
		},
		remoteStateOut,
	)
}
