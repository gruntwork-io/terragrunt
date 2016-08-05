package test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gruntwork-io/terragrunt/cli"
)

// hard-code this to match the test fixture for now
const (
	TERRAFORM_REMOTE_STATE_S3_REGION      = "us-west-2"
	TERRAFORM_REMOTE_STATE_S3_BUCKET_NAME = "gruntwork-terragrunt-tests"
	TEST_FIXTURE_PATH                     = "fixture/"
)

func TestTerragruntWorksWithLocalTerraformVersion(t *testing.T) {
	if err := validateTerraformIsInstalled(t); err != nil {
		t.Fatalf("A local instance of Terraform is not in the system PATH. Please install Terraform to continue.\n")
	}

	if err := validateS3BucketExists(t, TERRAFORM_REMOTE_STATE_S3_REGION, TERRAFORM_REMOTE_STATE_S3_BUCKET_NAME); err != nil {
		t.Fatalf("The S3 Bucket in the .terragrunt file does not exist. %s", err)
	}

	if err := runTerragruntApply(); err != nil {
		t.Fatalf("Failed to run terragrunt: %s", err)
	}
}

// Run Terragrunt Apply directory in the test fixture path
func runTerragruntApply() error {
	os.Chdir(TEST_FIXTURE_PATH)

	app := cli.CreateTerragruntCli("TEST")

	return app.Run(strings.Split("terragrunt apply", " "))
}

// Validate that a local instance of Terraform is installed.
func validateTerraformIsInstalled(t *testing.T) error {
	if err := runShellCommandWithNoOutput(t, "terraform", "version"); err != nil {
		return err
	}

	return nil
}

// Run the given "command" plus any "args" passed to it. Print the output to stdout.
func runShellCommandWithOutput(t *testing.T, command string, args ...string) error {
	cmd := exec.Command(command, args...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// Run the given "command" plus any "args" passed to it. Suppress output.
func runShellCommandWithNoOutput(t *testing.T, command string, args ...string) error {
	cmd := exec.Command(command, args...)

	cmd.Stdout = nil
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// Check that the S3 Bucket of the given name and region exists by attempting to list objects from it.
func validateS3BucketExists(t *testing.T, awsRegion string, bucketName string) error {
	client, err := createS3Client(awsRegion)
	if err != nil {
		return err
	}

	params := &s3.ListObjectsInput{
		Bucket: aws.String(bucketName),
	}

	_, err = client.ListObjects(params)
	if err != nil {
		return fmt.Errorf("S3 Bucket Name = '%s'. S3 Bucket Region = '%s'. Full Error Message = %s", bucketName, awsRegion, err)
	}

	return nil
}

// Create an authenticated client for S3
func createS3Client(awsRegion string) (*s3.S3, error) {
	config, err := createAwsConfig(awsRegion)
	if err != nil {
		return nil, err
	}

	return s3.New(session.New(), config), nil
}

// Returns an AWS config object for the given region, ensuring that the config has credentials
func createAwsConfig(awsRegion string) (*aws.Config, error) {
	config := defaults.Get().Config.WithRegion(awsRegion)

	_, err := config.Credentials.Get()
	if err != nil {
		return nil, fmt.Errorf("Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?): %s", err)
	}

	return config, nil
}
