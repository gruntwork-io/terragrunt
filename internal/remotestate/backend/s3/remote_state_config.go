package s3

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

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
	S3BucketTags                                 map[string]string   `mapstructure:"s3_bucket_tags"`
	DynamotableTags                              map[string]string   `mapstructure:"dynamodb_table_tags"`
	AccessLoggingBucketTags                      map[string]string   `mapstructure:"accesslogging_bucket_tags"`
	AccessLoggingBucketName                      string              `mapstructure:"accesslogging_bucket_name"`
	BucketSSEKMSKeyID                            string              `mapstructure:"bucket_sse_kms_key_id"`
	BucketSSEAlgorithm                           string              `mapstructure:"bucket_sse_algorithm"`
	AccessLoggingTargetPrefix                    string              `mapstructure:"accesslogging_target_prefix"`
	AccessLoggingTargetObjectPartitionDateSource string              `mapstructure:"accesslogging_target_object_partition_date_source"`
	RemoteStateConfigS3                          RemoteStateConfigS3 `mapstructure:",squash"`
	SkipBucketVersioning                         bool                `mapstructure:"skip_bucket_versioning"`
	SkipBucketAccessLogging                      bool                `mapstructure:"skip_bucket_accesslogging"`
	DisableBucketUpdate                          bool                `mapstructure:"disable_bucket_update"`
	EnableLockTableSSEncryption                  bool                `mapstructure:"enable_lock_table_ssencryption"`
	DisableAWSClientChecksums                    bool                `mapstructure:"disable_aws_client_checksums"`
	SkipBucketEnforcedTLS                        bool                `mapstructure:"skip_bucket_enforced_tls"`
	SkipBucketRootAccess                         bool                `mapstructure:"skip_bucket_root_access"`
	SkipBucketPublicAccessBlocking               bool                `mapstructure:"skip_bucket_public_access_blocking"`
	SkipAccessLoggingBucketACL                   bool                `mapstructure:"skip_accesslogging_bucket_acl"`
	SkipAccessLoggingBucketEnforcedTLS           bool                `mapstructure:"skip_accesslogging_bucket_enforced_tls"`
	SkipAccessLoggingBucketPublicAccessBlocking  bool                `mapstructure:"skip_accesslogging_bucket_public_access_blocking"`
	SkipAccessLoggingBucketSSEncryption          bool                `mapstructure:"skip_accesslogging_bucket_ssencryption"`
	SkipBucketSSEncryption                       bool                `mapstructure:"skip_bucket_ssencryption"`
	SkipCredentialsValidation                    bool                `mapstructure:"skip_credentials_validation"`
}

func (cfg *ExtendedRemoteStateConfigS3) FetchEncryptionAlgorithm() string {
	// Encrypt with KMS by default
	algorithm := string(s3types.ServerSideEncryptionAwsKms)
	if cfg.BucketSSEAlgorithm != "" {
		algorithm = cfg.BucketSSEAlgorithm
	}

	return algorithm
}

// GetAwsSessionConfig builds a session config for AWS related requests
// from the RemoteStateConfigS3 configuration.
func (cfg *ExtendedRemoteStateConfigS3) GetAwsSessionConfig() *awshelper.AwsSessionConfig {
	s3Endpoint := cfg.RemoteStateConfigS3.Endpoint
	if cfg.RemoteStateConfigS3.Endpoints.S3 != "" {
		s3Endpoint = cfg.RemoteStateConfigS3.Endpoints.S3
	}

	dynamoDBEndpoint := cfg.RemoteStateConfigS3.DynamoDBEndpoint
	if cfg.RemoteStateConfigS3.Endpoints.DynamoDB != "" {
		dynamoDBEndpoint = cfg.RemoteStateConfigS3.Endpoints.DynamoDB
	}

	return &awshelper.AwsSessionConfig{
		Region:                  cfg.RemoteStateConfigS3.Region,
		CustomS3Endpoint:        s3Endpoint,
		CustomDynamoDBEndpoint:  dynamoDBEndpoint,
		Profile:                 cfg.RemoteStateConfigS3.Profile,
		RoleArn:                 cfg.RemoteStateConfigS3.GetSessionRoleArn(),
		Tags:                    cfg.RemoteStateConfigS3.GetSessionTags(),
		ExternalID:              cfg.RemoteStateConfigS3.GetExternalID(),
		SessionName:             cfg.RemoteStateConfigS3.GetSessionName(),
		CredsFilename:           cfg.RemoteStateConfigS3.CredsFilename,
		S3ForcePathStyle:        cfg.RemoteStateConfigS3.S3ForcePathStyle,
		DisableComputeChecksums: cfg.DisableAWSClientChecksums,
	}
}

// CreateS3LoggingInput builds AWS S3 logging input struct from the configuration.
func (cfg *ExtendedRemoteStateConfigS3) CreateS3LoggingInput() s3.PutBucketLoggingInput {
	loggingInput := s3.PutBucketLoggingInput{
		Bucket: aws.String(cfg.RemoteStateConfigS3.Bucket),
		BucketLoggingStatus: &s3types.BucketLoggingStatus{
			LoggingEnabled: &s3types.LoggingEnabled{
				TargetBucket: aws.String(cfg.AccessLoggingBucketName),
			},
		},
	}

	if cfg.AccessLoggingTargetPrefix != "" {
		loggingInput.BucketLoggingStatus.LoggingEnabled.TargetPrefix = aws.String(cfg.AccessLoggingTargetPrefix)
	}

	if cfg.AccessLoggingTargetObjectPartitionDateSource != "" {
		loggingInput.BucketLoggingStatus.LoggingEnabled.TargetObjectKeyFormat = &s3types.TargetObjectKeyFormat{
			PartitionedPrefix: &s3types.PartitionedPrefix{
				PartitionDateSource: s3types.PartitionDateSource(cfg.AccessLoggingTargetObjectPartitionDateSource),
			},
		}
	}

	return loggingInput
}

// Validate validates all the parameters of the given S3 remote state configuration.
func (cfg *ExtendedRemoteStateConfigS3) Validate() error {
	var config = cfg.RemoteStateConfigS3

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

type RemoteStateConfigS3AssumeRole struct {
	RoleArn           string            `mapstructure:"role_arn"`
	Duration          string            `mapstructure:"duration"`
	ExternalID        string            `mapstructure:"external_id"`
	Policy            string            `mapstructure:"policy"`
	PolicyArns        []string          `mapstructure:"policy_arns"`
	SessionName       string            `mapstructure:"session_name"`
	SourceIdentity    string            `mapstructure:"source_identity"`
	Tags              map[string]string `mapstructure:"tags"`
	TransitiveTagKeys []string          `mapstructure:"transitive_tag_keys"`
}

type RemoteStateConfigS3Endpoints struct {
	S3       string `mapstructure:"s3"`
	DynamoDB string `mapstructure:"dynamodb"`
}

// RemoteStateConfigS3 is a representation of the
// configuration options available for S3 remote state.
type RemoteStateConfigS3 struct {
	Endpoints        RemoteStateConfigS3Endpoints  `mapstructure:"endpoints"`
	RoleArn          string                        `mapstructure:"role_arn"`
	ExternalID       string                        `mapstructure:"external_id"`
	Region           string                        `mapstructure:"region"`
	Endpoint         string                        `mapstructure:"endpoint"`
	DynamoDBEndpoint string                        `mapstructure:"dynamodb_endpoint"`
	Bucket           string                        `mapstructure:"bucket"`
	Key              string                        `mapstructure:"key"`
	CredsFilename    string                        `mapstructure:"shared_credentials_file"`
	Profile          string                        `mapstructure:"profile"`
	SessionName      string                        `mapstructure:"session_name"`
	LockTable        string                        `mapstructure:"lock_table"`
	DynamoDBTable    string                        `mapstructure:"dynamodb_table"`
	AssumeRole       RemoteStateConfigS3AssumeRole `mapstructure:"assume_role"`
	Encrypt          bool                          `mapstructure:"encrypt"`
	S3ForcePathStyle bool                          `mapstructure:"force_path_style"`
	UseLockfile      bool                          `mapstructure:"use_lockfile"`
}

// CacheKey returns a unique key for the given S3 config that can be used to cache the initialization
func (cfg *RemoteStateConfigS3) CacheKey() string {
	return fmt.Sprintf(
		"%s-%s-%s-%s",
		cfg.Bucket,
		cfg.Region,
		cfg.LockTable,
		cfg.DynamoDBTable,
	)
}

// GetLockTableName returns the name of the DynamoDB table used for locking.
//
// The DynamoDB lock table attribute used to be called "lock_table", but has since been renamed to "dynamodb_table", and
// the old attribute name deprecated. The old attribute name has been eventually removed from Terraform starting with
// release 0.13. To maintain backwards compatibility, we support both names.
func (cfg *RemoteStateConfigS3) GetLockTableName() string {
	if cfg.DynamoDBTable != "" {
		return cfg.DynamoDBTable
	}

	return cfg.LockTable
}

// GetSessionRoleArn returns the role defined in the AssumeRole struct
// or fallback to the top level argument deprecated in Terraform 1.6
func (cfg *RemoteStateConfigS3) GetSessionRoleArn() string {
	if cfg.AssumeRole.RoleArn != "" {
		return cfg.AssumeRole.RoleArn
	}

	return cfg.RoleArn
}

func (cfg *RemoteStateConfigS3) GetSessionTags() map[string]string {
	if len(cfg.AssumeRole.Tags) != 0 {
		return cfg.AssumeRole.Tags
	}

	return nil
}

// GetExternalID returns the external ID defined in the AssumeRole struct
// or fallback to the top level argument deprecated in Terraform 1.6
// The external ID is used to prevent confused deputy attacks.
func (cfg *RemoteStateConfigS3) GetExternalID() string {
	if cfg.AssumeRole.ExternalID != "" {
		return cfg.AssumeRole.ExternalID
	}

	return cfg.ExternalID
}

func (cfg *RemoteStateConfigS3) GetSessionName() string {
	if cfg.AssumeRole.SessionName != "" {
		return cfg.AssumeRole.SessionName
	}

	return cfg.SessionName
}
