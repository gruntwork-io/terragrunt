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
	"github.com/sirupsen/logrus"
)

const (
	lockTableDeprecationMessage              = "Remote state configuration 'lock_table' attribute is deprecated; use 'dynamodb_table' instead."
	DefaultS3BucketAccessLoggingTargetPrefix = "TFStateLogs/"
	SidRootPolicy                            = "RootAccess"
	SidEnforcedTLSPolicy                     = "EnforcedTLS"
)

/*
 * We use this construct to separate the three config keys 's3_bucket_tags', 'dynamodb_table_tags'
 * and 'accesslogging_bucket_tags' from the others, as they are specific to the s3 backend,
 * but only used by terragrunt to tag the s3 bucket, the dynamo db and the s3 bucket used to the
 * access logs, in case it has to create them.
 */
type ExtendedRemoteStateConfigS3 struct {
	remoteStateConfigS3 RemoteStateConfigS3

	S3BucketTags                   map[string]string `mapstructure:"s3_bucket_tags"`
	DynamotableTags                map[string]string `mapstructure:"dynamodb_table_tags"`
	AccessLoggingBucketTags        map[string]string `mapstructure:"accesslogging_bucket_tags"`
	SkipBucketVersioning           bool              `mapstructure:"skip_bucket_versioning"`
	SkipBucketSSEncryption         bool              `mapstructure:"skip_bucket_ssencryption"`
	SkipBucketAccessLogging        bool              `mapstructure:"skip_bucket_accesslogging"`
	SkipBucketRootAccess           bool              `mapstructure:"skip_bucket_root_access"`
	SkipBucketEnforcedTLS          bool              `mapstructure:"skip_bucket_enforced_tls"`
	SkipBucketPublicAccessBlocking bool              `mapstructure:"skip_bucket_public_access_blocking"`
	DisableBucketUpdate            bool              `mapstructure:"disable_bucket_update"`
	EnableLockTableSSEncryption    bool              `mapstructure:"enable_lock_table_ssencryption"`
	DisableAWSClientChecksums      bool              `mapstructure:"disable_aws_client_checksums"`
	AccessLoggingBucketName        string            `mapstructure:"accesslogging_bucket_name"`
	AccessLoggingTargetPrefix      string            `mapstructure:"accesslogging_target_prefix"`
	BucketSSEAlgorithm             string            `mapstructure:"bucket_sse_algorithm"`
	BucketSSEKMSKeyID              string            `mapstructure:"bucket_sse_kms_key_id"`
}

// These are settings that can appear in the remote_state config that are ONLY used by Terragrunt and NOT forwarded
// to the underlying Terraform backend configuration
var terragruntOnlyConfigs = []string{
	"s3_bucket_tags",
	"dynamodb_table_tags",
	"accesslogging_bucket_tags",
	"skip_bucket_versioning",
	"skip_bucket_ssencryption",
	"skip_bucket_accesslogging",
	"skip_bucket_root_access",
	"skip_bucket_enforced_tls",
	"skip_bucket_public_access_blocking",
	"disable_bucket_update",
	"enable_lock_table_ssencryption",
	"disable_aws_client_checksums",
	"accesslogging_bucket_name",
	"accesslogging_target_prefix",
	"bucket_sse_algorithm",
	"bucket_sse_kms_key_id",
}

// A representation of the configuration options available for S3 remote state
type RemoteStateConfigS3 struct {
	Encrypt          bool   `mapstructure:"encrypt"`
	Bucket           string `mapstructure:"bucket"`
	Key              string `mapstructure:"key"`
	Region           string `mapstructure:"region"`
	Endpoint         string `mapstructure:"endpoint"`
	DynamoDBEndpoint string `mapstructure:"dynamodb_endpoint"`
	Profile          string `mapstructure:"profile"`
	RoleArn          string `mapstructure:"role_arn"`
	ExternalID       string `mapstructure:"external_id"`
	SessionName      string `mapstructure:"session_name"`
	LockTable        string `mapstructure:"lock_table"` // Deprecated in Terraform version 0.13 or newer.
	DynamoDBTable    string `mapstructure:"dynamodb_table"`
	CredsFilename    string `mapstructure:"shared_credentials_file"`
	S3ForcePathStyle bool   `mapstructure:"force_path_style"`
}

// Builds a session config for AWS related requests from the RemoteStateConfigS3 configuration
func (c *ExtendedRemoteStateConfigS3) GetAwsSessionConfig() *aws_helper.AwsSessionConfig {
	return &aws_helper.AwsSessionConfig{
		Region:                  c.remoteStateConfigS3.Region,
		CustomS3Endpoint:        c.remoteStateConfigS3.Endpoint,
		CustomDynamoDBEndpoint:  c.remoteStateConfigS3.DynamoDBEndpoint,
		Profile:                 c.remoteStateConfigS3.Profile,
		RoleArn:                 c.remoteStateConfigS3.RoleArn,
		ExternalID:              c.remoteStateConfigS3.ExternalID,
		SessionName:             c.remoteStateConfigS3.SessionName,
		CredsFilename:           c.remoteStateConfigS3.CredsFilename,
		S3ForcePathStyle:        c.remoteStateConfigS3.S3ForcePathStyle,
		DisableComputeChecksums: c.DisableAWSClientChecksums,
	}
}

// The DynamoDB lock table attribute used to be called "lock_table", but has since been renamed to "dynamodb_table", and
// the old attribute name deprecated. The old attribute name has been eventually removed from Terraform starting with
// release 0.13. To maintain backwards compatibility, we support both names.
func (s3Config *RemoteStateConfigS3) GetLockTableName() string {
	if s3Config.DynamoDBTable != "" {
		return s3Config.DynamoDBTable
	}
	return s3Config.LockTable
}

const MAX_RETRIES_WAITING_FOR_S3_BUCKET = 12
const SLEEP_BETWEEN_RETRIES_WAITING_FOR_S3_BUCKET = 5 * time.Second

// To enable access logging in an S3 bucket, you must grant WRITE and READ_ACP permissions to the Log Delivery Group,
// which is represented by the following URI. For more info, see:
// https://docs.aws.amazon.com/AmazonS3/latest/dev/enable-logging-programming.html
const s3LogDeliveryGranteeUri = "http://acs.amazonaws.com/groups/s3/LogDelivery"

type S3Initializer struct{}

// Returns true if:
//
// 1. Any of the existing backend settings are different than the current config
// 2. The configured S3 bucket or DynamoDB table does not exist
func (s3Initializer S3Initializer) NeedsInitialization(remoteState *RemoteState, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if remoteState.DisableInit {
		return false, nil
	}

	// Nowadays it only makes sense to set the "dynamodb_table" attribute as it has
	// been supported in Terraform since the release of version 0.10. The deprecated
	// "lock_table" attribute is either set to NULL in the state file or missing
	// from it altogether. Display a deprecation warning when the "lock_table"
	// attribute is being used.
	if util.KindOf(remoteState.Config["lock_table"]) == reflect.String && remoteState.Config["lock_table"] != "" {
		terragruntOptions.Logger.Warnf("%s\n", lockTableDeprecationMessage)
		remoteState.Config["dynamodb_table"] = remoteState.Config["lock_table"]
		delete(remoteState.Config, "lock_table")
	}

	if !configValuesEqual(remoteState.Config, existingBackend, terragruntOptions) {
		return true, nil
	}

	s3ConfigExtended, err := ParseExtendedS3Config(remoteState.Config)
	if err != nil {
		return false, err
	}
	s3Config := s3ConfigExtended.remoteStateConfigS3

	sessionConfig := s3ConfigExtended.GetAwsSessionConfig()

	s3Client, err := CreateS3Client(sessionConfig, terragruntOptions)
	if err != nil {
		return false, err
	}

	if !DoesS3BucketExist(s3Client, &s3Config.Bucket) {
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
		terragruntOptions.Logger.Debugf("Backend type has changed from s3 to %s", existingBackend.Type)
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
			terragruntOptions.Logger.Warnf("Remote state configuration encrypt contains invalid value %v, should be boolean.", existingBackend.Config["encrypt"])
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

	// We now construct a version of the config that matches what we expect in the backend by stripping out terragrunt
	// related configs.
	terraformConfig := map[string]interface{}{}
	for key, val := range config {
		if !util.ListContainsElement(terragruntOnlyConfigs, key) {
			terraformConfig[key] = val
		}
	}

	if !terraformStateConfigEqual(existingBackend.Config, terraformConfig) {
		terragruntOptions.Logger.Debugf("Backend config changed from %s to %s", existingBackend.Config, config)
		return false
	}

	return true
}

// Initialize the remote state S3 bucket specified in the given config. This function will validate the config
// parameters, create the S3 bucket if it doesn't already exist, and check that versioning is enabled.
func (s3Initializer S3Initializer) Initialize(remoteState *RemoteState, terragruntOptions *options.TerragruntOptions) error {
	s3ConfigExtended, err := ParseExtendedS3Config(remoteState.Config)
	if err != nil {
		return err
	}

	if err := validateS3Config(s3ConfigExtended, terragruntOptions); err != nil {
		return err
	}

	var s3Config = s3ConfigExtended.remoteStateConfigS3

	// Display a deprecation warning when the "lock_table" attribute is being used
	// during initialization.
	if s3Config.LockTable != "" {
		terragruntOptions.Logger.Warnf("%s\n", lockTableDeprecationMessage)
	}

	s3Client, err := CreateS3Client(s3ConfigExtended.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return err
	}

	if err := createS3BucketIfNecessary(s3Client, s3ConfigExtended, terragruntOptions); err != nil {
		return err
	}

	if !terragruntOptions.DisableBucketUpdate && !s3ConfigExtended.DisableBucketUpdate {
		if err := updateS3BucketIfNecessary(s3Client, s3ConfigExtended, terragruntOptions); err != nil {
			return err
		}
	}

	if !s3ConfigExtended.SkipBucketVersioning {
		if _, err := checkIfVersioningEnabled(s3Client, &s3Config, terragruntOptions); err != nil {
			return err
		}
	}

	if err := createLockTableIfNecessary(s3ConfigExtended, s3ConfigExtended.DynamotableTags, terragruntOptions); err != nil {
		return err
	}

	if err := UpdateLockTableSetSSEncryptionOnIfNecessary(&s3Config, s3ConfigExtended, terragruntOptions); err != nil {
		return err
	}

	return nil
}

func (s3Initializer S3Initializer) GetTerraformInitArgs(config map[string]interface{}) map[string]interface{} {
	var filteredConfig = make(map[string]interface{})

	const (
		lockTableKey     = "lock_table"
		dynamoDBTableKey = "dynamodb_table"
	)

	for key, val := range config {
		// Remove attributes that are specific to Terragrunt as
		// Terraform would fail with an error while trying to
		// consume these attributes.
		if util.ListContainsElement(terragruntOnlyConfigs, key) {
			continue
		}

		// Remove the deprecated "lock_table" attribute so that it
		// will not be passed either when generating a backend block
		// or as a command-line argument.
		if key == lockTableKey {
			filteredConfig[dynamoDBTableKey] = val
			continue
		}

		filteredConfig[key] = val
	}

	return filteredConfig
}

// Parse the given map into an extended S3 config
func ParseExtendedS3Config(config map[string]interface{}) (*ExtendedRemoteStateConfigS3, error) {
	var s3Config RemoteStateConfigS3
	var extendedConfig ExtendedRemoteStateConfigS3

	if err := mapstructure.Decode(config, &s3Config); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if err := mapstructure.Decode(config, &extendedConfig); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	_, targetPrefixExists := config["accesslogging_target_prefix"]
	if !targetPrefixExists {
		extendedConfig.AccessLoggingTargetPrefix = DefaultS3BucketAccessLoggingTargetPrefix
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
		terragruntOptions.Logger.Warnf("Encryption is not enabled on the S3 remote state bucket %s. Terraform state files may contain secrets, so we STRONGLY recommend enabling encryption!", config.Bucket)
	}

	return nil
}

// If the bucket specified in the given config doesn't already exist, prompt the user to create it, and if the user
// confirms, create the bucket and enable versioning for it.
func createS3BucketIfNecessary(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if !DoesS3BucketExist(s3Client, &config.remoteStateConfigS3.Bucket) {
		if terragruntOptions.FailIfBucketCreationRequired {
			return BucketCreationNotAllowed(config.remoteStateConfigS3.Bucket)
		}

		prompt := fmt.Sprintf("Remote state S3 bucket %s does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", config.remoteStateConfigS3.Bucket)
		shouldCreateBucket, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
		if err != nil {
			return err
		}

		if shouldCreateBucket {
			// Creating the S3 bucket occasionally fails with eventual consistency errors: e.g., the S3 HeadBucket
			// operation says the bucket exists, but a subsequent call to enable versioning on that bucket fails with
			// the error "NoSuchBucket: The specified bucket does not exist." Therefore, when creating and configuring
			// the S3 bucket, we do so in a retry loop with a sleep between retries that will hopefully work around the
			// eventual consistency issues. Each S3 operation should be idempotent, so redoing steps that have already
			// been performed should be a no-op.
			description := fmt.Sprintf("Create S3 bucket with retry %s", config.remoteStateConfigS3.Bucket)
			maxRetries := 3
			sleepBetweenRetries := 10 * time.Second

			return util.DoWithRetry(description, maxRetries, sleepBetweenRetries, terragruntOptions.Logger, logrus.DebugLevel, func() error {
				return CreateS3BucketWithVersioningSSEncryptionAndAccessLogging(s3Client, config, terragruntOptions)
			})
		}
	}

	return nil
}

func updateS3BucketIfNecessary(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if !DoesS3BucketExist(s3Client, &config.remoteStateConfigS3.Bucket) {
		if terragruntOptions.FailIfBucketCreationRequired {
			return BucketCreationNotAllowed(config.remoteStateConfigS3.Bucket)
		}
		return errors.WithStackTrace(fmt.Errorf("remote state S3 bucket %s does not exist or you don't have permissions to access it", config.remoteStateConfigS3.Bucket))
	}

	needUpdate, bucketUpdatesRequired, err := checkIfS3BucketNeedsUpdate(s3Client, config, terragruntOptions)
	if err != nil {
		return err
	}

	if !needUpdate {
		terragruntOptions.Logger.Debug("S3 bucket is already up to date")
		return nil
	}

	prompt := fmt.Sprintf("Remote state S3 bucket %s is out of date. Would you like Terragrunt to update it?", config.remoteStateConfigS3.Bucket)
	shouldUpdateBucket, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
	if err != nil {
		return err
	}

	if !shouldUpdateBucket {
		return nil
	}

	if bucketUpdatesRequired.Versioning {
		if config.SkipBucketVersioning {
			terragruntOptions.Logger.Debugf("Versioning is disabled for the remote state S3 bucket %s using 'skip_bucket_versioning' config.", config.remoteStateConfigS3.Bucket)
		} else if err := EnableVersioningForS3Bucket(s3Client, &config.remoteStateConfigS3, terragruntOptions); err != nil {
			return err
		}
	}

	if bucketUpdatesRequired.SSEEncryption {
		if config.SkipBucketSSEncryption {
			terragruntOptions.Logger.Debugf("Server-Side Encryption is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_ssencryption' config.", config.remoteStateConfigS3.Bucket)
		} else if err := EnableSSEForS3BucketWide(s3Client, config.remoteStateConfigS3.Bucket, fetchEncryptionAlgorithm(config), config, terragruntOptions); err != nil {
			return err
		}
	}

	if bucketUpdatesRequired.RootAccess {
		if config.SkipBucketRootAccess {
			terragruntOptions.Logger.Debugf("Root access is disabled for the remote state S3 bucket %s using 'skip_bucket_root_access' config.", config.remoteStateConfigS3.Bucket)
		} else if err := EnableRootAccesstoS3Bucket(s3Client, config, terragruntOptions); err != nil {
			return err
		}
	}

	if bucketUpdatesRequired.EnforcedTLS {
		if config.SkipBucketEnforcedTLS {
			terragruntOptions.Logger.Debugf("Enforced TLS is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_enforced_tls' config.", config.remoteStateConfigS3.Bucket)
		} else if err := EnableEnforcedTLSAccesstoS3Bucket(s3Client, config.remoteStateConfigS3.Bucket, config, terragruntOptions); err != nil {
			return err
		}
	}

	if bucketUpdatesRequired.AccessLogging {
		if config.SkipBucketAccessLogging {
			terragruntOptions.Logger.Debugf("Access logging is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_access_logging' config.", config.remoteStateConfigS3.Bucket)
		} else {
			if config.AccessLoggingBucketName != "" {
				terragruntOptions.Logger.Debugf("Enabling bucket-wide Access Logging on AWS S3 bucket %s - using as TargetBucket %s", config.remoteStateConfigS3.Bucket, config.AccessLoggingBucketName)

				if err := CreateLogsS3BucketIfNecessary(s3Client, aws.String(config.AccessLoggingBucketName), terragruntOptions); err != nil {
					terragruntOptions.Logger.Errorf("Could not create logs bucket %s for AWS S3 bucket %s", config.AccessLoggingBucketName, config.remoteStateConfigS3.Bucket)
					return err
				}

				if err := EnablePublicAccessBlockingForS3Bucket(s3Client, config.AccessLoggingBucketName, terragruntOptions); err != nil {
					return err
				}

				if err := EnableAccessLoggingForS3BucketWide(s3Client, &config.remoteStateConfigS3, terragruntOptions, config.AccessLoggingBucketName, config.AccessLoggingTargetPrefix); err != nil {
					return err
				}

				if !config.SkipBucketSSEncryption {
					if err := EnableSSEForS3BucketWide(s3Client, config.AccessLoggingBucketName, s3.ServerSideEncryptionAes256, config, terragruntOptions); err != nil {
						return err
					}
				}

				if err := EnableEnforcedTLSAccesstoS3Bucket(s3Client, config.AccessLoggingBucketName, config, terragruntOptions); err != nil {
					return err
				}

			} else {
				terragruntOptions.Logger.Debugf("Access Logging is disabled for the remote state AWS S3 bucket %s", config.remoteStateConfigS3.Bucket)
			}
		}
	}

	if bucketUpdatesRequired.PublicAccess {
		if config.SkipBucketPublicAccessBlocking {
			terragruntOptions.Logger.Debugf("Public access blocking is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_public_access_blocking' config.", config.remoteStateConfigS3.Bucket)
		} else if err := EnablePublicAccessBlockingForS3Bucket(s3Client, config.remoteStateConfigS3.Bucket, terragruntOptions); err != nil {
			return err
		}
	}

	return nil
}

type S3BucketUpdatesRequired struct {
	Versioning    bool
	SSEEncryption bool
	RootAccess    bool
	EnforcedTLS   bool
	AccessLogging bool
	PublicAccess  bool
}

func checkIfS3BucketNeedsUpdate(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) (bool, S3BucketUpdatesRequired, error) {
	var needUpdate []string
	var configBucket S3BucketUpdatesRequired

	if !config.SkipBucketVersioning {
		enabled, err := checkIfVersioningEnabled(s3Client, &config.remoteStateConfigS3, terragruntOptions)
		if err != nil {
			return false, configBucket, err
		}

		if !enabled {
			configBucket.Versioning = true
			needUpdate = append(needUpdate, "Bucket Versioning")
		}
	}

	if !config.SkipBucketSSEncryption {
		enabled, err := checkIfSSEForS3Enabled(s3Client, config, terragruntOptions)
		if err != nil {
			return false, configBucket, err
		}

		if !enabled {
			configBucket.SSEEncryption = true
			needUpdate = append(needUpdate, "Bucket Server-Side Encryption")
		}
	}

	if !config.SkipBucketRootAccess {
		enabled, err := checkIfBucketRootAccess(s3Client, &config.remoteStateConfigS3, terragruntOptions)
		if err != nil {
			return false, configBucket, err
		}

		if !enabled {
			configBucket.RootAccess = true
			needUpdate = append(needUpdate, "Bucket Root Access")
		}
	}

	if !config.SkipBucketEnforcedTLS {
		enabled, err := checkIfBucketEnforcedTLS(s3Client, &config.remoteStateConfigS3, terragruntOptions)
		if err != nil {
			return false, configBucket, err
		}

		if !enabled {
			configBucket.EnforcedTLS = true
			needUpdate = append(needUpdate, "Bucket Enforced TLS")
		}
	}

	if !config.SkipBucketAccessLogging && config.AccessLoggingBucketName != "" {
		enabled, err := checkIfAccessLoggingForS3Enabled(s3Client, &config.remoteStateConfigS3, terragruntOptions)
		if err != nil {
			return false, configBucket, err
		}

		if !enabled {
			configBucket.AccessLogging = true
			needUpdate = append(needUpdate, "Bucket Access Logging")
		}
	}

	if !config.SkipBucketPublicAccessBlocking {
		enabled, err := checkIfS3PublicAccessBlockingEnabled(s3Client, &config.remoteStateConfigS3, terragruntOptions)
		if err != nil {
			return false, configBucket, err
		}
		if !enabled {
			configBucket.PublicAccess = true
			needUpdate = append(needUpdate, "Bucket Public Access Blocking")
		}
	}

	// show update message if any of the above configs are not set
	if len(needUpdate) > 0 {
		terragruntOptions.Logger.Warnf("The remote state S3 bucket %s needs to be updated:", config.remoteStateConfigS3.Bucket)
		for _, update := range needUpdate {
			terragruntOptions.Logger.Warnf("  - %s", update)
		}

		return true, configBucket, nil
	}

	return false, configBucket, nil
}

// Check if versioning is enabled for the S3 bucket specified in the given config and warn the user if it is not
func checkIfVersioningEnabled(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) (bool, error) {
	out, err := s3Client.GetBucketVersioning(&s3.GetBucketVersioningInput{Bucket: aws.String(config.Bucket)})
	if err != nil {
		return false, errors.WithStackTrace(err)
	}

	// NOTE: There must be a bug in the AWS SDK since out == nil when versioning is not enabled. In the future,
	// check the AWS SDK for updates to see if we can remove "out == nil ||".
	if out == nil || out.Status == nil || *out.Status != s3.BucketVersioningStatusEnabled {
		terragruntOptions.Logger.Warnf("Versioning is not enabled for the remote state S3 bucket %s. We recommend enabling versioning so that you can roll back to previous versions of your Terraform state in case of error.", config.Bucket)
		return false, nil
	}

	return true, nil
}

// Create the given S3 bucket and enable versioning for it
func CreateS3BucketWithVersioningSSEncryptionAndAccessLogging(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	err := CreateS3Bucket(s3Client, aws.String(config.remoteStateConfigS3.Bucket), terragruntOptions)

	if err != nil {
		if isBucketAlreadyOwnedByYouError(err) {
			terragruntOptions.Logger.Debugf("Looks like you're already creating bucket %s at the same time. Will not attempt to create it again.", config.remoteStateConfigS3.Bucket)
			return WaitUntilS3BucketExists(s3Client, &config.remoteStateConfigS3, terragruntOptions)
		}

		return err
	}

	if err := WaitUntilS3BucketExists(s3Client, &config.remoteStateConfigS3, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketRootAccess {
		terragruntOptions.Logger.Debugf("Root access is disabled for the remote state S3 bucket %s using 'skip_bucket_root_access' config.", config.remoteStateConfigS3.Bucket)
	} else if err := EnableRootAccesstoS3Bucket(s3Client, config, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketEnforcedTLS {
		terragruntOptions.Logger.Debugf("TLS enforcement is disabled for the remote state S3 bucket %s using 'skip_bucket_enforced_tls' config.", config.remoteStateConfigS3.Bucket)
	} else if err := EnableEnforcedTLSAccesstoS3Bucket(s3Client, config.remoteStateConfigS3.Bucket, config, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketPublicAccessBlocking {
		terragruntOptions.Logger.Debugf("Public access blocking is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_public_access_blocking' config.", config.remoteStateConfigS3.Bucket)
	} else if err := EnablePublicAccessBlockingForS3Bucket(s3Client, config.remoteStateConfigS3.Bucket, terragruntOptions); err != nil {
		return err
	}

	if err := TagS3Bucket(s3Client, config, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketVersioning {
		terragruntOptions.Logger.Debugf("Versioning is disabled for the remote state S3 bucket %s using 'skip_bucket_versioning' config.", config.remoteStateConfigS3.Bucket)
	} else if err := EnableVersioningForS3Bucket(s3Client, &config.remoteStateConfigS3, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketSSEncryption {
		terragruntOptions.Logger.Debugf("Server-Side Encryption is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_ssencryption' config.", config.remoteStateConfigS3.Bucket)
	} else if err := EnableSSEForS3BucketWide(s3Client, config.remoteStateConfigS3.Bucket, fetchEncryptionAlgorithm(config), config, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketAccessLogging {
		terragruntOptions.Logger.Warnf("Terragrunt configuration option 'skip_bucket_accesslogging' is now deprecated. Access logging for the state bucket %s is disabled by default. To enable access logging for bucket %s, please provide property `accesslogging_bucket_name` in the terragrunt config file. For more details, please refer to the Terragrunt documentation.", config.remoteStateConfigS3.Bucket, config.remoteStateConfigS3.Bucket)
	}

	if config.AccessLoggingBucketName != "" {
		terragruntOptions.Logger.Debugf("Enabling bucket-wide Access Logging on AWS S3 bucket %s - using as TargetBucket %s", config.remoteStateConfigS3.Bucket, config.AccessLoggingBucketName)

		if err := CreateLogsS3BucketIfNecessary(s3Client, aws.String(config.AccessLoggingBucketName), terragruntOptions); err != nil {
			terragruntOptions.Logger.Errorf("Could not create logs bucket %s for AWS S3 bucket %s", config.AccessLoggingBucketName, config.remoteStateConfigS3.Bucket)
			return err
		}

		if err := EnablePublicAccessBlockingForS3Bucket(s3Client, config.AccessLoggingBucketName, terragruntOptions); err != nil {
			return err
		}

		if err := EnableAccessLoggingForS3BucketWide(s3Client, &config.remoteStateConfigS3, terragruntOptions, config.AccessLoggingBucketName, config.AccessLoggingTargetPrefix); err != nil {
			return err
		}

		if !config.SkipBucketSSEncryption {
			if err := EnableSSEForS3BucketWide(s3Client, config.AccessLoggingBucketName, s3.ServerSideEncryptionAes256, config, terragruntOptions); err != nil {
				return err
			}
		}

		if err := EnableEnforcedTLSAccesstoS3Bucket(s3Client, config.AccessLoggingBucketName, config, terragruntOptions); err != nil {
			return err
		}
	} else {
		terragruntOptions.Logger.Debugf("Access Logging is disabled for the remote state AWS S3 bucket %s", config.remoteStateConfigS3.Bucket)
	}

	if err := TagS3BucketAccessLogging(s3Client, config, terragruntOptions); err != nil {
		return err
	}

	return nil
}

func CreateLogsS3BucketIfNecessary(s3Client *s3.S3, logsBucketName *string, terragruntOptions *options.TerragruntOptions) error {
	if !DoesS3BucketExist(s3Client, logsBucketName) {
		if terragruntOptions.FailIfBucketCreationRequired {
			return BucketCreationNotAllowed(*logsBucketName)
		}
		prompt := fmt.Sprintf("Logs S3 bucket %s for the remote state does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", *logsBucketName)
		shouldCreateBucket, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
		if err != nil {
			return err
		}

		if shouldCreateBucket {
			return CreateS3Bucket(s3Client, logsBucketName, terragruntOptions)
		}
	}
	return nil
}

func TagS3BucketAccessLogging(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {

	if config.AccessLoggingBucketTags == nil || len(config.AccessLoggingBucketTags) == 0 {
		terragruntOptions.Logger.Debugf("No tags specified for bucket %s.", config.AccessLoggingBucketName)
		return nil
	}

	// There must be one entry in the list
	var tagsConverted = convertTags(config.AccessLoggingBucketTags)

	terragruntOptions.Logger.Debugf("Tagging S3 bucket with %s", config.AccessLoggingBucketTags)

	putBucketTaggingInput := s3.PutBucketTaggingInput{
		Bucket: aws.String(config.AccessLoggingBucketName),
		Tagging: &s3.Tagging{
			TagSet: tagsConverted,
		},
	}

	_, err := s3Client.PutBucketTagging(&putBucketTaggingInput)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Debugf("Tagged S3 bucket with %s", config.AccessLoggingBucketTags)
	return nil
}

func TagS3Bucket(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {

	if config.S3BucketTags == nil || len(config.S3BucketTags) == 0 {
		terragruntOptions.Logger.Debugf("No tags specified for bucket %s.", config.remoteStateConfigS3.Bucket)
		return nil
	}

	// There must be one entry in the list
	var tagsConverted = convertTags(config.S3BucketTags)

	terragruntOptions.Logger.Debugf("Tagging S3 bucket with %s", config.S3BucketTags)

	putBucketTaggingInput := s3.PutBucketTaggingInput{
		Bucket: aws.String(config.remoteStateConfigS3.Bucket),
		Tagging: &s3.Tagging{
			TagSet: tagsConverted,
		},
	}

	_, err := s3Client.PutBucketTagging(&putBucketTaggingInput)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Debugf("Tagged S3 bucket with %s", config.S3BucketTags)
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
	terragruntOptions.Logger.Debugf("Waiting for bucket %s to be created", config.Bucket)
	for retries := 0; retries < MAX_RETRIES_WAITING_FOR_S3_BUCKET; retries++ {
		if DoesS3BucketExist(s3Client, aws.String(config.Bucket)) {
			terragruntOptions.Logger.Debugf("S3 bucket %s created.", config.Bucket)
			return nil
		} else if retries < MAX_RETRIES_WAITING_FOR_S3_BUCKET-1 {
			terragruntOptions.Logger.Debugf("S3 bucket %s has not been created yet. Sleeping for %s and will check again.", config.Bucket, SLEEP_BETWEEN_RETRIES_WAITING_FOR_S3_BUCKET)
			time.Sleep(SLEEP_BETWEEN_RETRIES_WAITING_FOR_S3_BUCKET)
		}
	}

	return errors.WithStackTrace(MaxRetriesWaitingForS3BucketExceeded(config.Bucket))
}

// Create the S3 bucket specified in the given config
func CreateS3Bucket(s3Client *s3.S3, bucket *string, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Creating S3 bucket %s", aws.StringValue(bucket))
	// https://github.com/aws/aws-sdk-go/blob/v1.44.245/service/s3/api.go#L41760
	_, err := s3Client.CreateBucket(&s3.CreateBucketInput{Bucket: bucket, ObjectOwnership: aws.String("ObjectWriter")})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	terragruntOptions.Logger.Debugf("Created S3 bucket %s", aws.StringValue(bucket))
	return nil
}

// Determine if this is an error that implies you've already made a request to create the S3 bucket and it succeeded
// or is in progress. This usually happens when running many tests in parallel or xxx-all commands.
func isBucketAlreadyOwnedByYouError(err error) bool {
	awsErr, isAwsErr := errors.Unwrap(err).(awserr.Error)
	return isAwsErr && (awsErr.Code() == "BucketAlreadyOwnedByYou" || awsErr.Code() == "OperationAborted")
}

// Add a policy to allow root access to the bucket
func EnableRootAccesstoS3Bucket(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	bucket := config.remoteStateConfigS3.Bucket
	terragruntOptions.Logger.Debugf("Enabling root access to S3 bucket %s", bucket)

	accountID, err := aws_helper.GetAWSAccountID(config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	partition, err := aws_helper.GetAWSPartition(config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	var policyInBucket aws_helper.Policy
	policyOutput, err := s3Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(bucket),
	})
	// If there's no policy, we need to create one
	if err != nil {
		terragruntOptions.Logger.Debugf("Policy not exists for bucket %s", bucket)
	}

	if policyOutput.Policy != nil {
		terragruntOptions.Logger.Debugf("Policy already exists for bucket %s", bucket)
		policyInBucket, err = aws_helper.UnmarshalPolicy(*policyOutput.Policy)
		if err != nil {
			return errors.WithStackTrace(err)
		}
	}

	for _, statement := range policyInBucket.Statement {
		if statement.Sid == SidRootPolicy {
			terragruntOptions.Logger.Debugf("Policy for RootAccess already exists for bucket %s", bucket)
			return nil
		}
	}

	rootS3Policy := aws_helper.Policy{
		Version: "2012-10-17",
		Statement: []aws_helper.Statement{
			{
				Sid:    SidRootPolicy,
				Effect: "Allow",
				Action: "s3:*",
				Resource: []string{
					"arn:" + partition + ":s3:::" + bucket,
					"arn:" + partition + ":s3:::" + bucket + "/*",
				},
				Principal: map[string][]string{
					"AWS": {
						"arn:" + partition + ":iam::" + accountID + ":root",
					},
				},
			},
		},
	}

	// Append the root s3 policy to the existing policy in the bucket
	rootS3Policy.Statement = append(rootS3Policy.Statement, policyInBucket.Statement...)
	policy, err := aws_helper.MarshalPolicy(rootS3Policy)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	_, err = s3Client.PutBucketPolicy(&s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(string(policy)),
	})
	if err != nil {
		return errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Debugf("Enabled root access to bucket %s", bucket)
	return nil
}

// Helper function to check if the root access policy is enabled for the bucket
func checkIfBucketRootAccess(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) (bool, error) {
	terragruntOptions.Logger.Debugf("Checking if bucket %s is have root access", config.Bucket)

	policyOutput, err := s3Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(config.Bucket),
	})
	if err != nil {
		terragruntOptions.Logger.Debugf("Could not get policy for bucket %s", config.Bucket)
		return false, nil
	}

	// If the bucket has no policy, it is not enforced
	if policyOutput == nil {
		return true, nil
	}

	policyInBucket, err := aws_helper.UnmarshalPolicy(*policyOutput.Policy)
	if err != nil {
		return false, errors.WithStackTrace(err)
	}

	for _, statement := range policyInBucket.Statement {
		if statement.Sid == SidRootPolicy {
			terragruntOptions.Logger.Debugf("Policy for RootAccess already exists for bucket %s", config.Bucket)
			return true, nil
		}
	}

	terragruntOptions.Logger.Debugf("Root access to bucket %s is not enabled", config.Bucket)
	return false, nil
}

// Add a policy to enforce TLS based access to the bucket
func EnableEnforcedTLSAccesstoS3Bucket(s3Client *s3.S3, bucket string, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Enabling enforced TLS access for S3 bucket %s", bucket)

	partition, err := aws_helper.GetAWSPartition(config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	var policyInBucket aws_helper.Policy
	policyOutput, err := s3Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(bucket),
	})
	// If there's no policy, we need to create one
	if err != nil {
		terragruntOptions.Logger.Debugf("Policy not exists for bucket %s", bucket)
	}

	if policyOutput.Policy != nil {
		terragruntOptions.Logger.Debugf("Policy already exists for bucket %s", bucket)
		policyInBucket, err = aws_helper.UnmarshalPolicy(*policyOutput.Policy)
		if err != nil {
			return errors.WithStackTrace(err)
		}
	}

	for _, statement := range policyInBucket.Statement {
		if statement.Sid == SidEnforcedTLSPolicy {
			terragruntOptions.Logger.Debugf("Policy for EnforceTLS already exists for bucket %s", bucket)
			return nil
		}
	}

	tlsS3Policy := aws_helper.Policy{
		Version: "2012-10-17",
		Statement: []aws_helper.Statement{
			{
				Sid:       SidEnforcedTLSPolicy,
				Effect:    "Deny",
				Action:    "s3:*",
				Principal: "*",
				Resource: []string{
					"arn:" + partition + ":s3:::" + bucket,
					"arn:" + partition + ":s3:::" + bucket + "/*",
				},
				Condition: &map[string]interface{}{
					"Bool": map[string]interface{}{
						"aws:SecureTransport": "false",
					},
				},
			},
		},
	}

	// Append the root s3 policy to the existing policy in the bucket
	tlsS3Policy.Statement = append(tlsS3Policy.Statement, policyInBucket.Statement...)
	policy, err := aws_helper.MarshalPolicy(tlsS3Policy)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	_, err = s3Client.PutBucketPolicy(&s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(string(policy)),
	})
	if err != nil {
		return errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Debugf("Enabled enforced TLS access for bucket %s", bucket)
	return nil
}

// Helper function to check if the enforced TLS policy is enabled for the bucket
func checkIfBucketEnforcedTLS(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) (bool, error) {
	terragruntOptions.Logger.Debugf("Checking if bucket %s is enforced with TLS", config.Bucket)

	policyOutput, err := s3Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(config.Bucket),
	})
	if err != nil {
		// S3 API error codes:
		// http://docs.aws.amazon.com/AmazonS3/latest/API/ErrorResponses.html
		if aerr, ok := err.(awserr.Error); ok {
			// Enforced TLS policy if is not found bucket policy
			if aerr.Code() == "NoSuchBucketPolicy" {
				terragruntOptions.Logger.Debugf("Could not get policy for bucket %s", config.Bucket)
				return false, nil
			}
		}

		return false, errors.WithStackTrace(err)
	}

	if policyOutput.Policy == nil {
		return true, nil
	}

	policyInBucket, err := aws_helper.UnmarshalPolicy(*policyOutput.Policy)
	if err != nil {
		return false, errors.WithStackTrace(err)
	}

	for _, statement := range policyInBucket.Statement {
		if statement.Sid == SidEnforcedTLSPolicy {
			terragruntOptions.Logger.Debugf("Policy for EnforcedTLS already exists for bucket %s", config.Bucket)
			return true, nil
		}
	}

	terragruntOptions.Logger.Debugf("Bucket %s is not enforced with TLS Policy", config.Bucket)
	return false, nil
}

// Enable versioning for the S3 bucket specified in the given config
func EnableVersioningForS3Bucket(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Enabling versioning on S3 bucket %s", config.Bucket)
	input := s3.PutBucketVersioningInput{
		Bucket:                  aws.String(config.Bucket),
		VersioningConfiguration: &s3.VersioningConfiguration{Status: aws.String(s3.BucketVersioningStatusEnabled)},
	}

	_, err := s3Client.PutBucketVersioning(&input)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Debugf("Enabled versioning on S3 bucket %s", config.Bucket)
	return nil
}

// Enable bucket-wide Server-Side Encryption for the AWS S3 bucket specified in the given config
func EnableSSEForS3BucketWide(s3Client *s3.S3, bucketName string, algorithm string, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Enabling bucket-wide SSE on AWS S3 bucket %s", bucketName)

	accountID, err := aws_helper.GetAWSAccountID(config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	partition, err := aws_helper.GetAWSPartition(config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	defEnc := &s3.ServerSideEncryptionByDefault{
		SSEAlgorithm: aws.String(algorithm),
	}
	if algorithm == s3.ServerSideEncryptionAwsKms && config.BucketSSEKMSKeyID != "" {
		defEnc.KMSMasterKeyID = aws.String(config.BucketSSEKMSKeyID)
	} else if algorithm == s3.ServerSideEncryptionAwsKms {
		kmsKeyID := fmt.Sprintf("arn:%s:kms:%s:%s:alias/aws/s3", partition, config.remoteStateConfigS3.Region, accountID)
		defEnc.KMSMasterKeyID = aws.String(kmsKeyID)
	}

	rule := &s3.ServerSideEncryptionRule{ApplyServerSideEncryptionByDefault: defEnc}
	rules := []*s3.ServerSideEncryptionRule{rule}
	serverConfig := &s3.ServerSideEncryptionConfiguration{Rules: rules}
	input := &s3.PutBucketEncryptionInput{Bucket: aws.String(bucketName), ServerSideEncryptionConfiguration: serverConfig}

	_, err = s3Client.PutBucketEncryption(input)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Debugf("Enabled bucket-wide SSE on AWS S3 bucket %s", bucketName)
	return nil
}

func fetchEncryptionAlgorithm(config *ExtendedRemoteStateConfigS3) string {
	// Encrypt with KMS by default
	algorithm := s3.ServerSideEncryptionAwsKms
	if config.BucketSSEAlgorithm != "" {
		algorithm = config.BucketSSEAlgorithm
	}
	return algorithm
}

func checkIfSSEForS3Enabled(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) (bool, error) {
	terragruntOptions.Logger.Debugf("Checking if SSE is enabled for AWS S3 bucket %s", config.remoteStateConfigS3.Bucket)

	input := &s3.GetBucketEncryptionInput{Bucket: aws.String(config.remoteStateConfigS3.Bucket)}
	output, err := s3Client.GetBucketEncryption(input)
	if err != nil {
		terragruntOptions.Logger.Debugf("Error checking if SSE is enabled for AWS S3 bucket %s: %s", config.remoteStateConfigS3.Bucket, err.Error())
		return false, nil
	}

	if output.ServerSideEncryptionConfiguration == nil {
		return false, nil
	}

	for _, rule := range output.ServerSideEncryptionConfiguration.Rules {
		if rule.ApplyServerSideEncryptionByDefault != nil {
			if rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm != nil {
				if *rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm == fetchEncryptionAlgorithm(config) {
					return true, nil
				}

				return false, nil
			}
		}
	}

	return false, nil
}

// Enable bucket-wide Access Logging for the AWS S3 bucket specified in the given config
func EnableAccessLoggingForS3BucketWide(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions, logsBucket string, logsBucketPrefix string) error {
	if err := configureBucketAccessLoggingAcl(s3Client, aws.String(logsBucket), terragruntOptions); err != nil {
		return errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Debugf("Putting bucket logging on S3 bucket %s with TargetBucket %s and TargetPrefix %s", config.Bucket, logsBucket, logsBucketPrefix)

	loggingInput := s3.PutBucketLoggingInput{
		Bucket: aws.String(config.Bucket),
		BucketLoggingStatus: &s3.BucketLoggingStatus{
			LoggingEnabled: &s3.LoggingEnabled{
				TargetBucket: aws.String(logsBucket),
				TargetPrefix: aws.String(logsBucketPrefix),
			},
		},
	}

	if _, err := s3Client.PutBucketLogging(&loggingInput); err != nil {
		return errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Debugf("Enabled bucket-wide Access Logging on AWS S3 bucket %s", config.Bucket)
	return nil
}

func checkIfAccessLoggingForS3Enabled(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) (bool, error) {
	terragruntOptions.Logger.Debugf("Checking if Access Logging is enabled for AWS S3 bucket %s", config.Bucket)

	input := &s3.GetBucketLoggingInput{Bucket: aws.String(config.Bucket)}
	output, err := s3Client.GetBucketLogging(input)
	if err != nil {
		terragruntOptions.Logger.Debugf("Error checking if Access Logging is enabled for AWS S3 bucket %s: %s", config.Bucket, err.Error())
		return false, nil
	}

	if output.LoggingEnabled == nil {
		return false, nil
	}

	return true, nil
}

// Block all public access policies on the bucket and objects. These settings ensure that a misconfiguration of the
// bucket or objects will not accidentally enable public access to those items. See
// https://docs.aws.amazon.com/AmazonS3/latest/dev/access-control-block-public-access.html for more information.
func EnablePublicAccessBlockingForS3Bucket(s3Client *s3.S3, bucketName string, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Blocking all public access to S3 bucket %s", bucketName)
	_, err := s3Client.PutPublicAccessBlock(
		&s3.PutPublicAccessBlockInput{
			Bucket: aws.String(bucketName),
			PublicAccessBlockConfiguration: &s3.PublicAccessBlockConfiguration{
				BlockPublicAcls:       aws.Bool(true),
				BlockPublicPolicy:     aws.Bool(true),
				IgnorePublicAcls:      aws.Bool(true),
				RestrictPublicBuckets: aws.Bool(true),
			},
		},
	)

	if err != nil {
		return errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Debugf("Blocked all public access to S3 bucket %s", bucketName)
	return nil
}

func checkIfS3PublicAccessBlockingEnabled(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) (bool, error) {
	terragruntOptions.Logger.Debugf("Checking if S3 bucket %s is configured to block public access", config.Bucket)
	output, err := s3Client.GetPublicAccessBlock(&s3.GetPublicAccessBlockInput{
		Bucket: aws.String(config.Bucket),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			// Enforced block public access if is not found bucket policy
			if aerr.Code() == "NoSuchPublicAccessBlockConfiguration" {
				terragruntOptions.Logger.Debugf("Could not get public access block for bucket %s", config.Bucket)
				return false, nil
			}
		}

		return false, errors.WithStackTrace(err)
	}

	return validatePublicAccessBlock(output)
}

func validatePublicAccessBlock(output *s3.GetPublicAccessBlockOutput) (bool, error) {
	if output.PublicAccessBlockConfiguration == nil {
		return false, nil
	}

	if !aws.BoolValue(output.PublicAccessBlockConfiguration.BlockPublicAcls) {
		return false, nil
	}
	if !aws.BoolValue(output.PublicAccessBlockConfiguration.BlockPublicAcls) {
		return false, nil
	}
	if !aws.BoolValue(output.PublicAccessBlockConfiguration.BlockPublicAcls) {
		return false, nil
	}
	if !aws.BoolValue(output.PublicAccessBlockConfiguration.BlockPublicAcls) {
		return false, nil
	}

	return true, nil
}

// To enable access logging in an S3 bucket, you must grant WRITE and READ_ACP permissions to the Log Delivery
// Group. For more info, see:
// https://docs.aws.amazon.com/AmazonS3/latest/dev/enable-logging-programming.html
func configureBucketAccessLoggingAcl(s3Client *s3.S3, bucket *string, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Granting WRITE and READ_ACP permissions to S3 Log Delivery (%s) for bucket %s. This is required for access logging.", s3LogDeliveryGranteeUri, aws.StringValue(bucket))

	uri := fmt.Sprintf("uri=%s", s3LogDeliveryGranteeUri)
	aclInput := s3.PutBucketAclInput{
		Bucket:       bucket,
		GrantWrite:   aws.String(uri),
		GrantReadACP: aws.String(uri),
	}

	if _, err := s3Client.PutBucketAcl(&aclInput); err != nil {
		return errors.WithStackTrace(err)
	}

	return waitUntilBucketHasAccessLoggingAcl(s3Client, bucket, terragruntOptions)
}

func waitUntilBucketHasAccessLoggingAcl(s3Client *s3.S3, bucket *string, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Waiting for ACL bucket %s to have the updated ACL for access logging.", aws.StringValue(bucket))

	maxRetries := 10
	timeBetweenRetries := 5 * time.Second

	for i := 0; i < maxRetries; i++ {
		out, err := s3Client.GetBucketAcl(&s3.GetBucketAclInput{Bucket: bucket})
		if err != nil {
			return errors.WithStackTrace(err)
		}

		hasReadAcp := false
		hasWrite := false

		for _, grant := range out.Grants {
			if aws.StringValue(grant.Grantee.URI) == s3LogDeliveryGranteeUri {
				if aws.StringValue(grant.Permission) == s3.PermissionReadAcp {
					hasReadAcp = true
				}
				if aws.StringValue(grant.Permission) == s3.PermissionWrite {
					hasWrite = true
				}
			}
		}

		if hasReadAcp && hasWrite {
			terragruntOptions.Logger.Debugf("Bucket %s now has the proper ACL permissions for access logging!", aws.StringValue(bucket))
			return nil
		}

		terragruntOptions.Logger.Debugf("Bucket %s still does not have the ACL permissions for access logging. Will sleep for %v and check again.", aws.StringValue(bucket), timeBetweenRetries)
		time.Sleep(timeBetweenRetries)
	}

	return errors.WithStackTrace(MaxRetriesWaitingForS3ACLExceeded(aws.StringValue(bucket)))
}

// Returns true if the S3 bucket specified in the given config exists and the current user has the ability to access
// it.
func DoesS3BucketExist(s3Client *s3.S3, bucket *string) bool {
	_, err := s3Client.HeadBucket(&s3.HeadBucketInput{Bucket: bucket})
	return err == nil
}

// Create a table for locks in DynamoDB if the user has configured a lock table and the table doesn't already exist
func createLockTableIfNecessary(extendedS3Config *ExtendedRemoteStateConfigS3, tags map[string]string, terragruntOptions *options.TerragruntOptions) error {

	if extendedS3Config.remoteStateConfigS3.GetLockTableName() == "" {
		return nil
	}

	dynamodbClient, err := dynamodb.CreateDynamoDbClient(extendedS3Config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return err
	}

	return dynamodb.CreateLockTableIfNecessary(extendedS3Config.remoteStateConfigS3.GetLockTableName(), tags, dynamodbClient, terragruntOptions)
}

// Update a table for locks in DynamoDB if the user has configured a lock table and the table's server-side encryption isn't turned on
func UpdateLockTableSetSSEncryptionOnIfNecessary(s3Config *RemoteStateConfigS3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if !config.EnableLockTableSSEncryption {
		return nil
	}

	if s3Config.GetLockTableName() == "" {
		return nil
	}

	dynamodbClient, err := dynamodb.CreateDynamoDbClient(config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return err
	}

	return dynamodb.UpdateLockTableSetSSEncryptionOnIfNecessary(s3Config.GetLockTableName(), dynamodbClient, terragruntOptions)
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

type MaxRetriesWaitingForS3ACLExceeded string

func (err MaxRetriesWaitingForS3ACLExceeded) Error() string {
	return fmt.Sprintf("Exceeded max retries waiting for bucket S3 bucket %s to have proper ACL for access logging", string(err))
}

type InvalidAccessLoggingBucketEncryption struct {
	BucketSSEAlgorithm string
}

func (err InvalidAccessLoggingBucketEncryption) Error() string {
	return fmt.Sprintf("Encryption algorithm %s is not supported for access logging bucket. Please use AES256", err.BucketSSEAlgorithm)
}
