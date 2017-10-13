package remote

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/mitchellh/mapstructure"
)

// A representation of the configuration options available for S3 remote state
type RemoteStateConfigS3 struct {
	Encrypt       bool   `mapstructure:"encrypt"`
	Bucket        string `mapstructure:"bucket"`
	Key           string `mapstructure:"key"`
	Region        string `mapstructure:"region"`
	Profile       string `mapstructure:"profile"`
	LockTable     string `mapstructure:"lock_table"`
	DynamoDBTable string `mapstructure:"dynamodb_table"`
}

// The DynamoDB lock table name used to be called lock_table, but has since been renamed to dynamodb_table, and the old
// name deprecated. To maintain backwards compatibility, we support both names.
func (s3Config *RemoteStateConfigS3) GetLockTableName() string {
	if s3Config.DynamoDBTable != "" {
		return s3Config.DynamoDBTable
	}
	return s3Config.LockTable
}

const MAX_RETRIES_WAITING_FOR_S3_BUCKET = 12
const SLEEP_BETWEEN_RETRIES_WAITING_FOR_S3_BUCKET = 5 * time.Second

type S3Initializer struct{}

// Returns true if the S3 bucket or DynamoDB table does not exist
func (s3Initializer S3Initializer) NeedsInitialization(config map[string]interface{}, terragruntOptions *options.TerragruntOptions) (bool, error) {
	s3Config, err := parseS3Config(config)
	if err != nil {
		return false, err
	}

	s3Client, err := CreateS3Client(s3Config.Region, s3Config.Profile)
	if err != nil {
		return false, err
	}

	if !DoesS3BucketExist(s3Client, s3Config) {
		return true, nil
	}

	if s3Config.GetLockTableName() != "" {
		dynamodbClient, err := dynamodb.CreateDynamoDbClient(s3Config.Region, s3Config.Profile)
		if err != nil {
			return false, err
		}

		tableExists, err := dynamodb.LockTableExistsAndIsActive(s3Config.GetLockTableName(), dynamodbClient)
		if err != nil {
			return false, err
		}
		if !tableExists {
			return true, nil
		}
	}

	return false, nil
}

// Initialize the remote state S3 bucket specified in the given config. This function will validate the config
// parameters, create the S3 bucket if it doesn't already exist, and check that versioning is enabled.
func (s3Initializer S3Initializer) Initialize(config map[string]interface{}, terragruntOptions *options.TerragruntOptions) error {
	s3Config, err := parseS3Config(config)
	if err != nil {
		return err
	}

	if err := validateS3Config(s3Config, terragruntOptions); err != nil {
		return err
	}

	s3Client, err := CreateS3Client(s3Config.Region, s3Config.Profile)
	if err != nil {
		return err
	}

	if err := createS3BucketIfNecessary(s3Client, s3Config, terragruntOptions); err != nil {
		return err
	}

	if err := checkIfVersioningEnabled(s3Client, s3Config, terragruntOptions); err != nil {
		return err
	}

	if err := createLockTableIfNecessary(s3Config, terragruntOptions); err != nil {
		return err
	}

	return nil
}

// Parse the given map into an S3 config
func parseS3Config(config map[string]interface{}) (*RemoteStateConfigS3, error) {
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

	if !config.Encrypt {
		terragruntOptions.Logger.Printf("WARNING: encryption is not enabled on the S3 remote state bucket %s. Terraform state files may contain secrets, so we STRONGLY recommend enabling encryption!", config.Bucket)
	}

	return nil
}

// If the bucket specified in the given config doesn't already exist, prompt the user to create it, and if the user
// confirms, create the bucket and enable versioning for it.
func createS3BucketIfNecessary(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if !DoesS3BucketExist(s3Client, config) {
		if !shell.CanAcceptPrompt() {
			return fmt.Errorf("remote: S3 bucket does not exist, or you don't have permissions to access it: %q", config.Bucket)
		}
		prompt := fmt.Sprintf("Remote state S3 bucket %s does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", config.Bucket)
		shouldCreateBucket, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
		if err != nil {
			return err
		}

		if shouldCreateBucket {
			return CreateS3BucketWithVersioning(s3Client, config, terragruntOptions)
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
		terragruntOptions.Logger.Printf("WARNING: Versioning is not enabled for the remote state S3 bucket %s. We recommend enabling versioning so that you can roll back to previous versions of your Terraform state in case of error.", config.Bucket)
	}

	return nil
}

// Create the given S3 bucket and enable versioning for it
func CreateS3BucketWithVersioning(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if err := CreateS3Bucket(s3Client, config, terragruntOptions); err != nil {
		return err
	}

	if err := WaitUntilS3BucketExists(s3Client, config, terragruntOptions); err != nil {
		return err
	}

	if err := EnableVersioningForS3Bucket(s3Client, config, terragruntOptions); err != nil {
		return err
	}

	return nil
}

// AWS is eventually consistent, so after creating an S3 bucket, this method can be used to wait until the information
// about that S3 bucket has propagated everywhere
func WaitUntilS3BucketExists(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	for retries := 0; retries < MAX_RETRIES_WAITING_FOR_S3_BUCKET; retries++ {
		if DoesS3BucketExist(s3Client, config) {
			terragruntOptions.Logger.Printf("S3 bucket %s created.", config.Bucket)
			return nil
		} else if retries < MAX_RETRIES_WAITING_FOR_S3_BUCKET-1 {
			terragruntOptions.Logger.Printf("S3 bucket %s has not been created yet. Sleeping for %s and will check again.", config.Bucket, SLEEP_BETWEEN_RETRIES_WAITING_FOR_S3_BUCKET)
			time.Sleep(SLEEP_BETWEEN_RETRIES_WAITING_FOR_S3_BUCKET)
		}
	}

	return errors.WithStackTrace(MaxRetriesWaitingForS3BucketExceeded(config.Bucket))
}

// Create the S3 bucket specified in the given config
func CreateS3Bucket(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Creating S3 bucket %s", config.Bucket)
	_, err := s3Client.CreateBucket(&s3.CreateBucketInput{Bucket: aws.String(config.Bucket)})
	return errors.WithStackTrace(err)
}

// Enable versioning for the S3 bucket specified in the given config
func EnableVersioningForS3Bucket(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Enabling versioning on S3 bucket %s", config.Bucket)
	input := s3.PutBucketVersioningInput{
		Bucket:                  aws.String(config.Bucket),
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

// Create a table for locks in DynamoDB if the user has configured a lock table and the table doesn't already exist
func createLockTableIfNecessary(s3Config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if s3Config.GetLockTableName() == "" {
		return nil
	}

	dynamodbClient, err := dynamodb.CreateDynamoDbClient(s3Config.Region, s3Config.Profile)
	if err != nil {
		return err
	}

	return dynamodb.CreateLockTableIfNecessary(s3Config.GetLockTableName(), dynamodbClient, terragruntOptions)
}

// Create an authenticated client for DynamoDB
func CreateS3Client(awsRegion, awsProfile string) (*s3.S3, error) {
	session, err := aws_helper.CreateAwsSession(awsRegion, awsProfile)
	if err != nil {
		return nil, err
	}

	return s3.New(session), nil
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
