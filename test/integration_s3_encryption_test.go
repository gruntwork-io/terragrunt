//go:build aws

package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	terraws "github.com/gruntwork-io/terratest/modules/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/config"
)

const (
	s3SSEAESFixturePath            = "fixtures/s3-encryption/sse-aes"
	s3SSECustomKeyFixturePath      = "fixtures/s3-encryption/custom-key"
	s3SSBasicEncryptionFixturePath = "fixtures/s3-encryption/basic-encryption"
	s3SSEKMSFixturePath            = "fixtures/s3-encryption/sse-kms"
)

func TestAwsS3SSEAES(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, s3SSEAESFixturePath)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, s3SSEAESFixturePath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(t, s3SSEAESFixturePath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	helpers.RunTerragrunt(t, applyCommand(tmpTerragruntConfigPath, testPath))

	client := terraws.NewS3Client(t, helpers.TerraformRemoteStateS3Region)
	resp, err := client.GetBucketEncryption(&s3.GetBucketEncryptionInput{Bucket: aws.String(s3BucketName)})
	require.NoError(t, err)
	require.Len(t, resp.ServerSideEncryptionConfiguration.Rules, 1)
	sseRule := resp.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault
	require.NotNil(t, sseRule)
	assert.Equal(t, s3.ServerSideEncryptionAes256, aws.StringValue(sseRule.SSEAlgorithm))
	assert.Nil(t, sseRule.KMSMasterKeyID)
}

func TestAwsS3SSECustomKey(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, s3SSECustomKeyFixturePath)
	testPath := util.JoinPath(tmpEnvPath, s3SSECustomKeyFixturePath)
	helpers.CleanupTerraformFolder(t, testPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(t, s3SSECustomKeyFixturePath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)
	helpers.RunTerragrunt(t, applyCommand(tmpTerragruntConfigPath, testPath))

	client := terraws.NewS3Client(t, helpers.TerraformRemoteStateS3Region)
	resp, err := client.GetBucketEncryption(&s3.GetBucketEncryptionInput{Bucket: aws.String(s3BucketName)})
	require.NoError(t, err)
	require.Len(t, resp.ServerSideEncryptionConfiguration.Rules, 1)
	sseRule := resp.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault
	require.NotNil(t, sseRule)
	assert.Equal(t, s3.ServerSideEncryptionAwsKms, aws.StringValue(sseRule.SSEAlgorithm))
	assert.True(t, strings.HasSuffix(aws.StringValue(sseRule.KMSMasterKeyID), "alias/dedicated-test-key"))

	// Replace the custom key with a new one, and check that the key is updated in s3
	helpers.CleanupTerraformFolder(t, testPath)

	contents, err := util.ReadFileAsString(tmpTerragruntConfigPath)
	require.NoError(t, err)

	err = os.Remove(tmpTerragruntConfigPath)
	require.NoError(t, err)

	contents = strings.ReplaceAll(contents, "dedicated-test-key", "other-dedicated-test-key")

	err = os.WriteFile(tmpTerragruntConfigPath, []byte(contents), 0444)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, applyCommand(tmpTerragruntConfigPath, testPath))

	resp, err = client.GetBucketEncryption(&s3.GetBucketEncryptionInput{Bucket: aws.String(s3BucketName)})
	require.NoError(t, err)
	require.Len(t, resp.ServerSideEncryptionConfiguration.Rules, 1)
	sseRule = resp.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault
	require.NotNil(t, sseRule)
	assert.Equal(t, s3.ServerSideEncryptionAwsKms, aws.StringValue(sseRule.SSEAlgorithm))

	// This check is asserting that the following bug still isn't fixed:
	// https://github.com/gruntwork-io/terragrunt/issues/3364
	//
	// There were unanticipated consequences to addressing it that should be resolved before the fix is implemented:
	// https://github.com/gruntwork-io/terragrunt/issues/3384
	//
	// At the very least, it should be documented as a breaking change.
	assert.False(t, strings.HasSuffix(aws.StringValue(sseRule.KMSMasterKeyID), "alias/other-dedicated-test-key"))
}

func TestAwsS3SSEKeyNotReverted(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, s3SSBasicEncryptionFixturePath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(t, s3SSBasicEncryptionFixturePath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+filepath.Dir(tmpTerragruntConfigPath))
	require.NoError(t, err)
	output := fmt.Sprintf(stdout, stderr)

	// verify that bucket encryption message is not printed
	assert.NotContains(t, output, "Bucket Server-Side Encryption")

	tmpTerragruntConfigPath = helpers.CreateTmpTerragruntConfig(t, s3SSBasicEncryptionFixturePath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)
	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+filepath.Dir(tmpTerragruntConfigPath))
	require.NoError(t, err)
	output = fmt.Sprintf(stdout, stderr)
	assert.NotContains(t, output, "Bucket Server-Side Encryption")

	// verify that encryption key is not reverted
	client := terraws.NewS3Client(t, helpers.TerraformRemoteStateS3Region)
	resp, err := client.GetBucketEncryption(&s3.GetBucketEncryptionInput{Bucket: aws.String(s3BucketName)})
	require.NoError(t, err)
	require.Len(t, resp.ServerSideEncryptionConfiguration.Rules, 1)
	sseRule := resp.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault
	require.NotNil(t, sseRule)
	assert.Equal(t, s3.ServerSideEncryptionAwsKms, aws.StringValue(sseRule.SSEAlgorithm))
	assert.True(t, strings.HasSuffix(aws.StringValue(sseRule.KMSMasterKeyID), "alias/dedicated-test-key"))
}

func TestAwsS3EncryptionWarning(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, s3SSEKMSFixturePath)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, s3SSEKMSFixturePath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	require.NoError(t, createS3BucketE(t, helpers.TerraformRemoteStateS3Region, s3BucketName))

	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, helpers.TerraformRemoteStateS3Region)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(t, s3SSEKMSFixturePath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, applyCommand(tmpTerragruntConfigPath, testPath))
	require.NoError(t, err)
	output := fmt.Sprintf(stdout, stderr)
	// check that warning is printed
	assert.Contains(t, output, "Encryption is not enabled on the S3 remote state bucket "+s3BucketName)

	// verify that encryption configuration is set
	client := terraws.NewS3Client(t, helpers.TerraformRemoteStateS3Region)
	resp, err := client.GetBucketEncryption(&s3.GetBucketEncryptionInput{Bucket: aws.String(s3BucketName)})
	require.NoError(t, err)
	require.Len(t, resp.ServerSideEncryptionConfiguration.Rules, 1)
	sseRule := resp.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault
	require.NotNil(t, sseRule)
	assert.Equal(t, s3.ServerSideEncryptionAwsKms, aws.StringValue(sseRule.SSEAlgorithm))

	// check that second warning is not printed
	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(t, applyCommand(tmpTerragruntConfigPath, testPath))
	require.NoError(t, err)
	output = fmt.Sprintf(stdout, stderr)
	assert.NotContains(t, output, "Encryption is not enabled on the S3 remote state bucket "+s3BucketName)
}

func TestAwsSkipBackend(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, s3SSEAESFixturePath)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, s3SSEAESFixturePath)

	// The bucket and table name here are intentionally invalid.
	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(t, s3SSEAESFixturePath, "N/A", "N/A", config.DefaultTerragruntConfigPath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt init --terragrunt-non-interactive --terragrunt-config "+tmpTerragruntConfigPath+" --terragrunt-working-dir "+testPath+" -backend=false")
	require.Error(t, err)

	lockFile := util.JoinPath(testPath, ".terraform.lock.hcl")
	assert.False(t, util.FileExists(lockFile), "Lock file %s exists", lockFile)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt init --terragrunt-non-interactive --terragrunt-config "+tmpTerragruntConfigPath+" --terragrunt-working-dir "+testPath+" --terragrunt-disable-bucket-update -backend=false")
	require.NoError(t, err)

	assert.True(t, util.FileExists(lockFile), "Lock file %s does not exist", lockFile)
}

func applyCommand(configPath, fixturePath string) string {
	return fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", configPath, fixturePath)
}
