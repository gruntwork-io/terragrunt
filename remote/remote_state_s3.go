package remote

import (
	"github.com/mitchellh/mapstructure"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws"
	"fmt"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"time"
)

// A representation of the configuration options available for S3 remote state
type RemoteStateConfigS3 struct {
	Encrypt string
	Bucket  string
	Key     string
	Region  string
}

const MAX_RETRIES_WAITING_FOR_S3_BUCKET = 12
const SLEEP_BETWEEN_RETRIES_WAITING_FOR_S3_BUCKET = 5 * time.Second

// Initialize the remote state S3 bucket specified in the given config. This function will validate the config
// parameters, create the S3 bucket if it doesn't already exist, and check that versioning is enabled.
func InitializeRemoteStateS3(config map[string]string, terragruntOptions *options.TerragruntOptions) error {
	s3Config, err := parseS3Config(config)
	if err != nil {
		return err
	}

	if err := validateS3Config(s3Config, terragruntOptions); err != nil {
		return err
	}

	s3Client, err := CreateS3Client(s3Config.Region)
	if err != nil {
		return err
	}

	if err := createS3BucketIfNecessary(s3Client, s3Config, terragruntOptions); err != nil {
		return err
	}

	if err := checkIfVersioningEnabled(s3Client, s3Config, terragruntOptions); err != nil {
		return err
	}

	return nil
}

// Parse the given map into an S3 config
func parseS3Config(config map[string]string) (*RemoteStateConfigS3, error) {
	var s3Config RemoteStateConfigS3
	if err := mapstructure.Decode(config, &s3Config); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return &s3Config, nil
}

// Validate all the parameters of the given S3 remote state configuration
func validateS3Config(config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if config.Region == "" {
		return errors.WithStackTrace(MissingRequiredS3RemoteStateConfig("region"))
	}

	if config.Bucket == "" {
		return errors.WithStackTrace(MissingRequiredS3RemoteStateConfig("bucket"))
	}

	if config.Key == "" {
		return errors.WithStackTrace(MissingRequiredS3RemoteStateConfig("key"))
	}

	if config.Encrypt != "true" {
		util.Logger.Printf("WARNING: encryption is not enabled on the S3 remote state bucket %s. Terraform state files may contain secrets, so we STRONGLY recommend enablying encryption!", config.Bucket)
	}

	return nil
}

// If the bucket specified in the given config doesn't already exist, prompt the user to create it, and if the user
// confirms, create the bucket and enable versioning for it.
func createS3BucketIfNecessary(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if !DoesS3BucketExist(s3Client, config) {
		prompt := fmt.Sprintf("Remote state S3 bucket %s does not exist or you don't have permissions to access it. Would you Terragrunt to create it?", config.Bucket)
		shouldCreateBucket, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
		if err != nil {
			return err
		}

		if shouldCreateBucket {
			return CreateS3BucketWithVersioning(s3Client, config)
		}
	}

	return nil
}

// Check if versioning is enabled for the S3 bucket specified in the given config and warn the user if it is not
func checkIfVersioningEnabled(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	out, err := s3Client.GetBucketVersioning(&s3.GetBucketVersioningInput{Bucket: aws.String(config.Bucket)})
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// NOTE: There must be a bug in the AWS SDK since out == nil when versioning is not enabled. In the future,
	// check the AWS SDK for updates to see if we can remove "out == nil ||".
	if out == nil || out.Status == nil || *out.Status != s3.BucketVersioningStatusEnabled {
		util.Logger.Printf("WARNING: Versioning is not enabled for the remote state S3 bucket %s. We recommend enabling versioning so that you can roll back to previous versions of your Terraform state in case of error.", config.Bucket)
	}

	return nil
}

// Create the given S3 bucket and enable versioning for it
func CreateS3BucketWithVersioning(s3Client *s3.S3, config *RemoteStateConfigS3) error {
	if err := CreateS3Bucket(s3Client, config); err != nil {
		return err
	}

	if err := WaitUntilS3BucketExists(s3Client, config); err != nil {
		return err
	}

	if err := EnableVersioningForS3Bucket(s3Client, config); err != nil {
		return err
	}

	return nil
}

// AWS is eventually consistent, so after creating an S3 bucket, this method can be used to wait until the information
// about that S3 bucket has propagated everywhere
func WaitUntilS3BucketExists(s3Client *s3.S3, config *RemoteStateConfigS3) error {
	for retries := 0; retries < MAX_RETRIES_WAITING_FOR_S3_BUCKET; retries++ {
		if DoesS3BucketExist(s3Client, config) {
			util.Logger.Printf("S3 bucket %s created.", config.Bucket)
			return nil
		} else if retries < MAX_RETRIES_WAITING_FOR_S3_BUCKET - 1 {
			util.Logger.Printf("S3 bucket %s has not been created yet. Sleeping for %s and will check again.", config.Bucket, SLEEP_BETWEEN_RETRIES_WAITING_FOR_S3_BUCKET)
			time.Sleep(SLEEP_BETWEEN_RETRIES_WAITING_FOR_S3_BUCKET)
		}
	}

	return errors.WithStackTrace(MaxRetriesWaitingForS3BucketExceeded(config.Bucket))
}

// Create the S3 bucket specified in the given config
func CreateS3Bucket(s3Client *s3.S3, config *RemoteStateConfigS3) error {
	util.Logger.Printf("Creating S3 bucket %s", config.Bucket)
	_, err := s3Client.CreateBucket(&s3.CreateBucketInput{Bucket: aws.String(config.Bucket)})
	return errors.WithStackTrace(err)
}

// Enable versioning for the S3 bucket specified in the given config
func EnableVersioningForS3Bucket(s3Client *s3.S3, config *RemoteStateConfigS3) error {
	util.Logger.Printf("Enabling versioning on S3 bucket %s", config.Bucket)
	input := s3.PutBucketVersioningInput{
		Bucket: aws.String(config.Bucket),
		VersioningConfiguration: &s3.VersioningConfiguration{Status: aws.String(s3.BucketVersioningStatusEnabled)},
	}
	_, err := s3Client.PutBucketVersioning(&input)
	return errors.WithStackTrace(err)
}

// Returns true if the S3 bucket specified in the given config exists and the current user has the ability to access
// it.
func DoesS3BucketExist(s3Client *s3.S3, config *RemoteStateConfigS3) bool {
	_, err := s3Client.HeadBucket(&s3.HeadBucketInput{Bucket: aws.String(config.Bucket)})
	return err == nil
}

// Create an authenticated client for DynamoDB
func CreateS3Client(awsRegion string) (*s3.S3, error) {
	config, err := aws_helper.CreateAwsConfig(awsRegion)
	if err != nil {
		return nil, err
	}

	return s3.New(session.New(), config), nil
}

// Custom error types

type MissingRequiredS3RemoteStateConfig string
func (configName MissingRequiredS3RemoteStateConfig) Error() string {
	return fmt.Sprintf("Missing required S3 remote state configuration %s", string(configName))
}

type MaxRetriesWaitingForS3BucketExceeded string
func (err MaxRetriesWaitingForS3BucketExceeded) Error() string {
	return fmt.Sprintf("Exceeded max retries (%d) waiting for bucket S3 bucket %s", MAX_RETRIES_WAITING_FOR_S3_BUCKET, string(err))
}
