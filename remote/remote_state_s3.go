package remote

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gruntwork-io/terragrunt/awshelper"
	"github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/mitchellh/mapstructure"
)

const (
	lockTableDeprecationMessage              = "Remote state configuration 'lock_table' attribute is deprecated; use 'dynamodb_table' instead."
	DefaultS3BucketAccessLoggingTargetPrefix = "TFStateLogs/"
	SidRootPolicy                            = "RootAccess"
	SidEnforcedTLSPolicy                     = "EnforcedTLS"

	s3TimeBetweenRetries  = 5 * time.Second
	s3MaxRetries          = 3
	s3SleepBetweenRetries = 10 * time.Second
)

/* ExtendedRemoteStateConfigS3 is a struct that contains the RemoteStateConfigS3 struct and additional
 * configuration options that are specific to the S3 backend. This struct is used to parse the configuration
 * from the Terragrunt configuration file.
 *
 * We use this construct to separate the three config keys 's3_bucket_tags', 'dynamodb_table_tags'
 * and 'accesslogging_bucket_tags' from the others, as they are specific to the s3 backend,
 * but only used by terragrunt to tag the s3 bucket, the dynamo db and the s3 bucket used to the
 * access logs, in case it has to create them.
 */
type ExtendedRemoteStateConfigS3 struct {
	RemoteStateConfigS3 RemoteStateConfigS3 `mapstructure:",squash"`

	S3BucketTags                                 map[string]string `mapstructure:"s3_bucket_tags"`
	DynamotableTags                              map[string]string `mapstructure:"dynamodb_table_tags"`
	AccessLoggingBucketTags                      map[string]string `mapstructure:"accesslogging_bucket_tags"`
	SkipCredentialsValidation                    bool              `mapstructure:"skip_credentials_validation"`
	SkipBucketVersioning                         bool              `mapstructure:"skip_bucket_versioning"`
	SkipBucketSSEncryption                       bool              `mapstructure:"skip_bucket_ssencryption"`
	SkipBucketAccessLogging                      bool              `mapstructure:"skip_bucket_accesslogging"`
	SkipBucketRootAccess                         bool              `mapstructure:"skip_bucket_root_access"`
	SkipBucketEnforcedTLS                        bool              `mapstructure:"skip_bucket_enforced_tls"`
	SkipBucketPublicAccessBlocking               bool              `mapstructure:"skip_bucket_public_access_blocking"`
	DisableBucketUpdate                          bool              `mapstructure:"disable_bucket_update"`
	EnableLockTableSSEncryption                  bool              `mapstructure:"enable_lock_table_ssencryption"`
	DisableAWSClientChecksums                    bool              `mapstructure:"disable_aws_client_checksums"`
	AccessLoggingBucketName                      string            `mapstructure:"accesslogging_bucket_name"`
	AccessLoggingTargetObjectPartitionDateSource string            `mapstructure:"accesslogging_target_object_partition_date_source"`
	AccessLoggingTargetPrefix                    string            `mapstructure:"accesslogging_target_prefix"`
	SkipAccessLoggingBucketACL                   bool              `mapstructure:"skip_accesslogging_bucket_acl"`
	SkipAccessLoggingBucketEnforcedTLS           bool              `mapstructure:"skip_accesslogging_bucket_enforced_tls"`
	SkipAccessLoggingBucketPublicAccessBlocking  bool              `mapstructure:"skip_accesslogging_bucket_public_access_blocking"`
	SkipAccessLoggingBucketSSEncryption          bool              `mapstructure:"skip_accesslogging_bucket_ssencryption"`
	BucketSSEAlgorithm                           string            `mapstructure:"bucket_sse_algorithm"`
	BucketSSEKMSKeyID                            string            `mapstructure:"bucket_sse_kms_key_id"`
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
	"accesslogging_target_object_partition_date_source",
	"accesslogging_target_prefix",
	"skip_accesslogging_bucket_acl",
	"skip_accesslogging_bucket_enforced_tls",
	"skip_accesslogging_bucket_public_access_blocking",
	"skip_accesslogging_bucket_ssencryption",
	"bucket_sse_algorithm",
	"bucket_sse_kms_key_id",
}

type RemoteStateConfigS3AssumeRole struct {
	RoleArn     string `mapstructure:"role_arn"`
	ExternalID  string `mapstructure:"external_id"`
	SessionName string `mapstructure:"session_name"`
}

type RemoteStateConfigS3Endpoints struct {
	S3       string `mapstructure:"s3"`
	DynamoDB string `mapstructure:"dynamodb"`
}

// RemoteStateConfigS3 is a representation of the
// configuration options available for S3 remote state.
type RemoteStateConfigS3 struct {
	Encrypt          bool                          `mapstructure:"encrypt"`
	Bucket           string                        `mapstructure:"bucket"`
	Key              string                        `mapstructure:"key"`
	Region           string                        `mapstructure:"region"`
	Endpoint         string                        `mapstructure:"endpoint"`          // Deprecated in Terraform version 1.6 or newer.
	DynamoDBEndpoint string                        `mapstructure:"dynamodb_endpoint"` // Deprecated in Terraform version 1.6 or newer.
	Endpoints        RemoteStateConfigS3Endpoints  `mapstructure:"endpoints"`
	Profile          string                        `mapstructure:"profile"`
	RoleArn          string                        `mapstructure:"role_arn"`     // Deprecated in Terraform version 1.6 or newer.
	ExternalID       string                        `mapstructure:"external_id"`  // Deprecated in Terraform version 1.6 or newer.
	SessionName      string                        `mapstructure:"session_name"` // Deprecated in Terraform version 1.6 or newer.
	LockTable        string                        `mapstructure:"lock_table"`   // Deprecated in Terraform version 0.13 or newer.
	DynamoDBTable    string                        `mapstructure:"dynamodb_table"`
	CredsFilename    string                        `mapstructure:"shared_credentials_file"`
	S3ForcePathStyle bool                          `mapstructure:"force_path_style"`
	AssumeRole       RemoteStateConfigS3AssumeRole `mapstructure:"assume_role"`
}

// GetAwsSessionConfig builds a session config for AWS related requests
// from the RemoteStateConfigS3 configuration.
func (c *ExtendedRemoteStateConfigS3) GetAwsSessionConfig() *awshelper.AwsSessionConfig {
	s3Endpoint := c.RemoteStateConfigS3.Endpoint
	if c.RemoteStateConfigS3.Endpoints.S3 != "" {
		s3Endpoint = c.RemoteStateConfigS3.Endpoints.S3
	}

	dynamoDBEndpoint := c.RemoteStateConfigS3.DynamoDBEndpoint
	if c.RemoteStateConfigS3.Endpoints.DynamoDB != "" {
		dynamoDBEndpoint = c.RemoteStateConfigS3.Endpoints.DynamoDB
	}

	return &awshelper.AwsSessionConfig{
		Region:                  c.RemoteStateConfigS3.Region,
		CustomS3Endpoint:        s3Endpoint,
		CustomDynamoDBEndpoint:  dynamoDBEndpoint,
		Profile:                 c.RemoteStateConfigS3.Profile,
		RoleArn:                 c.RemoteStateConfigS3.GetSessionRoleArn(),
		ExternalID:              c.RemoteStateConfigS3.GetExternalID(),
		SessionName:             c.RemoteStateConfigS3.GetSessionName(),
		CredsFilename:           c.RemoteStateConfigS3.CredsFilename,
		S3ForcePathStyle:        c.RemoteStateConfigS3.S3ForcePathStyle,
		DisableComputeChecksums: c.DisableAWSClientChecksums,
	}
}

// CreateS3LoggingInput builds AWS S3 logging input struct from the configuration.
func (c *ExtendedRemoteStateConfigS3) CreateS3LoggingInput() s3.PutBucketLoggingInput {
	loggingInput := s3.PutBucketLoggingInput{
		Bucket: aws.String(c.RemoteStateConfigS3.Bucket),
		BucketLoggingStatus: &s3.BucketLoggingStatus{
			LoggingEnabled: &s3.LoggingEnabled{
				TargetBucket: aws.String(c.AccessLoggingBucketName),
			},
		},
	}

	if c.AccessLoggingTargetPrefix != "" {
		loggingInput.BucketLoggingStatus.LoggingEnabled.TargetPrefix = aws.String(c.AccessLoggingTargetPrefix)
	}

	if c.AccessLoggingTargetObjectPartitionDateSource != "" {
		loggingInput.BucketLoggingStatus.LoggingEnabled.TargetObjectKeyFormat = &s3.TargetObjectKeyFormat{
			PartitionedPrefix: &s3.PartitionedPrefix{
				PartitionDateSource: aws.String(c.AccessLoggingTargetObjectPartitionDateSource),
			},
		}
	}

	return loggingInput
}

// GetLockTableName returns the name of the DynamoDB table used for locking.
//
// The DynamoDB lock table attribute used to be called "lock_table", but has since been renamed to "dynamodb_table", and
// the old attribute name deprecated. The old attribute name has been eventually removed from Terraform starting with
// release 0.13. To maintain backwards compatibility, we support both names.
func (s3Config *RemoteStateConfigS3) GetLockTableName() string {
	if s3Config.DynamoDBTable != "" {
		return s3Config.DynamoDBTable
	}

	return s3Config.LockTable
}

// GetSessionRoleArn returns the role defined in the AssumeRole struct
// or fallback to the top level argument deprecated in Terraform 1.6
func (s3Config *RemoteStateConfigS3) GetSessionRoleArn() string {
	if s3Config.AssumeRole.RoleArn != "" {
		return s3Config.AssumeRole.RoleArn
	}

	return s3Config.RoleArn
}

// GetExternalID returns the external ID defined in the AssumeRole struct
// or fallback to the top level argument deprecated in Terraform 1.6
// The external ID is used to prevent confused deputy attacks.
func (s3Config *RemoteStateConfigS3) GetExternalID() string {
	if s3Config.AssumeRole.ExternalID != "" {
		return s3Config.AssumeRole.ExternalID
	}

	return s3Config.ExternalID
}

func (s3Config *RemoteStateConfigS3) GetSessionName() string {
	if s3Config.AssumeRole.SessionName != "" {
		return s3Config.AssumeRole.SessionName
	}

	return s3Config.SessionName
}

const MaxRetriesWaitingForS3Bucket = 12
const SleepBetweenRetriesWaitingForS3Bucket = 5 * time.Second

// To enable access logging in an S3 bucket, you must grant WRITE and READ_ACP permissions to the Log Delivery Group,
// which is represented by the following URI. For more info, see:
// https://docs.aws.amazon.com/AmazonS3/latest/dev/enable-logging-programming.html
const s3LogDeliveryGranteeURI = "http://acs.amazonaws.com/groups/s3/LogDelivery"

type S3Initializer struct{}

// NeedsInitialization returns true if the remote state S3 bucket specified in the given config needs to be initialized.
//
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

	if !ConfigValuesEqual(remoteState.Config, existingBackend, terragruntOptions) {
		return true, nil
	}

	s3ConfigExtended, err := ParseExtendedS3Config(remoteState.Config)
	if err != nil {
		return false, err
	}

	s3Config := s3ConfigExtended.RemoteStateConfigS3

	sessionConfig := s3ConfigExtended.GetAwsSessionConfig()

	// Validate current AWS session before checking S3
	if !s3ConfigExtended.SkipCredentialsValidation {
		if err = awshelper.ValidateAwsSession(sessionConfig, terragruntOptions); err != nil {
			return false, err
		}
	}

	s3Client, err := CreateS3Client(sessionConfig, terragruntOptions)
	if err != nil {
		return false, err
	}

	if !DoesS3BucketExist(s3Client, &s3Config.Bucket) {
		return true, nil
	}

	if s3Config.GetLockTableName() != "" {
		dynamodbClient, err := dynamodb.CreateDynamoDBClient(sessionConfig, terragruntOptions)
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

// ConfigValuesEqual returns true if the given config is in any way different than what is configured for the backend
func ConfigValuesEqual(config map[string]interface{}, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) bool {
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

// buildInitializerCacheKey returns a unique key for the given S3 config that can be used to cache the initialization
func (s3Initializer S3Initializer) buildInitializerCacheKey(
	s3Config *RemoteStateConfigS3,
) string {
	return fmt.Sprintf(
		"%s-%s-%s-%s",
		s3Config.Bucket,
		s3Config.Region,
		s3Config.LockTable,
		s3Config.DynamoDBTable,
	)
}

// Initialize the remote state S3 bucket specified in the given config. This function will validate the config
// parameters, create the S3 bucket if it doesn't already exist, and check that versioning is enabled.
func (s3Initializer S3Initializer) Initialize(ctx context.Context, remoteState *RemoteState, terragruntOptions *options.TerragruntOptions) error {
	s3ConfigExtended, err := ParseExtendedS3Config(remoteState.Config)
	if err != nil {
		return errors.New(err)
	}

	if err := ValidateS3Config(s3ConfigExtended); err != nil {
		return errors.New(err)
	}

	var s3Config = s3ConfigExtended.RemoteStateConfigS3

	cacheKey := s3Initializer.buildInitializerCacheKey(&s3Config)
	if initialized, hit := initializedRemoteStateCache.Get(ctx, cacheKey); initialized && hit {
		terragruntOptions.Logger.Debugf("S3 bucket %s has already been confirmed to be initialized, skipping initialization checks", s3Config.Bucket)
		return nil
	}

	// ensure that only one goroutine can initialize bucket
	return stateAccessLock.StateBucketUpdate(s3Config.Bucket, func() error {
		// Check if another goroutine has already initialized the bucket
		if initialized, hit := initializedRemoteStateCache.Get(ctx, cacheKey); initialized && hit {
			terragruntOptions.Logger.Debugf("S3 bucket %s has already been confirmed to be initialized, skipping initialization checks", s3Config.Bucket)
			return nil
		}

		// Display a deprecation warning when the "lock_table" attribute is being used
		// during initialization.
		if s3Config.LockTable != "" {
			terragruntOptions.Logger.Warnf("%s\n", lockTableDeprecationMessage)
		}

		s3Client, err := CreateS3Client(s3ConfigExtended.GetAwsSessionConfig(), terragruntOptions)
		if err != nil {
			return errors.New(err)
		}

		if err := createS3BucketIfNecessary(ctx, s3Client, s3ConfigExtended, terragruntOptions); err != nil {
			return errors.New(err)
		}

		if !terragruntOptions.DisableBucketUpdate && !s3ConfigExtended.DisableBucketUpdate {
			if err := updateS3BucketIfNecessary(ctx, s3Client, s3ConfigExtended, terragruntOptions); err != nil {
				return errors.New(err)
			}
		}

		if !s3ConfigExtended.SkipBucketVersioning {
			if _, err := checkIfVersioningEnabled(s3Client, &s3Config, terragruntOptions); err != nil {
				return errors.New(err)
			}
		}

		if err := createLockTableIfNecessary(s3ConfigExtended, s3ConfigExtended.DynamotableTags, terragruntOptions); err != nil {
			return errors.New(err)
		}

		if err := UpdateLockTableSetSSEncryptionOnIfNecessary(&s3Config, s3ConfigExtended, terragruntOptions); err != nil {
			return errors.New(err)
		}

		initializedRemoteStateCache.Put(ctx, cacheKey, true)

		return nil
	})
}

func (s3Initializer S3Initializer) GetTerraformInitArgs(config map[string]interface{}) map[string]interface{} {
	var filteredConfig = make(map[string]interface{})

	const (
		lockTableKey     = "lock_table"
		dynamoDBTableKey = "dynamodb_table"
		assumeRoleKey    = "assume_role"
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

		if key == assumeRoleKey {
			if mapVal, ok := val.(map[string]interface{}); ok {
				filteredConfig[key] = WrapMapToSingleLineHcl(mapVal)
				continue
			}
		}

		filteredConfig[key] = val
	}

	return filteredConfig
}

// ParseExtendedS3Config parses the given map into an extended S3 config.
func ParseExtendedS3Config(config map[string]interface{}) (*ExtendedRemoteStateConfigS3, error) {
	var (
		s3Config       RemoteStateConfigS3
		extendedConfig ExtendedRemoteStateConfigS3
	)

	if err := mapstructure.Decode(config, &s3Config); err != nil {
		return nil, errors.New(err)
	}

	if err := mapstructure.Decode(config, &extendedConfig); err != nil {
		return nil, errors.New(err)
	}

	_, targetPrefixExists := config["accesslogging_target_prefix"]
	if !targetPrefixExists {
		extendedConfig.AccessLoggingTargetPrefix = DefaultS3BucketAccessLoggingTargetPrefix
	}

	extendedConfig.RemoteStateConfigS3 = s3Config

	return &extendedConfig, nil
}

// ValidateS3Config validates all the parameters of the given S3 remote state configuration.
func ValidateS3Config(extendedConfig *ExtendedRemoteStateConfigS3) error {
	var config = extendedConfig.RemoteStateConfigS3

	if config.Region == "" {
		return errors.New(MissingRequiredS3RemoteStateConfig("region"))
	}

	if config.Bucket == "" {
		return errors.New(MissingRequiredS3RemoteStateConfig("bucket"))
	}

	if config.Key == "" {
		return errors.New(MissingRequiredS3RemoteStateConfig("key"))
	}

	return nil
}

// If the bucket specified in the given config doesn't already exist, prompt the user to create it, and if the user
// confirms, create the bucket and enable versioning for it.
func createS3BucketIfNecessary(ctx context.Context, s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if DoesS3BucketExist(s3Client, &config.RemoteStateConfigS3.Bucket) {
		return nil
	}

	if terragruntOptions.FailIfBucketCreationRequired {
		return BucketCreationNotAllowed(config.RemoteStateConfigS3.Bucket)
	}

	prompt := fmt.Sprintf("Remote state S3 bucket %s does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", config.RemoteStateConfigS3.Bucket)

	shouldCreateBucket, err := shell.PromptUserForYesNo(ctx, prompt, terragruntOptions)
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
		description := "Create S3 bucket with retry " + config.RemoteStateConfigS3.Bucket

		return util.DoWithRetry(ctx, description, s3MaxRetries, s3SleepBetweenRetries, terragruntOptions.Logger, log.DebugLevel, func(ctx context.Context) error {
			err := CreateS3BucketWithVersioningSSEncryptionAndAccessLogging(ctx, s3Client, config, terragruntOptions)
			if err != nil {
				if isBucketCreationErrorRetriable(err) {
					return err
				}
				// return FatalError so that retry loop will not continue
				return util.FatalError{Underlying: err}
			}

			return nil
		})
	}

	return nil
}

func updateS3BucketIfNecessary(ctx context.Context, s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if !DoesS3BucketExist(s3Client, &config.RemoteStateConfigS3.Bucket) {
		if terragruntOptions.FailIfBucketCreationRequired {
			return BucketCreationNotAllowed(config.RemoteStateConfigS3.Bucket)
		}

		return errors.New(fmt.Errorf("remote state S3 bucket %s does not exist or you don't have permissions to access it", config.RemoteStateConfigS3.Bucket))
	}

	needsUpdate, bucketUpdatesRequired, err := checkIfS3BucketNeedsUpdate(s3Client, config, terragruntOptions)
	if err != nil {
		return err
	}

	if !needsUpdate {
		terragruntOptions.Logger.Debug("S3 bucket is already up to date")
		return nil
	}

	prompt := fmt.Sprintf("Remote state S3 bucket %s is out of date. Would you like Terragrunt to update it?", config.RemoteStateConfigS3.Bucket)

	shouldUpdateBucket, err := shell.PromptUserForYesNo(ctx, prompt, terragruntOptions)
	if err != nil {
		return err
	}

	if !shouldUpdateBucket {
		return nil
	}

	if bucketUpdatesRequired.Versioning {
		if config.SkipBucketVersioning {
			terragruntOptions.Logger.Debugf("Versioning is disabled for the remote state S3 bucket %s using 'skip_bucket_versioning' config.", config.RemoteStateConfigS3.Bucket)
		} else if err := EnableVersioningForS3Bucket(s3Client, &config.RemoteStateConfigS3, terragruntOptions); err != nil {
			return err
		}
	}

	if bucketUpdatesRequired.SSEEncryption {
		msg := fmt.Sprintf("Encryption is not enabled on the S3 remote state bucket %s. Terraform state files may contain secrets, so we STRONGLY recommend enabling encryption!", config.RemoteStateConfigS3.Bucket)

		if config.SkipBucketSSEncryption {
			terragruntOptions.Logger.Debug(msg)
			terragruntOptions.Logger.Debugf("Server-Side Encryption enabling is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_ssencryption' config.", config.RemoteStateConfigS3.Bucket)

			return nil
		} else {
			terragruntOptions.Logger.Warn(msg)
		}

		terragruntOptions.Logger.Infof("Enabling Server-Side Encryption for the remote state AWS S3 bucket %s.", config.RemoteStateConfigS3.Bucket)

		if err := EnableSSEForS3BucketWide(s3Client, config.RemoteStateConfigS3.Bucket, fetchEncryptionAlgorithm(config), config, terragruntOptions); err != nil {
			terragruntOptions.Logger.Errorf("Failed to enable Server-Side Encryption for the remote state AWS S3 bucket %s: %v", config.RemoteStateConfigS3.Bucket, err)
			return err
		}

		terragruntOptions.Logger.Infof("Successfully enabled Server-Side Encryption for the remote state AWS S3 bucket %s.", config.RemoteStateConfigS3.Bucket)
	}

	if bucketUpdatesRequired.RootAccess {
		if config.SkipBucketRootAccess {
			terragruntOptions.Logger.Debugf("Root access is disabled for the remote state S3 bucket %s using 'skip_bucket_root_access' config.", config.RemoteStateConfigS3.Bucket)
		} else if err := EnableRootAccesstoS3Bucket(s3Client, config, terragruntOptions); err != nil {
			return err
		}
	}

	if bucketUpdatesRequired.EnforcedTLS {
		if config.SkipBucketEnforcedTLS {
			terragruntOptions.Logger.Debugf("Enforced TLS is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_enforced_tls' config.", config.RemoteStateConfigS3.Bucket)
		} else if err := EnableEnforcedTLSAccesstoS3Bucket(s3Client, config.RemoteStateConfigS3.Bucket, config, terragruntOptions); err != nil {
			return err
		}
	}

	if bucketUpdatesRequired.AccessLogging {
		if config.SkipBucketAccessLogging {
			terragruntOptions.Logger.Debugf("Access logging is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_access_logging' config.", config.RemoteStateConfigS3.Bucket)
		} else {
			if config.AccessLoggingBucketName != "" {
				if err := configureAccessLogBucket(ctx, terragruntOptions, s3Client, config); err != nil {
					// TODO: Remove lint suppression
					return nil //nolint:nilerr
				}
			} else {
				terragruntOptions.Logger.Debugf("Access Logging is disabled for the remote state AWS S3 bucket %s", config.RemoteStateConfigS3.Bucket)
			}
		}
	}

	if bucketUpdatesRequired.PublicAccess {
		if config.SkipBucketPublicAccessBlocking {
			terragruntOptions.Logger.Debugf("Public access blocking is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_public_access_blocking' config.", config.RemoteStateConfigS3.Bucket)
		} else if err := EnablePublicAccessBlockingForS3Bucket(s3Client, config.RemoteStateConfigS3.Bucket, terragruntOptions); err != nil {
			return err
		}
	}

	return nil
}

// configureAccessLogBucket - configure access log bucket.
func configureAccessLogBucket(ctx context.Context, terragruntOptions *options.TerragruntOptions, s3Client *s3.S3, config *ExtendedRemoteStateConfigS3) error {
	terragruntOptions.Logger.Debugf("Enabling bucket-wide Access Logging on AWS S3 bucket %s - using as TargetBucket %s", config.RemoteStateConfigS3.Bucket, config.AccessLoggingBucketName)

	if err := CreateLogsS3BucketIfNecessary(ctx, s3Client, aws.String(config.AccessLoggingBucketName), terragruntOptions); err != nil {
		terragruntOptions.Logger.Errorf("Could not create logs bucket %s for AWS S3 bucket %s\n%s", config.AccessLoggingBucketName, config.RemoteStateConfigS3.Bucket, err.Error())

		return errors.New(err)
	}

	if !config.SkipAccessLoggingBucketPublicAccessBlocking {
		if err := EnablePublicAccessBlockingForS3Bucket(s3Client, config.AccessLoggingBucketName, terragruntOptions); err != nil {
			terragruntOptions.Logger.Errorf("Could not enable public access blocking on %s\n%s", config.AccessLoggingBucketName, err.Error())

			return errors.New(err)
		}
	}

	if err := EnableAccessLoggingForS3BucketWide(s3Client, config, terragruntOptions); err != nil {
		terragruntOptions.Logger.Errorf("Could not enable access logging on %s\n%s", config.RemoteStateConfigS3.Bucket, err.Error())

		return errors.New(err)
	}

	if !config.SkipAccessLoggingBucketSSEncryption {
		if err := EnableSSEForS3BucketWide(s3Client, config.AccessLoggingBucketName, s3.ServerSideEncryptionAes256, config, terragruntOptions); err != nil {
			terragruntOptions.Logger.Errorf("Could not enable encryption on %s\n%s", config.AccessLoggingBucketName, err.Error())

			return errors.New(err)
		}
	}

	if !config.SkipAccessLoggingBucketEnforcedTLS {
		if err := EnableEnforcedTLSAccesstoS3Bucket(s3Client, config.AccessLoggingBucketName, config, terragruntOptions); err != nil {
			terragruntOptions.Logger.Errorf("Could not enable TLS access on %s\n%s", config.AccessLoggingBucketName, err.Error())

			return errors.New(err)
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
	var (
		updates  []string
		toUpdate S3BucketUpdatesRequired
	)

	if !config.SkipBucketVersioning {
		enabled, err := checkIfVersioningEnabled(s3Client, &config.RemoteStateConfigS3, terragruntOptions)
		if err != nil {
			return false, toUpdate, err
		}

		if !enabled {
			toUpdate.Versioning = true

			updates = append(updates, "Bucket Versioning")
		}
	}

	if !config.SkipBucketSSEncryption {
		matches, err := checkIfSSEForS3MatchesConfig(s3Client, config, terragruntOptions)
		if err != nil {
			return false, toUpdate, err
		}

		if !matches {
			toUpdate.SSEEncryption = true

			updates = append(updates, "Bucket Server-Side Encryption")
		}
	}

	if !config.SkipBucketRootAccess {
		enabled, err := checkIfBucketRootAccess(s3Client, &config.RemoteStateConfigS3, terragruntOptions)
		if err != nil {
			return false, toUpdate, err
		}

		if !enabled {
			toUpdate.RootAccess = true

			updates = append(updates, "Bucket Root Access")
		}
	}

	if !config.SkipBucketEnforcedTLS {
		enabled, err := checkIfBucketEnforcedTLS(s3Client, &config.RemoteStateConfigS3, terragruntOptions)
		if err != nil {
			return false, toUpdate, err
		}

		if !enabled {
			toUpdate.EnforcedTLS = true

			updates = append(updates, "Bucket Enforced TLS")
		}
	}

	if !config.SkipBucketAccessLogging && config.AccessLoggingBucketName != "" {
		enabled, err := checkS3AccessLoggingConfiguration(s3Client, config, terragruntOptions)
		if err != nil {
			return false, toUpdate, err
		}

		if !enabled {
			toUpdate.AccessLogging = true

			updates = append(updates, "Bucket Access Logging")
		}
	}

	if !config.SkipBucketPublicAccessBlocking {
		enabled, err := checkIfS3PublicAccessBlockingEnabled(s3Client, &config.RemoteStateConfigS3, terragruntOptions)
		if err != nil {
			return false, toUpdate, err
		}

		if !enabled {
			toUpdate.PublicAccess = true

			updates = append(updates, "Bucket Public Access Blocking")
		}
	}

	// show update message if any of the above configs are not set
	if len(updates) > 0 {
		terragruntOptions.Logger.Warnf("The remote state S3 bucket %s needs to be updated:", config.RemoteStateConfigS3.Bucket)

		for _, update := range updates {
			terragruntOptions.Logger.Warnf("  - %s", update)
		}

		return true, toUpdate, nil
	}

	return false, toUpdate, nil
}

// Check if versioning is enabled for the S3 bucket specified in the given config and warn the user if it is not
func checkIfVersioningEnabled(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) (bool, error) {
	terragruntOptions.Logger.Debugf("Verifying AWS S3 Bucket Versioning %s", config.Bucket)

	out, err := s3Client.GetBucketVersioning(&s3.GetBucketVersioningInput{Bucket: aws.String(config.Bucket)})
	if err != nil {
		return false, errors.New(err)
	}

	// NOTE: There must be a bug in the AWS SDK since out == nil when versioning is not enabled. In the future,
	// check the AWS SDK for updates to see if we can remove "out == nil ||".
	if out == nil || out.Status == nil || *out.Status != s3.BucketVersioningStatusEnabled {
		terragruntOptions.Logger.Warnf("Versioning is not enabled for the remote state S3 bucket %s. We recommend enabling versioning so that you can roll back to previous versions of your Terraform state in case of error.", config.Bucket)
		return false, nil
	}

	return true, nil
}

// CreateS3BucketWithVersioningSSEncryptionAndAccessLogging creates the given S3 bucket and enable versioning for it.
func CreateS3BucketWithVersioningSSEncryptionAndAccessLogging(ctx context.Context, s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Create S3 bucket %s with versioning, SSE encryption, and access logging.", config.RemoteStateConfigS3.Bucket)

	err := CreateS3Bucket(s3Client, aws.String(config.RemoteStateConfigS3.Bucket), terragruntOptions)

	if err != nil {
		if accessError := checkBucketAccess(s3Client, aws.String(config.RemoteStateConfigS3.Bucket), aws.String(config.RemoteStateConfigS3.Key)); accessError != nil {
			return accessError
		}

		if isBucketAlreadyOwnedByYouError(err) {
			terragruntOptions.Logger.Debugf("Looks like you're already creating bucket %s at the same time. Will not attempt to create it again.", config.RemoteStateConfigS3.Bucket)
			return WaitUntilS3BucketExists(s3Client, &config.RemoteStateConfigS3, terragruntOptions)
		}

		return err
	}

	if err := WaitUntilS3BucketExists(s3Client, &config.RemoteStateConfigS3, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketRootAccess {
		terragruntOptions.Logger.Debugf("Root access is disabled for the remote state S3 bucket %s using 'skip_bucket_root_access' config.", config.RemoteStateConfigS3.Bucket)
	} else if err := EnableRootAccesstoS3Bucket(s3Client, config, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketEnforcedTLS {
		terragruntOptions.Logger.Debugf("TLS enforcement is disabled for the remote state S3 bucket %s using 'skip_bucket_enforced_tls' config.", config.RemoteStateConfigS3.Bucket)
	} else if err := EnableEnforcedTLSAccesstoS3Bucket(s3Client, config.RemoteStateConfigS3.Bucket, config, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketPublicAccessBlocking {
		terragruntOptions.Logger.Debugf("Public access blocking is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_public_access_blocking' config.", config.RemoteStateConfigS3.Bucket)
	} else if err := EnablePublicAccessBlockingForS3Bucket(s3Client, config.RemoteStateConfigS3.Bucket, terragruntOptions); err != nil {
		return err
	}

	if err := TagS3Bucket(s3Client, config, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketVersioning {
		terragruntOptions.Logger.Debugf("Versioning is disabled for the remote state S3 bucket %s using 'skip_bucket_versioning' config.", config.RemoteStateConfigS3.Bucket)
	} else if err := EnableVersioningForS3Bucket(s3Client, &config.RemoteStateConfigS3, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketSSEncryption {
		terragruntOptions.Logger.Debugf("Server-Side Encryption is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_ssencryption' config.", config.RemoteStateConfigS3.Bucket)
	} else if err := EnableSSEForS3BucketWide(s3Client, config.RemoteStateConfigS3.Bucket, fetchEncryptionAlgorithm(config), config, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketAccessLogging {
		terragruntOptions.Logger.Warnf("Terragrunt configuration option 'skip_bucket_accesslogging' is now deprecated. Access logging for the state bucket %s is disabled by default. To enable access logging for bucket %s, please provide property `accesslogging_bucket_name` in the terragrunt config file. For more details, please refer to the Terragrunt documentation.", config.RemoteStateConfigS3.Bucket, config.RemoteStateConfigS3.Bucket)
	}

	if config.AccessLoggingBucketName != "" {
		if err := configureAccessLogBucket(ctx, terragruntOptions, s3Client, config); err != nil {
			// TODO: Remove lint suppression
			return nil //nolint:nilerr
		}
	} else {
		terragruntOptions.Logger.Debugf("Access Logging is disabled for the remote state AWS S3 bucket %s", config.RemoteStateConfigS3.Bucket)
	}

	if err := TagS3BucketAccessLogging(s3Client, config, terragruntOptions); err != nil {
		return err
	}

	return nil
}

func CreateLogsS3BucketIfNecessary(ctx context.Context, s3Client *s3.S3, logsBucketName *string, terragruntOptions *options.TerragruntOptions) error {
	if !DoesS3BucketExist(s3Client, logsBucketName) {
		if terragruntOptions.FailIfBucketCreationRequired {
			return BucketCreationNotAllowed(*logsBucketName)
		}

		prompt := fmt.Sprintf("Logs S3 bucket %s for the remote state does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", *logsBucketName)

		shouldCreateBucket, err := shell.PromptUserForYesNo(ctx, prompt, terragruntOptions)
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
	if len(config.AccessLoggingBucketTags) == 0 {
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
		return errors.New(err)
	}

	terragruntOptions.Logger.Debugf("Tagged S3 bucket with %s", config.AccessLoggingBucketTags)

	return nil
}

func TagS3Bucket(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if len(config.S3BucketTags) == 0 {
		terragruntOptions.Logger.Debugf("No tags specified for bucket %s.", config.RemoteStateConfigS3.Bucket)
		return nil
	}

	// There must be one entry in the list
	var tagsConverted = convertTags(config.S3BucketTags)

	terragruntOptions.Logger.Debugf("Tagging S3 bucket with %s", config.S3BucketTags)

	putBucketTaggingInput := s3.PutBucketTaggingInput{
		Bucket: aws.String(config.RemoteStateConfigS3.Bucket),
		Tagging: &s3.Tagging{
			TagSet: tagsConverted,
		},
	}

	_, err := s3Client.PutBucketTagging(&putBucketTaggingInput)
	if err != nil {
		return errors.New(err)
	}

	terragruntOptions.Logger.Debugf("Tagged S3 bucket with %s", config.S3BucketTags)

	return nil
}

func convertTags(tags map[string]string) []*s3.Tag {
	var tagsConverted = make([]*s3.Tag, 0, len(tags))

	for k, v := range tags {
		var tag = s3.Tag{
			Key:   aws.String(k),
			Value: aws.String(v)}

		tagsConverted = append(tagsConverted, &tag)
	}

	return tagsConverted
}

// WaitUntilS3BucketExists waits until the given S3 bucket exists.
//
// AWS is eventually consistent, so after creating an S3 bucket, this method can be used to wait until the information
// about that S3 bucket has propagated everywhere.
func WaitUntilS3BucketExists(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Waiting for bucket %s to be created", config.Bucket)

	for retries := 0; retries < MaxRetriesWaitingForS3Bucket; retries++ {
		if DoesS3BucketExist(s3Client, aws.String(config.Bucket)) {
			terragruntOptions.Logger.Debugf("S3 bucket %s created.", config.Bucket)
			return nil
		} else if retries < MaxRetriesWaitingForS3Bucket-1 {
			terragruntOptions.Logger.Debugf("S3 bucket %s has not been created yet. Sleeping for %s and will check again.", config.Bucket, SleepBetweenRetriesWaitingForS3Bucket)
			time.Sleep(SleepBetweenRetriesWaitingForS3Bucket)
		}
	}

	return errors.New(MaxRetriesWaitingForS3BucketExceeded(config.Bucket))
}

// CreateS3Bucket creates the S3 bucket specified in the given config.
func CreateS3Bucket(s3Client *s3.S3, bucket *string, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Creating S3 bucket %s", aws.StringValue(bucket))
	// https://github.com/aws/aws-sdk-go/blob/v1.44.245/service/s3/api.go#L41760
	_, err := s3Client.CreateBucket(&s3.CreateBucketInput{Bucket: bucket, ObjectOwnership: aws.String("ObjectWriter")})
	if err != nil {
		return errors.New(err)
	}

	terragruntOptions.Logger.Debugf("Created S3 bucket %s", aws.StringValue(bucket))

	return nil
}

// Determine if this is an error that implies you've already made a request to create the S3 bucket and it succeeded
// or is in progress. This usually happens when running many tests in parallel or xxx-all commands.
func isBucketAlreadyOwnedByYouError(err error) bool {
	var awsErr awserr.Error
	ok := errors.As(err, &awsErr)

	return ok && (awsErr.Code() == "BucketAlreadyOwnedByYou" || awsErr.Code() == "OperationAborted")
}

// isBucketCreationErrorRetriable returns true if the error is temporary and bucket creation can be retried.
func isBucketCreationErrorRetriable(err error) bool {
	var awsErr awserr.Error

	ok := errors.As(err, &awsErr)
	if !ok {
		return true
	}

	return awsErr.Code() == "InternalError" || awsErr.Code() == "OperationAborted" || awsErr.Code() == "InvalidParameter"
}

// EnableRootAccesstoS3Bucket adds a policy to allow root access to the bucket.
func EnableRootAccesstoS3Bucket(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	bucket := config.RemoteStateConfigS3.Bucket
	terragruntOptions.Logger.Debugf("Enabling root access to S3 bucket %s", bucket)

	accountID, err := awshelper.GetAWSAccountID(config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return errors.Errorf("error getting AWS account ID %s for bucket %s: %w", accountID, bucket, err)
	}

	partition, err := awshelper.GetAWSPartition(config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return errors.Errorf("error getting AWS partition %s for bucket %s: %w", partition, bucket, err)
	}

	var policyInBucket awshelper.Policy

	policyOutput, err := s3Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(bucket),
	})

	// If there's no policy, we need to create one
	if err != nil {
		terragruntOptions.Logger.Debugf("Policy not exists for bucket %s", bucket)
	}

	if policyOutput.Policy != nil {
		terragruntOptions.Logger.Debugf("Policy already exists for bucket %s", bucket)

		policyInBucket, err = awshelper.UnmarshalPolicy(*policyOutput.Policy)
		if err != nil {
			return errors.Errorf("error unmarshalling policy for bucket %s: %w", bucket, err)
		}
	}

	for _, statement := range policyInBucket.Statement {
		if statement.Sid == SidRootPolicy {
			terragruntOptions.Logger.Debugf("Policy for RootAccess already exists for bucket %s", bucket)
			return nil
		}
	}

	rootS3Policy := awshelper.Policy{
		Version: "2012-10-17",
		Statement: []awshelper.Statement{
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

	policy, err := awshelper.MarshalPolicy(rootS3Policy)
	if err != nil {
		return errors.Errorf("error marshalling policy for bucket %s: %w", bucket, err)
	}

	_, err = s3Client.PutBucketPolicy(&s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(string(policy)),
	})
	if err != nil {
		return errors.Errorf("error putting policy for bucket %s: %w", bucket, err)
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
		// NoSuchBucketPolicy error is considered as no policy.
		var awsErr awserr.Error
		if ok := errors.As(err, &awsErr); ok {
			if awsErr.Code() == "NoSuchBucketPolicy" {
				return false, nil
			}
		}

		terragruntOptions.Logger.Debugf("Could not get policy for bucket %s", config.Bucket)

		return false, errors.Errorf("error checking if bucket %s is have root access: %w", config.Bucket, err)
	}

	// If the bucket has no policy, it is not enforced
	if policyOutput == nil {
		return true, nil
	}

	policyInBucket, err := awshelper.UnmarshalPolicy(*policyOutput.Policy)
	if err != nil {
		return false, errors.Errorf("error unmarshalling policy for bucket %s: %w", config.Bucket, err)
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

// EnableEnforcedTLSAccesstoS3Bucket adds a policy to enforce TLS based access to the bucket.
func EnableEnforcedTLSAccesstoS3Bucket(s3Client *s3.S3, bucket string, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Enabling enforced TLS access for S3 bucket %s", bucket)

	partition, err := awshelper.GetAWSPartition(config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return errors.New(err)
	}

	var policyInBucket awshelper.Policy

	policyOutput, err := s3Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(bucket),
	})
	// If there's no policy, we need to create one
	if err != nil {
		terragruntOptions.Logger.Debugf("Policy not exists for bucket %s", bucket)
	}

	if policyOutput.Policy != nil {
		terragruntOptions.Logger.Debugf("Policy already exists for bucket %s", bucket)

		policyInBucket, err = awshelper.UnmarshalPolicy(*policyOutput.Policy)
		if err != nil {
			return errors.Errorf("error unmarshalling policy for bucket %s: %w", bucket, err)
		}
	}

	for _, statement := range policyInBucket.Statement {
		if statement.Sid == SidEnforcedTLSPolicy {
			terragruntOptions.Logger.Debugf("Policy for EnforceTLS already exists for bucket %s", bucket)
			return nil
		}
	}

	tlsS3Policy := awshelper.Policy{
		Version: "2012-10-17",
		Statement: []awshelper.Statement{
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

	policy, err := awshelper.MarshalPolicy(tlsS3Policy)
	if err != nil {
		return errors.Errorf("error marshalling policy for bucket %s: %w", bucket, err)
	}

	_, err = s3Client.PutBucketPolicy(&s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(string(policy)),
	})
	if err != nil {
		return errors.Errorf("error putting policy for bucket %s: %w", bucket, err)
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
		var awsErr awserr.Error
		if ok := errors.As(err, &awsErr); ok {
			// Enforced TLS policy if is not found bucket policy
			if awsErr.Code() == "NoSuchBucketPolicy" {
				terragruntOptions.Logger.Debugf("Could not get policy for bucket %s", config.Bucket)
				return false, nil
			}
		}

		return false, errors.Errorf("error checking if bucket %s is enforced with TLS: %w", config.Bucket, err)
	}

	if policyOutput.Policy == nil {
		return true, nil
	}

	policyInBucket, err := awshelper.UnmarshalPolicy(*policyOutput.Policy)
	if err != nil {
		return false, errors.Errorf("error unmarshalling policy for bucket %s: %w", config.Bucket, err)
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

// EnableVersioningForS3Bucket enables versioning for the S3 bucket specified in the given config.
func EnableVersioningForS3Bucket(s3Client *s3.S3, config *RemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Enabling versioning on S3 bucket %s", config.Bucket)
	input := s3.PutBucketVersioningInput{
		Bucket:                  aws.String(config.Bucket),
		VersioningConfiguration: &s3.VersioningConfiguration{Status: aws.String(s3.BucketVersioningStatusEnabled)},
	}

	_, err := s3Client.PutBucketVersioning(&input)
	if err != nil {
		return errors.Errorf("error enabling versioning on S3 bucket %s: %w", config.Bucket, err)
	}

	terragruntOptions.Logger.Debugf("Enabled versioning on S3 bucket %s", config.Bucket)

	return nil
}

// EnableSSEForS3BucketWide enables bucket-wide Server-Side Encryption for the AWS S3 bucket specified in the given config.
func EnableSSEForS3BucketWide(s3Client *s3.S3, bucketName string, algorithm string, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Enabling bucket-wide SSE on AWS S3 bucket %s", bucketName)

	accountID, err := awshelper.GetAWSAccountID(config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return errors.New(err)
	}

	partition, err := awshelper.GetAWSPartition(config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return errors.New(err)
	}

	defEnc := &s3.ServerSideEncryptionByDefault{
		SSEAlgorithm: aws.String(algorithm),
	}
	if algorithm == s3.ServerSideEncryptionAwsKms && config.BucketSSEKMSKeyID != "" {
		defEnc.KMSMasterKeyID = aws.String(config.BucketSSEKMSKeyID)
	} else if algorithm == s3.ServerSideEncryptionAwsKms {
		kmsKeyID := fmt.Sprintf("arn:%s:kms:%s:%s:alias/aws/s3", partition, config.RemoteStateConfigS3.Region, accountID)
		defEnc.KMSMasterKeyID = aws.String(kmsKeyID)
	}

	rule := &s3.ServerSideEncryptionRule{ApplyServerSideEncryptionByDefault: defEnc}
	rules := []*s3.ServerSideEncryptionRule{rule}
	serverConfig := &s3.ServerSideEncryptionConfiguration{Rules: rules}
	input := &s3.PutBucketEncryptionInput{Bucket: aws.String(bucketName), ServerSideEncryptionConfiguration: serverConfig}

	_, err = s3Client.PutBucketEncryption(input)
	if err != nil {
		return errors.Errorf("error enabling bucket-wide SSE on AWS S3 bucket %s: %w", bucketName, err)
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

func checkIfSSEForS3MatchesConfig(
	s3Client *s3.S3,
	config *ExtendedRemoteStateConfigS3,
	terragruntOptions *options.TerragruntOptions,
) (bool, error) {
	terragruntOptions.Logger.Debugf("Checking if SSE is enabled for AWS S3 bucket %s", config.RemoteStateConfigS3.Bucket)

	input := &s3.GetBucketEncryptionInput{Bucket: aws.String(config.RemoteStateConfigS3.Bucket)}

	output, err := s3Client.GetBucketEncryption(input)
	if err != nil {
		terragruntOptions.Logger.Debugf("Error checking if SSE is enabled for AWS S3 bucket %s: %s", config.RemoteStateConfigS3.Bucket, err.Error())

		return false, errors.Errorf("error checking if SSE is enabled for AWS S3 bucket %s: %w", config.RemoteStateConfigS3.Bucket, err)
	}

	if output.ServerSideEncryptionConfiguration == nil {
		return false, nil
	}

	for _, rule := range output.ServerSideEncryptionConfiguration.Rules {
		if rule.ApplyServerSideEncryptionByDefault != nil && rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm != nil {
			if *rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm == fetchEncryptionAlgorithm(config) {
				return true, nil
			}

			return false, nil
		}
	}

	return false, nil
}

// EnableAccessLoggingForS3BucketWide enables bucket-wide Access Logging for the AWS S3 bucket specified in the given config.
func EnableAccessLoggingForS3BucketWide(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	bucket := config.RemoteStateConfigS3.Bucket
	logsBucket := config.AccessLoggingBucketName
	logsBucketPrefix := config.AccessLoggingTargetPrefix

	if !config.SkipAccessLoggingBucketACL {
		if err := configureBucketAccessLoggingACL(s3Client, aws.String(logsBucket), terragruntOptions); err != nil {
			return errors.Errorf("error configuring bucket access logging ACL on S3 bucket %s: %w", config.RemoteStateConfigS3.Bucket, err)
		}
	}

	loggingInput := config.CreateS3LoggingInput()
	terragruntOptions.Logger.Debugf("Putting bucket logging on S3 bucket %s with TargetBucket %s and TargetPrefix %s\n%s", bucket, logsBucket, logsBucketPrefix, loggingInput)

	if _, err := s3Client.PutBucketLogging(&loggingInput); err != nil {
		return errors.Errorf("error enabling bucket-wide Access Logging on AWS S3 bucket %s: %w", config.RemoteStateConfigS3.Bucket, err)
	}

	terragruntOptions.Logger.Debugf("Enabled bucket-wide Access Logging on AWS S3 bucket %s", bucket)

	return nil
}

func checkS3AccessLoggingConfiguration(s3Client *s3.S3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) (bool, error) {
	terragruntOptions.Logger.Debugf("Checking if Access Logging is enabled for AWS S3 bucket %s", config.RemoteStateConfigS3.Bucket)

	input := &s3.GetBucketLoggingInput{Bucket: aws.String(config.RemoteStateConfigS3.Bucket)}

	output, err := s3Client.GetBucketLogging(input)
	if err != nil {
		terragruntOptions.Logger.Debugf("Error checking if Access Logging is enabled for AWS S3 bucket %s: %s", config.RemoteStateConfigS3.Bucket, err.Error())
		return false, errors.Errorf("error checking if Access Logging is enabled for AWS S3 bucket %s: %w", config.RemoteStateConfigS3.Bucket, err)
	}

	if output.LoggingEnabled == nil {
		return false, nil
	}

	loggingInput := config.CreateS3LoggingInput()

	if !reflect.DeepEqual(output.LoggingEnabled, loggingInput.BucketLoggingStatus.LoggingEnabled) {
		return false, nil
	}

	return true, nil
}

// EnablePublicAccessBlockingForS3Bucket blocks all public access policies on the bucket and objects.
// These settings ensure that a misconfiguration of the
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
		return errors.Errorf("error blocking all public access to S3 bucket %s: %w", bucketName, err)
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
		var awsErr awserr.Error
		if ok := errors.As(err, &awsErr); ok {
			// Enforced block public access if is not found bucket policy
			if awsErr.Code() == "NoSuchPublicAccessBlockConfiguration" {
				terragruntOptions.Logger.Debugf("Could not get public access block for bucket %s", config.Bucket)
				return false, nil
			}
		}

		return false, errors.Errorf("error checking if S3 bucket %s is configured to block public access: %w", config.Bucket, err)
	}

	return ValidatePublicAccessBlock(output)
}

func ValidatePublicAccessBlock(output *s3.GetPublicAccessBlockOutput) (bool, error) {
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

// configureBucketAccessLoggingACL grants WRITE and READ_ACP permissions to
// the Log Delivery Group for the S3 bucket.
//
// To enable access logging in an S3 bucket, you must grant WRITE and READ_ACP permissions to the Log Delivery
// Group. For more info, see:
// https://docs.aws.amazon.com/AmazonS3/latest/dev/enable-logging-programming.html
func configureBucketAccessLoggingACL(s3Client *s3.S3, bucket *string, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Granting WRITE and READ_ACP permissions to S3 Log Delivery (%s) for bucket %s. This is required for access logging.", s3LogDeliveryGranteeURI, aws.StringValue(bucket))

	uri := "uri=" + s3LogDeliveryGranteeURI
	aclInput := s3.PutBucketAclInput{
		Bucket:       bucket,
		GrantWrite:   aws.String(uri),
		GrantReadACP: aws.String(uri),
	}

	if _, err := s3Client.PutBucketAcl(&aclInput); err != nil {
		return errors.Errorf("error granting WRITE and READ_ACP permissions to S3 Log Delivery (%s) for bucket %s: %w", s3LogDeliveryGranteeURI, *bucket, err)
	}

	return waitUntilBucketHasAccessLoggingACL(s3Client, bucket, terragruntOptions)
}

func waitUntilBucketHasAccessLoggingACL(s3Client *s3.S3, bucket *string, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Waiting for ACL bucket %s to have the updated ACL for access logging.", aws.StringValue(bucket))

	maxRetries := 10

	for i := 0; i < maxRetries; i++ {
		out, err := s3Client.GetBucketAcl(&s3.GetBucketAclInput{Bucket: bucket})
		if err != nil {
			return errors.Errorf("error getting ACL for bucket %s: %w", *bucket, err)
		}

		hasReadAcp := false
		hasWrite := false

		for _, grant := range out.Grants {
			if aws.StringValue(grant.Grantee.URI) == s3LogDeliveryGranteeURI {
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

		terragruntOptions.Logger.Debugf("Bucket %s still does not have the ACL permissions for access logging. Will sleep for %v and check again.", aws.StringValue(bucket), s3TimeBetweenRetries)
		time.Sleep(s3TimeBetweenRetries)
	}

	return errors.New(MaxRetriesWaitingForS3ACLExceeded(aws.StringValue(bucket)))
}

// DoesS3BucketExist checks if the S3 bucket specified in the given config exists.
//
// Returns true if the S3 bucket specified in the given config exists and the current user has the ability to access
// it.
func DoesS3BucketExist(s3Client *s3.S3, bucket *string) bool {
	_, err := s3Client.HeadBucket(&s3.HeadBucketInput{Bucket: bucket})
	return err == nil
}

// checkBucketAccess checks if the current user has the ability to access the S3 bucket keys.
func checkBucketAccess(s3Client *s3.S3, bucket *string, key *string) error {
	_, err := s3Client.GetObject(&s3.GetObjectInput{Key: key, Bucket: bucket})
	if err == nil {
		return nil
	}

	var awsErr awserr.Error

	ok := errors.As(err, &awsErr)
	if !ok {
		return err
	}

	// filter permissions errors
	if awsErr.Code() == "NoSuchBucket" || awsErr.Code() == "NoSuchKey" {
		return nil
	}

	return errors.Errorf("error checking access to S3 bucket %s: %w", *bucket, err)
}

// Create a table for locks in DynamoDB if the user has configured a lock table and the table doesn't already exist
func createLockTableIfNecessary(extendedS3Config *ExtendedRemoteStateConfigS3, tags map[string]string, terragruntOptions *options.TerragruntOptions) error {
	if extendedS3Config.RemoteStateConfigS3.GetLockTableName() == "" {
		return nil
	}

	dynamodbClient, err := dynamodb.CreateDynamoDBClient(extendedS3Config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return err
	}

	return dynamodb.CreateLockTableIfNecessary(extendedS3Config.RemoteStateConfigS3.GetLockTableName(), tags, dynamodbClient, terragruntOptions)
}

// UpdateLockTableSetSSEncryptionOnIfNecessary updates a table for locks in DynamoDB
// if the user has configured a lock table and the table's server-side encryption isn't turned on.
func UpdateLockTableSetSSEncryptionOnIfNecessary(s3Config *RemoteStateConfigS3, config *ExtendedRemoteStateConfigS3, terragruntOptions *options.TerragruntOptions) error {
	if !config.EnableLockTableSSEncryption {
		return nil
	}

	if s3Config.GetLockTableName() == "" {
		return nil
	}

	dynamodbClient, err := dynamodb.CreateDynamoDBClient(config.GetAwsSessionConfig(), terragruntOptions)
	if err != nil {
		return err
	}

	return dynamodb.UpdateLockTableSetSSEncryptionOnIfNecessary(s3Config.GetLockTableName(), dynamodbClient, terragruntOptions)
}

// CreateS3Client creates an authenticated client for DynamoDB.
func CreateS3Client(config *awshelper.AwsSessionConfig, terragruntOptions *options.TerragruntOptions) (*s3.S3, error) {
	session, err := awshelper.CreateAwsSession(config, terragruntOptions)
	if err != nil {
		return nil, err
	}

	return s3.New(session), nil
}

// Custom error types

type MissingRequiredS3RemoteStateConfig string

func (configName MissingRequiredS3RemoteStateConfig) Error() string {
	return "Missing required S3 remote state configuration " + string(configName)
}

type MultipleTagsDeclarations string

func (target MultipleTagsDeclarations) Error() string {
	return fmt.Sprintf("Tags for %s got declared multiple times. Please do only declare in one block.", string(target))
}

type MaxRetriesWaitingForS3BucketExceeded string

func (err MaxRetriesWaitingForS3BucketExceeded) Error() string {
	return fmt.Sprintf("Exceeded max retries (%d) waiting for bucket S3 bucket %s", MaxRetriesWaitingForS3Bucket, string(err))
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
