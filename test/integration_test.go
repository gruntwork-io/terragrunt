package test

import (
	"fmt"
	"strings"
	"testing"

	"bytes"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/config"
	terragruntDynamoDb "github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

// hard-code this to match the test fixture for now
const (
	TERRAFORM_REMOTE_STATE_S3_REGION                    = "us-west-2"
	TEST_FIXTURE_PATH                                   = "fixture/"
	TEST_FIXTURE_INCLUDE_PATH                           = "fixture-include/"
	TEST_FIXTURE_INCLUDE_CHILD_REL_PATH                 = "qa/my-app"
	TEST_FIXTURE_STACK                                  = "fixture-stack/"
	TEST_FIXTURE_OUTPUT_ALL                             = "fixture-output-all"
	TEST_FIXTURE_EXTRA_ARGS_PATH                        = "fixture-extra-args/"
	TEST_FIXTURE_LOCAL_DOWNLOAD_PATH                    = "fixture-download/local"
	TEST_FIXTURE_REMOTE_DOWNLOAD_PATH                   = "fixture-download/remote"
	TEST_FIXTURE_OVERRIDE_DOWNLOAD_PATH                 = "fixture-download/override"
	TEST_FIXTURE_LOCAL_RELATIVE_DOWNLOAD_PATH           = "fixture-download/local-relative"
	TEST_FIXTURE_REMOTE_RELATIVE_DOWNLOAD_PATH          = "fixture-download/remote-relative"
	TEST_FIXTURE_LOCAL_RELATIVE_ARGS_DOWNLOAD_PATH      = "fixture-download/local-with-relative-extra-args"
	TEST_FIXTURE_OLD_CONFIG_INCLUDE_PATH                = "fixture-old-terragrunt-config/include"
	TEST_FIXTURE_OLD_CONFIG_INCLUDE_CHILD_UPDATED_PATH  = "fixture-old-terragrunt-config/include-child-updated"
	TEST_FIXTURE_OLD_CONFIG_INCLUDE_PARENT_UPDATED_PATH = "fixture-old-terragrunt-config/include-parent-updated"
	TEST_FIXTURE_OLD_CONFIG_STACK_PATH                  = "fixture-old-terragrunt-config/stack"
	TEST_FIXTURE_OLD_CONFIG_DOWNLOAD_PATH               = "fixture-old-terragrunt-config/download"
	TERRAFORM_FOLDER                                    = ".terraform"
	TERRAFORM_STATE                                     = "terraform.tfstate"
	TERRAFORM_STATE_BACKUP                              = "terraform.tfstate.backup"
	DEFAULT_TEST_REGION                                 = "us-east-1"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func TestTerragruntWorksWithLocalTerraformVersion(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_PATH)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, TEST_FIXTURE_PATH, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, TEST_FIXTURE_PATH))
	validateS3BucketExists(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
}

func TestTerragruntWorksWithIncludes(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(TEST_FIXTURE_INCLUDE_PATH, TEST_FIXTURE_INCLUDE_CHILD_REL_PATH)
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, TEST_FIXTURE_INCLUDE_PATH, TEST_FIXTURE_INCLUDE_CHILD_REL_PATH, s3BucketName, config.DefaultTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
}

func TestTerragruntWorksWithIncludesAndOldConfig(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(TEST_FIXTURE_OLD_CONFIG_INCLUDE_PATH, "child")
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, TEST_FIXTURE_OLD_CONFIG_INCLUDE_PATH, "child", s3BucketName, config.OldTerragruntConfigPath, config.OldTerragruntConfigPath)

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
}

func TestTerragruntWorksWithIncludesChildUpdatedAndOldConfig(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(TEST_FIXTURE_OLD_CONFIG_INCLUDE_CHILD_UPDATED_PATH, "child")
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, TEST_FIXTURE_OLD_CONFIG_INCLUDE_CHILD_UPDATED_PATH, "child", s3BucketName, config.OldTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
}

func TestTerragruntWorksWithIncludesParentUpdatedAndOldConfig(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(TEST_FIXTURE_OLD_CONFIG_INCLUDE_PARENT_UPDATED_PATH, "child")
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, TEST_FIXTURE_OLD_CONFIG_INCLUDE_PARENT_UPDATED_PATH, "child", s3BucketName, config.DefaultTerragruntConfigPath, config.OldTerragruntConfigPath)

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
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
}

func TestTerragruntStackCommands(t *testing.T) {
	t.Parallel()

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	lockTableName := fmt.Sprintf("terragrunt-test-locks-%s", strings.ToLower(uniqueId()))

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_STACK)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, "fixture-stack", config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, lockTableName)

	mgmtEnvironmentPath := fmt.Sprintf("%s/fixture-stack/mgmt", tmpEnvPath)
	stageEnvironmentPath := fmt.Sprintf("%s/fixture-stack/stage", tmpEnvPath)

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	defer cleanupTableForTest(t, lockTableName, TERRAFORM_REMOTE_STATE_S3_REGION)

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

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_OLD_CONFIG_STACK_PATH)

	rootPath := util.JoinPath(tmpEnvPath, "fixture-old-terragrunt-config/stack")
	stagePath := util.JoinPath(rootPath, "stage")
	rootTerragruntConfigPath := util.JoinPath(rootPath, config.OldTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used")

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

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

func TestLocalWithRelativeExtraArgs(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_RELATIVE_ARGS_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_RELATIVE_ARGS_DOWNLOAD_PATH))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_RELATIVE_ARGS_DOWNLOAD_PATH))
}

func TestExtraArguments(t *testing.T) {
	t.Parallel()
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_EXTRA_ARGS_PATH))
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
	tmpDir, err := ioutil.TempDir("", "terragrunt-stack-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

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
func validateS3BucketExists(t *testing.T, awsRegion string, bucketName string) {
	s3Client, err := remote.CreateS3Client(awsRegion, "")
	if err != nil {
		t.Fatalf("Error creating S3 client: %v", err)
	}

	remoteStateConfig := remote.RemoteStateConfigS3{Bucket: bucketName, Region: awsRegion}
	assert.True(t, remote.DoesS3BucketExist(s3Client, &remoteStateConfig), "Terragrunt failed to create remote state S3 bucket %s", bucketName)
}

// Delete the specified S3 bucket to clean up after a test
func deleteS3Bucket(t *testing.T, awsRegion string, bucketName string) {
	s3Client, err := remote.CreateS3Client(awsRegion, "")
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
func createDynamoDbClient(awsRegion, awsProfile string) (*dynamodb.DynamoDB, error) {
	session, err := aws_helper.CreateAwsSession(awsRegion, awsProfile)
	if err != nil {
		return nil, err
	}

	return dynamodb.New(session), nil
}

func createDynamoDbClientForTest(t *testing.T, awsRegion string) *dynamodb.DynamoDB {
	client, err := createDynamoDbClient(awsRegion, "")
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
