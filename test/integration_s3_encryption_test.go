package test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	terraws "github.com/gruntwork-io/terratest/modules/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/config"
)

const (
	s3SSEAESFixturePath            = "fixture-s3-encryption/sse-aes"
	s3SSECustomKeyFixturePath      = "fixture-s3-encryption/custom-key"
	s3SSBasicEncryptionFixturePath = "fixture-s3-encryption/basic-encryption"
	s3SSEKMSFixturePath            = "fixture-s3-encryption/sse-kms"
)

func TestTerragruntS3SSEAES(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, s3SSEAESFixturePath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, s3SSEAESFixturePath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, s3SSEAESFixturePath))

	client := terraws.NewS3Client(t, TERRAFORM_REMOTE_STATE_S3_REGION)
	resp, err := client.GetBucketEncryption(&s3.GetBucketEncryptionInput{Bucket: aws.String(s3BucketName)})
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.ServerSideEncryptionConfiguration.Rules))
	sseRule := resp.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault
	require.NotNil(t, sseRule)
	assert.Equal(t, s3.ServerSideEncryptionAes256, aws.StringValue(sseRule.SSEAlgorithm))
	assert.Nil(t, sseRule.KMSMasterKeyID)
}

func TestTerragruntS3SSECustomKey(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, s3SSECustomKeyFixturePath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, s3SSECustomKeyFixturePath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, s3SSECustomKeyFixturePath))

	client := terraws.NewS3Client(t, TERRAFORM_REMOTE_STATE_S3_REGION)
	resp, err := client.GetBucketEncryption(&s3.GetBucketEncryptionInput{Bucket: aws.String(s3BucketName)})
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.ServerSideEncryptionConfiguration.Rules))
	sseRule := resp.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault
	require.NotNil(t, sseRule)
	assert.Equal(t, s3.ServerSideEncryptionAwsKms, aws.StringValue(sseRule.SSEAlgorithm))
	assert.True(t, strings.HasSuffix(aws.StringValue(sseRule.KMSMasterKeyID), "alias/dedicated-test-key"))

}

func TestTerragruntS3SSEKeyNotReverted(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, s3SSBasicEncryptionFixturePath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, s3SSBasicEncryptionFixturePath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)
	stdout, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", filepath.Dir(tmpTerragruntConfigPath)))
	require.NoError(t, err)
	output := fmt.Sprintf(stdout, stderr)

	// verify that bucket encryption message is not printed
	assert.NotContains(t, output, "Bucket Server-Side Encryption")

	tmpTerragruntConfigPath = createTmpTerragruntConfig(t, s3SSBasicEncryptionFixturePath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)
	stdout, stderr, err = runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", filepath.Dir(tmpTerragruntConfigPath)))
	require.NoError(t, err)
	output = fmt.Sprintf(stdout, stderr)
	assert.NotContains(t, output, "Bucket Server-Side Encryption")

	// verify that encryption key is not reverted
	client := terraws.NewS3Client(t, TERRAFORM_REMOTE_STATE_S3_REGION)
	resp, err := client.GetBucketEncryption(&s3.GetBucketEncryptionInput{Bucket: aws.String(s3BucketName)})
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.ServerSideEncryptionConfiguration.Rules))
	sseRule := resp.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault
	require.NotNil(t, sseRule)
	assert.Equal(t, s3.ServerSideEncryptionAwsKms, aws.StringValue(sseRule.SSEAlgorithm))
	assert.True(t, strings.HasSuffix(aws.StringValue(sseRule.KMSMasterKeyID), "alias/dedicated-test-key"))
}

func TestTerragruntS3EncryptionWarning(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, s3SSEKMSFixturePath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	require.NoError(t, createS3BucketE(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName))

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, s3SSEKMSFixturePath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	terragruntCommand := fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, s3SSEKMSFixturePath)
	stdout, stderr, err := runTerragruntCommandWithOutput(t, terragruntCommand)
	require.NoError(t, err)
	output := fmt.Sprintf(stdout, stderr)
	// check that warning is printed
	assert.Contains(t, output, fmt.Sprintf("Encryption is not enabled on the S3 remote state bucket %s", s3BucketName))

	// verify that encryption configuration is set
	client := terraws.NewS3Client(t, TERRAFORM_REMOTE_STATE_S3_REGION)
	resp, err := client.GetBucketEncryption(&s3.GetBucketEncryptionInput{Bucket: aws.String(s3BucketName)})
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.ServerSideEncryptionConfiguration.Rules))
	sseRule := resp.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault
	require.NotNil(t, sseRule)
	assert.Equal(t, s3.ServerSideEncryptionAwsKms, aws.StringValue(sseRule.SSEAlgorithm))

	// check that second warning is not printed
	stdout, stderr, err = runTerragruntCommandWithOutput(t, terragruntCommand)
	require.NoError(t, err)
	output = fmt.Sprintf(stdout, stderr)
	assert.NotContains(t, output, fmt.Sprintf("Encryption is not enabled on the S3 remote state bucket %s", s3BucketName))
}
