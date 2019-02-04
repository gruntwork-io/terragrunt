package remote

import (
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/mitchellh/mapstructure"
)

/*
 * We use this construct to separate the two config keys 's3_bucket_tags' and 'dynamodb_table_tags'
 * from the others, as they are specific to the s3 backend, but only used by terragrunt to tag
 * the s3 bucket and the dynamo db, in case it has to create them.
 */
type ExtendedRemoteStateConfigS3 struct {
	remoteStateConfigS3 RemoteStateConfigS3

	S3BucketTags            []map[string]string `mapstructure:"s3_bucket_tags"`
	DynamotableTags         []map[string]string `mapstructure:"dynamodb_table_tags"`
	SkipBucketVersioning    bool                `mapstructure:"skip_bucket_versioning"`
	SkipBucketSSEncryption  bool                `mapstructure:"skip_bucket_ssencryption"`
	SkipBucketAccessLogging bool                `mapstructure:"skip_bucket_accesslogging"`
}

// A representation of the configuration options available for S3 remote state
type RemoteStateConfigS3 struct {
	Encrypt          bool   `mapstructure:"encrypt"`
	Bucket           string `mapstructure:"bucket"`
	Key              string `mapstructure:"key"`
	Region           string `mapstructure:"region"`
	Endpoint         string `mapstructure:"endpoint"`
	Profile          string `mapstructure:"profile"`
	RoleArn          string `mapstructure:"role_arn"`
	LockTable        string `mapstructure:"lock_table"`
	DynamoDBTable    string `mapstructure:"dynamodb_table"`
	CredsFilename    string `mapstructure:"shared_credentials_file"`
	S3ForcePathStyle bool   `mapstructure:"force_path_style"`
}

// Builds a session config for AWS related requests from the RemoteStateConfigS3 configuration
func (c *RemoteStateConfigS3) GetAwsSessionConfig() *aws_helper.AwsSessionConfig {
	return &aws_helper.AwsSessionConfig{
		Region:           c.Region,
		CustomS3Endpoint: c.Endpoint,
		Profile:          c.Profile,
		RoleArn:          c.RoleArn,
		CredsFilename:    c.CredsFilename,
		S3ForcePathStyle: c.S3ForcePathStyle,
	}
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

// Returns true if:
//
// 1. Any of the existing backend settings are different than the current config
// 2. The configured S3 bucket or DynamoDB table does not exist
func (s3Initializer S3Initializer) NeedsInitialization(config map[string]interface{}, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if !configValuesEqual(config, existingBackend, terragruntOptions) {
		return true, nil
	}

	s3Config, err := parseS3Config(config)
	if err != nil {
		return false, err
	}

	sessionConfig := s3Config.GetAwsSessionConfig()

	s3Client, err := CreateS3Client(sessionConfig, terragruntOptions)
	if err != nil {
		return false, err
	}

	if !DoesS3BucketExist(s3Client, s3Config) {
		return true, nil
	}

	if s3Config.GetLockTableName() != "" {
		dynamodbClient, err := dynamodb.CreateDynamoDbClient(sessionConfig, terragruntOptions)
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

// Return true if the given config is in any way different than what is configured for the backend
func configValuesEqual(config map[string]interface{}, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) bool {
	if existingBackend == nil {
		return len(config) == 0
	}

	if existingBackend.Type != "s3" {
		terragruntOptions.Logger.Printf("Backend type has changed from s3 to %s", existingBackend.Type)
		return false
	}

	if len(config) == 0 && len(existingBackend.Config) == 0 {
		return true
	}

	// Terraform's `backend` configuration uses a boolean for the `encrypt` parameter. However, perhaps for backwards compatibility reasons,
	// Terraform stores that parameter as a string in the `terraform.tfstate` file. Therefore, we have to convert it accordingly, or `DeepEqual`
	// will fail.
	if util.KindOf(existingBackend.Config["encrypt"]) == reflect.String && util.KindOf(config["encrypt"]) == reflect.Bool {
		// If encrypt in remoteState is a bool and a string in existingBackend, DeepEqual will consider the maps to be different.
		// So we convert the value from string to bool to make them equivalent.
		if value, err := strconv.ParseBool(existingBackend.Config["encrypt"].(string)); err == nil {
			existingBackend.Config["encrypt"] = value
		} else {
			terragruntOptions.Logger.Printf("Remote state configuration encrypt contains invalid value %v, should be boolean.", existingBackend.Config["encrypt"])
		}
	}

	// If other keys in config are bools, DeepEqual also will consider the maps to be different.
	for key, value := range existingBackend.Config {
		if util.KindOf(existingBackend.Config[key]) == reflect.String && util.KindOf(config[key]) == reflect.Bool {
			if convertedValue, err := strconv.ParseBool(value.(string)); err == nil {
				existingBackend.Config[key] = convertedValue
			}
		}
	}

	// Delete S3 and DynamoDB and Bucket Versioning tags, as these are only stored in Terragrunt config and not in Terraform's backend
	delete(config, "s3_bucket_tags")
	delete(config, "dynamodb_table_tags")
	delete(config, "skip_bucket_versioning")
	delete(config, "skip_bucket_ssencryption")
	delete(config, "skip_bucket_accesslogging")

	if !reflect.DeepEqual(existingBackend.Config, config) {
		terragruntOptions.Logger.Printf("Backend config has changed from %s to %s", existingBackend.Config, config)
		return false
	}

	return true
}

// Initialize the remote state S3 bucket specified in the given config. This function will validate the config
// parameters, create the S3 bucket if it doesn't already exist, and check that versioning is enabled.
func (s3Initializer S3Initializer) Initialize(config map[string]interface{}, terragruntOptions *options.TerragruntOptions) error {
	s3ConfigExtended, err := parseExtendedS3Config(config)
	if err != nil {
		return err
	}

	if err := validateS3Config(s3ConfigExtended, terragruntOptions); err != nil {
		return err
	}

	var s3Config = s3ConfigExtended.remoteStateConfigS3

	s3Client, err := CreateS3Client(s3Config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return err
	}

	if err := createS3BucketIfNecessary(s3Client, s3ConfigExtended, terragruntOptions); err != nil {
		return err
	}

	if err := checkIfVersioningEnabled(s3Client, &s3Config, terragruntOptions); err != nil {
		return err
	}

	if err := createLockTableIfNecessary(&s3Config, s3ConfigExtended.DynamotableTags, terragruntOptions); err != nil {
		return err
	}

	return nil
}

func (s3Initializer S3Initializer) GetTerraformInitArgs(config map[string]interface{}) map[string]interface{} {
	var filteredConfig = make(map[string]interface{})

	for key, val := range config {

		if key == "s3_bucket_tags" || key == "dynamodb_table_tags" || key == "skip_bucket_versioning" || key == "skip_bucket_ssencryption" || key == "skip_bucket_accesslogging" {
			continue
		}

		filteredConfig[key] = val
	}

	return filteredConfig
}

// Parse the given map into an S3 config
func parseS3Config(config map[string]interface{}) (*RemoteStateConfigS3, error) {
	var s3Config RemoteStateConfigS3
	if err := mapstructure.Decode(config, &s3Config); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return &s3Config, nil
}

// Parse the given map into an extended S3 config
func parseExtendedS3Config(config map[string]interface{}) (*ExtendedRemoteStateConfigS3, error) {
	var s3Config RemoteStateConfigS3
	var extendedConfig ExtendedRemoteStateConfigS3

	if err := mapstructure.Decode(config, &s3Config); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if err := mapstructure.Decode(config, &extendedConfig); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	extendedConfig.remoteStateConfigS3 = s3Config

	return &extendedConfig, nil
}

// Validate all the parameters of the given S3 remote state configuration
func validateS3Config(extendedConfig *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	var config = extendedConfig.remoteStateConfigS3

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

	if len(extendedConfig.S3BucketTags) > 1 {
		return errors.WithStackTrace(MultipleTagsDeclarations("S3 bucket"))

	}

	if len(extendedConfig.DynamotableTags) > 1 {
		return errors.WithStackTrace(MultipleTagsDeclarations("DynamoDB table"))

	}

	return nil
}

// If the bucket specified in the given config doesn't already exist, prompt the user to create it, and if the user
// confirms, create the bucket and enable versioning for it.
func createS3BucketIfNecessary(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if !DoesS3BucketExist(s3Client, &config.remoteStateConfigS3) {
		prompt := fmt.Sprintf("Remote state S3 bucket %s does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", config.remoteStateConfigS3.Bucket)
		shouldCreateBucket, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
		if err != nil {
			return err
		}

		if shouldCreateBucket {
			return CreateS3BucketWithVersioningSSEcryptionAndAccessLogging(s3Client, config, terragruntOptions)
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
func CreateS3BucketWithVersioningSSEncryptionAndAccessLogging(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	err := CreateS3Bucket(s3Client, &config.remoteStateConfigS3, terragruntOptions)

	if err != nil {
		if isBucketAlreadyOwnedByYourError(err) {
			terragruntOptions.Logger.Printf("Looks like someone is creating bucket %s at the same time. Will not attempt to create it again.", config.remoteStateConfigS3.Bucket)
			return WaitUntilS3BucketExists(s3Client, &config.remoteStateConfigS3, terragruntOptions)
		}

		return err
	}

	if err := WaitUntilS3BucketExists(s3Client, &config.remoteStateConfigS3, terragruntOptions); err != nil {
		return err
	}

	if err := TagS3Bucket(s3Client, config, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketVersioning {
		terragruntOptions.Logger.Printf("Versioning is disabled for the remote state S3 bucket %s using 'skip_bucket_versioning' config.", config.remoteStateConfigS3.Bucket)
	} else if err := EnableVersioningForS3Bucket(s3Client, &config.remoteStateConfigS3, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketSSEncryption {
		terragruntOptions.Logger.Printf("Server-Side Encryption is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_ssencryption' config.", config.remoteStateConfigS3.Bucket)
	} else if err := EnableSSEForS3BucketWide(s3Client, &config.remoteStateConfigS3, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketAccessLogging {
		terragruntOptions.Logger.Printf("Access Logging is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_accesslogging' config.", config.remoteStateConfigS3.Bucket)
	} else if err := EnableAccessLoggingForS3BucketWide(s3Client, &config.remoteStateConfigS3, terragruntOptions); err != nil {
		return err
	}

	return nil
}

func TagS3Bucket(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {

	if config.S3BucketTags == nil || len(config.S3BucketTags) == 0 {
		terragruntOptions.Logger.Println("No tags for S3 bucket given.")
		return nil
	}

	// There must be one entry in the list
	var tags = config.S3BucketTags[0]
	var tagsConverted = convertTags(tags)

	terragruntOptions.Logger.Printf("Tagging S3 bucket with %s", tags)

	putBucketTaggingInput := s3.PutBucketTaggingInput{
		Bucket: aws.String(config.remoteStateConfigS3.Bucket),
		Tagging: &s3.Tagging{
			TagSet: tagsConverted}}

	_, err := s3Client.PutBucketTagging(&putBucketTaggingInput)

	if err != nil {
		return errors.WithStackTrace(err)
	}

	return nil

}

func convertTags(tags map[string]string) []*s3.Tag {

	var tagsConverted []*s3.Tag

	for k, v := range tags {
		var tag = s3.Tag{
			Key:   aws.String(k),
			Value: aws.String(v)}

		tagsConverted = append(tagsConverted, &tag)
	}

	return tagsConverted
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

// Determine if this is an error that implies you've already made a request to create the S3 bucket and it succeeded
// or is in progress. This usually happens when running many tests in parallel or xxx-all commands.
func isBucketAlreadyOwnedByYourError(err error) bool {
	awsErr, isAwsErr := errors.Unwrap(err).(awserr.Error)
	return isAwsErr && (awsErr.Code() == "BucketAlreadyOwnedByYou" || awsErr.Code() == "OperationAborted")
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

// Enable bucket-wide Server-Side Encryption for the AWS S3 bucket specified in the given config
func EnableSSEForS3BucketWide(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Enabling bucket-wide SSE on AWS S3 bucket %s", config.Bucket)
	// Encrypt with KMS by default
	defEnc := &s3.ServerSideEncryptionByDefault{KMSMasterKeyID: aws.String("aws/s3"), SSEAlgorithm: aws.String(s3.ServerSideEncryptionAwsKms)}
	rule := &s3.ServerSideEncryptionRule{ApplyServerSideEncryptionByDefault: defEnc}
	rules := []*s3.ServerSideEncryptionRule{rule}
	serverConfig := &s3.ServerSideEncryptionConfiguration{Rules: rules}
	input := &s3.PutBucketEncryptionInput{Bucket: aws.String(config.Bucket), ServerSideEncryptionConfiguration: serverConfig}

	_, err := s3Client.PutBucketEncryption(input)
	return errors.WithStackTrace(err)
}

// Enable bucket-wide Access Logging for the AWS S3 bucket specified in the given config
func EnableAccessLoggingForS3BucketWide(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Enabling bucket-wide Access Logging on AWS S3 bucket \"%s\" - using as TargetBucket \"%s\"", config.Bucket, config.Bucket)
	input := s3.PutBucketLoggingInput{
		Bucket: aws.String(config.Bucket),
		BucketLoggingStatus: &s3.BucketLoggingStatus{
			LoggingEnabled: &s3.LoggingEnabled{
				TargetBucket: aws.String(config.Bucket),
				TargetPrefix: aws.String("TFStateLogs/"),
			},
		},
	}

	_, err := s3Client.PutBucketLogging(&input)
	return errors.WithStackTrace(err)
}

// Returns true if the S3 bucket specified in the given config exists and the current user has the ability to access
// it.
func DoesS3BucketExist(s3Client *s3.S3, config *RemoteStateConfigS3) bool {
	_, err := s3Client.HeadBucket(&s3.HeadBucketInput{Bucket: aws.String(config.Bucket)})
	return err == nil
}

// Create a table for locks in DynamoDB if the user has configured a lock table and the table doesn't already exist
func createLockTableIfNecessary(s3Config *RemoteStateConfigS3, tagsDeclarations []map[string]string, terragruntOptions *options.TerragruntOptions) error {

	if s3Config.GetLockTableName() == "" {
		return nil
	}

	dynamodbClient, err := dynamodb.CreateDynamoDbClient(s3Config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return err
	}

	var tags map[string]string = nil
	if len(tagsDeclarations) == 1 {
		tags = tagsDeclarations[0]
	}

	return dynamodb.CreateLockTableIfNecessary(s3Config.GetLockTableName(), tags, dynamodbClient, terragruntOptions)
}

// Create an authenticated client for DynamoDB
func CreateS3Client(config *aws_helper.AwsSessionConfig, terragruntOptions *options.TerragruntOptions) (*s3.S3, error) {
	session, err := aws_helper.CreateAwsSession(config, terragruntOptions)
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

type MultipleTagsDeclarations string

func (target MultipleTagsDeclarations) Error() string {
	return fmt.Sprintf("Tags for %s got declared multiple times. Please do only declare in one block.", string(target))
}

type MaxRetriesWaitingForS3BucketExceeded string

func (err MaxRetriesWaitingForS3BucketExceeded) Error() string {
	return fmt.Sprintf("Exceeded max retries (%d) waiting for bucket S3 bucket %s", MAX_RETRIES_WAITING_FOR_S3_BUCKET, string(err))
}
