package test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gruntwork-io/terragrunt/cli"
	"bytes"
	"time"
	"math/rand"
	"io/ioutil"
	"path"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/stretchr/testify/assert"
)

// hard-code this to match the test fixture for now
const (
	TERRAFORM_REMOTE_STATE_S3_REGION      = "us-west-2"
	TEST_FIXTURE_PATH                     = "fixture/"
)

func TestTerragruntWorksWithLocalTerraformVersion(t *testing.T) {
	validateCommandInstalled(t, "terraform")

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, TEST_FIXTURE_PATH, s3BucketName)

	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
	runTerragruntApply(t, TEST_FIXTURE_PATH, tmpTerragruntConfigPath)
	validateS3BucketExists(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)
}

// Run Terragrunt Apply directory in the test fixture path
func runTerragruntApply(t *testing.T, templatesPath string, terragruntConfigPath string) {
	os.Chdir(templatesPath)

	app := cli.CreateTerragruntCli("TEST")

	cmd := fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-config %s", terragruntConfigPath)
	args := strings.Split(cmd, " ")

	if err := app.Run(args); err != nil {
		t.Fatalf("Failed to run terragrunt: %s", err)
	}
}

func createTmpTerragruntConfig(t *testing.T, templatesPath string, s3BucketName string) string {
	tmpTerragruntConfigFile, err := ioutil.TempFile("", ".terragrunt")
	if err != nil {
		t.Fatalf("Failed to create temp file due to error: %v", err)
	}

	originalTerragruntConfigPath := path.Join(templatesPath, ".terragrunt")
	originalTerragruntConfigBytes, err := ioutil.ReadFile(originalTerragruntConfigPath)
	if err != nil {
		t.Fatalf("Error reading Terragrunt config at %s: %v", originalTerragruntConfigPath, err)
	}

	originalTerragruntConfigString := string(originalTerragruntConfigBytes)
	newTerragruntConfigString := strings.Replace(originalTerragruntConfigString, "__FILL_IN_BUCKET_NAME__", s3BucketName, -1)

	if err := ioutil.WriteFile(tmpTerragruntConfigFile.Name(), []byte(newTerragruntConfigString), 0444); err != nil {
		t.Fatalf("Error writing temp Terragrunt config to %s: %v", tmpTerragruntConfigFile.Name(), err)
	}

	return tmpTerragruntConfigFile.Name()
}

// Returns a unique (ish) id we can attach to resources and tfstate files so they don't conflict with each other
// Uses base 62 to generate a 6 character string that's unlikely to collide with the handful of tests we run in
// parallel. Based on code here: http://stackoverflow.com/a/9543797/483528
func uniqueId() string {
	const BASE_62_CHARS = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const UNIQUE_ID_LENGTH = 6 // Should be good for 62^6 = 56+ billion combinations

	var out bytes.Buffer

	randInstance := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < UNIQUE_ID_LENGTH; i++ {
		out.WriteByte(BASE_62_CHARS[randInstance.Intn(len(BASE_62_CHARS))])
	}

	return out.String()
}

// Validate that the given command is available in PATH
func validateCommandInstalled(t *testing.T, command string) {
	_, err := exec.LookPath(command)
	if err != nil {
		t.Fatalf("Command '%s' not found in PATH", command)
	}
}

// Check that the S3 Bucket of the given name and region exists. Terragrunt should create this bucket during the test.
func validateS3BucketExists(t *testing.T, awsRegion string, bucketName string) {
	s3Client, err := remote.CreateS3Client(awsRegion)
	if err != nil {
		t.Fatalf("Error creating S3 client: %v", err)
	}

	remoteStateConfig := remote.RemoteStateConfigS3{Bucket: bucketName, Region: awsRegion}
	assert.True(t, remote.DoesS3BucketExist(s3Client, &remoteStateConfig), "Terragrunt failed to create remote state S3 bucket %s", bucketName)
}

// Delete the specified S3 bucket to clean up after a test
func deleteS3Bucket(t *testing.T, awsRegion string, bucketName string) {
	s3Client, err := remote.CreateS3Client(awsRegion)
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
			Key: version.Key,
			VersionId: version.VersionId,
		})
	}

	deleteInput := &s3.DeleteObjectsInput{
		Bucket: aws.String(bucketName),
		Delete: &s3.Delete{Objects: objectIdentifiers},
	}
	if _, err := s3Client.DeleteObjects(deleteInput); err != nil {
		t.Fatalf("Error deleting all versions of all objects in bucket %s: %v", bucketName, err)
	}

	if _, err := s3Client.DeleteBucket(&s3.DeleteBucketInput{Bucket: aws.String(bucketName)}); err != nil {
		t.Fatalf("Failed to delete S3 bucket %s: %v", bucketName, err)
	}
}
