package s3

import (
	"context"
	"fmt"
	"path"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	SidRootPolicy        = "RootAccess"
	SidEnforcedTLSPolicy = "EnforcedTLS"

	s3TimeBetweenRetries  = 5 * time.Second
	s3MaxRetries          = 3
	s3SleepBetweenRetries = 10 * time.Second

	maxRetriesWaitingForS3Bucket          = 12
	sleepBetweenRetriesWaitingForS3Bucket = 5 * time.Second

	// To enable access logging in an S3 bucket, you must grant WRITE and READ_ACP permissions to the Log Delivery Group,
	// which is represented by the following URI. For more info, see:
	// https://docs.aws.amazon.com/AmazonS3/latest/dev/enable-logging-programming.html
	s3LogDeliveryGranteeURI = "http://acs.amazonaws.com/groups/s3/LogDelivery"

	// DynamoDB only allows 10 table creates/deletes simultaneously. To ensure we don't hit this error, especially when
	// running many automated tests in parallel, we use a counting semaphore
	dynamoParallelOperations = 10

	// AttrLockID is the name of the primary key for the lock table in DynamoDB.
	// OpenTofu/Terraform requires the DynamoDB table to have a primary key with this name
	AttrLockID = "LockID"

	// stateIDSuffix is last saved serial in tablestore with this suffix for consistency checks.
	stateIDSuffix = "-md5"

	// MaxRetriesWaitingForTableToBeActive is the maximum number of times we
	// will retry waiting for a table to be active.
	//
	// Default is to retry for up to 5 minutes
	MaxRetriesWaitingForTableToBeActive = 30

	// SleepBetweenTableStatusChecks is the amount of time we will sleep between
	// checks to see if a table is active.
	SleepBetweenTableStatusChecks = 10 * time.Second

	// DynamodbPayPerRequestBillingMode is the billing mode for DynamoDB tables that allows for pay-per-request billing
	// instead of provisioned capacity.
	DynamodbPayPerRequestBillingMode = "PAY_PER_REQUEST"

	sleepBetweenRetriesWaitingForEncryption = 20 * time.Second
	maxRetriesWaitingForEncryption          = 15
)

var tableCreateDeleteSemaphore = NewCountingSemaphore(dynamoParallelOperations)

type Client struct {
	*ExtendedRemoteStateConfigS3

	s3Client     *s3.Client
	dynamoClient *dynamodb.Client
	awsConfig    aws.Config

	failIfBucketCreationRequired bool
}

func NewClient(ctx context.Context, l log.Logger, config *ExtendedRemoteStateConfigS3, opts *options.TerragruntOptions) (*Client, error) {
	awsConfig := config.GetAwsSessionConfig()

	cfg, err := awshelper.CreateAwsConfig(ctx, l, awsConfig, opts)
	if err != nil {
		return nil, errors.New(err)
	}

	if !config.SkipCredentialsValidation {
		if err = awshelper.ValidateAwsConfig(ctx, cfg); err != nil {
			return nil, err
		}
	}

	s3Client, err := awshelper.CreateS3Client(ctx, l, awsConfig, opts)
	if err != nil {
		return nil, errors.New(err)
	}

	dynamoDBClient := dynamodb.NewFromConfig(cfg)
	if awsConfig.CustomDynamoDBEndpoint != "" {
		dynamoDBClient = dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(awsConfig.CustomDynamoDBEndpoint)
		})
	}

	client := &Client{
		ExtendedRemoteStateConfigS3:  config,
		s3Client:                     s3Client,
		dynamoClient:                 dynamoDBClient,
		awsConfig:                    cfg,
		failIfBucketCreationRequired: opts.FailIfBucketCreationRequired,
	}

	return client, nil
}

// CreateS3BucketIfNecessary prompts the user to create the given bucket if it doesn't already exist and if the user
// confirms, creates the bucket and enables versioning for it.
func (client *Client) CreateS3BucketIfNecessary(ctx context.Context, l log.Logger, bucketName string, opts *options.TerragruntOptions) error {
	if client.ExtendedRemoteStateConfigS3 == nil {
		return errors.Errorf("client configuration is nil - cannot create S3 bucket if necessary")
	}

	cfg := &client.ExtendedRemoteStateConfigS3.RemoteStateConfigS3

	if exists, err := client.DoesS3BucketExistWithLogging(ctx, l, cfg.Bucket); err != nil || exists {
		return err
	}

	if opts.FailIfBucketCreationRequired {
		return backend.BucketCreationNotAllowed(bucketName)
	}

	prompt := fmt.Sprintf("Remote state S3 bucket %s does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", bucketName)

	shouldCreateBucket, err := shell.PromptUserForYesNo(ctx, l, prompt, opts)
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
		description := "Create S3 bucket with retry " + bucketName

		return util.DoWithRetry(ctx, description, s3MaxRetries, s3SleepBetweenRetries, l, log.DebugLevel, func(ctx context.Context) error {
			err := client.CreateS3BucketWithVersioningSSEncryptionAndAccessLogging(ctx, l, opts)
			if err != nil {
				if isBucketErrorRetriable(err) {
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

func (client *Client) UpdateS3BucketIfNecessary(ctx context.Context, l log.Logger, bucketName string, opts *options.TerragruntOptions) error {
	if exists, err := client.DoesS3BucketExistWithLogging(ctx, l, bucketName); err != nil {
		return err
	} else if !exists && opts.FailIfBucketCreationRequired {
		return backend.BucketCreationNotAllowed(bucketName)
	}

	needsUpdate, bucketUpdatesRequired, err := client.checkIfS3BucketNeedsUpdate(ctx, l, bucketName)
	if err != nil {
		return err
	}

	if !needsUpdate {
		l.Debug("S3 bucket is already up to date")
		return nil
	}

	prompt := fmt.Sprintf("Remote state S3 bucket %s is out of date. Would you like Terragrunt to update it?", bucketName)

	shouldUpdateBucket, err := shell.PromptUserForYesNo(ctx, l, prompt, opts)
	if err != nil {
		return err
	}

	if !shouldUpdateBucket {
		return nil
	}

	if bucketUpdatesRequired.Versioning {
		if client.SkipBucketVersioning {
			l.Debugf("Versioning is disabled for the remote state S3 bucket %s using 'skip_bucket_versioning' config.", bucketName)
		} else if err := client.EnableVersioningForS3Bucket(ctx, l, bucketName); err != nil {
			return err
		}
	}

	if bucketUpdatesRequired.SSEEncryption {
		msg := fmt.Sprintf("Encryption is not enabled on the S3 remote state bucket %s. Terraform state files may contain secrets, so we STRONGLY recommend enabling encryption!", bucketName)

		if client.SkipBucketSSEncryption {
			l.Debug(msg)
			l.Debugf("Server-Side Encryption enabling is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_ssencryption' config.", bucketName)

			return nil
		} else {
			l.Warn(msg)
		}

		l.Infof("Enabling Server-Side Encryption for the remote state AWS S3 bucket %s.", bucketName)

		if err := client.EnableSSEForS3BucketWide(ctx, l, bucketName, client.FetchEncryptionAlgorithm()); err != nil {
			l.Errorf("Failed to enable Server-Side Encryption for the remote state AWS S3 bucket %s: %v", bucketName, err)
			return err
		}

		l.Infof("Successfully enabled Server-Side Encryption for the remote state AWS S3 bucket %s.", bucketName)
	}

	if bucketUpdatesRequired.RootAccess {
		if client.SkipBucketRootAccess {
			l.Debugf("Root access is disabled for the remote state S3 bucket %s using 'skip_bucket_root_access' config.", bucketName)
		} else if err := client.EnableRootAccesstoS3Bucket(ctx, l); err != nil {
			return err
		}
	}

	if bucketUpdatesRequired.EnforcedTLS {
		if client.SkipBucketEnforcedTLS {
			l.Debugf("Enforced TLS is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_enforced_tls' config.", bucketName)
		} else if err := client.EnableEnforcedTLSAccesstoS3Bucket(ctx, l, bucketName); err != nil {
			return err
		}
	}

	if bucketUpdatesRequired.AccessLogging {
		if client.SkipBucketAccessLogging {
			l.Debugf("Access logging is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_access_logging' config.", bucketName)
		} else {
			if client.AccessLoggingBucketName != "" {
				if err := client.configureAccessLogBucket(ctx, l, opts); err != nil {
					// TODO: Remove lint suppression
					return nil //nolint:nilerr
				}
			} else {
				l.Debugf("Access Logging is disabled for the remote state AWS S3 bucket %s", bucketName)
			}
		}
	}

	if bucketUpdatesRequired.PublicAccess {
		if client.SkipBucketPublicAccessBlocking {
			l.Debugf("Public access blocking is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_public_access_blocking' config.", bucketName)
		} else if err := client.EnablePublicAccessBlockingForS3Bucket(ctx, l, bucketName); err != nil {
			return err
		}
	}

	return nil
}

// configureAccessLogBucket - configure access log bucket.
func (client *Client) configureAccessLogBucket(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	if client.ExtendedRemoteStateConfigS3 == nil {
		return errors.Errorf("client configuration is nil - cannot configure access log bucket")
	}

	cfg := &client.ExtendedRemoteStateConfigS3.RemoteStateConfigS3

	l.Debugf("Enabling bucket-wide Access Logging on AWS S3 bucket %s - using as TargetBucket %s", cfg.Bucket, client.AccessLoggingBucketName)

	if err := client.CreateLogsS3BucketIfNecessary(ctx, l, client.AccessLoggingBucketName, opts); err != nil {
		l.Errorf("Could not create logs bucket %s for AWS S3 bucket %s\n%s", client.AccessLoggingBucketName, cfg.Bucket, err.Error())

		return err
	}

	if !client.SkipAccessLoggingBucketPublicAccessBlocking {
		if err := client.EnablePublicAccessBlockingForS3Bucket(ctx, l, client.AccessLoggingBucketName); err != nil {
			l.Errorf("Could not enable public access blocking on %s\n%s", client.AccessLoggingBucketName, err.Error())

			return err
		}
	}

	if err := client.EnableAccessLoggingForS3BucketWide(ctx, l); err != nil {
		l.Errorf("Could not enable access logging on %s\n%s", cfg.Bucket, err.Error())

		return err
	}

	if !client.SkipAccessLoggingBucketSSEncryption {
		if err := client.EnableSSEForS3BucketWide(ctx, l, client.AccessLoggingBucketName, string(types.ServerSideEncryptionAes256)); err != nil {
			l.Errorf("Could not enable encryption on %s\n%s", client.AccessLoggingBucketName, err.Error())

			return err
		}
	}

	if !client.SkipAccessLoggingBucketEnforcedTLS {
		if err := client.EnableEnforcedTLSAccesstoS3Bucket(ctx, l, client.AccessLoggingBucketName); err != nil {
			l.Errorf("Could not enable TLS access on %s\n%s", client.AccessLoggingBucketName, err.Error())

			return err
		}
	}

	if client.SkipBucketVersioning {
		l.Debugf("Versioning is disabled for the remote state S3 bucket %s using 'skip_bucket_versioning' config.", client.AccessLoggingBucketName)
	} else if err := client.EnableVersioningForS3Bucket(ctx, l, client.AccessLoggingBucketName); err != nil {
		return err
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

func (client *Client) checkIfS3BucketNeedsUpdate(ctx context.Context, l log.Logger, bucketName string) (bool, S3BucketUpdatesRequired, error) {
	var (
		updates  []string
		toUpdate S3BucketUpdatesRequired
	)

	if !client.SkipBucketVersioning {
		enabled, err := client.CheckIfVersioningEnabled(ctx, l, bucketName)
		if err != nil {
			return false, toUpdate, err
		}

		if !enabled {
			toUpdate.Versioning = true

			updates = append(updates, "Bucket Versioning")
		}
	}

	if !client.SkipBucketSSEncryption {
		matches, err := client.checkIfSSEForS3MatchesConfig(ctx, bucketName)
		if err != nil {
			return false, toUpdate, err
		}

		if !matches {
			toUpdate.SSEEncryption = true

			updates = append(updates, "Bucket Server-Side Encryption")
		}
	}

	if !client.SkipBucketRootAccess {
		enabled, err := client.checkIfBucketRootAccess(ctx, l, bucketName)
		if err != nil {
			return false, toUpdate, err
		}

		if !enabled {
			toUpdate.RootAccess = true

			updates = append(updates, "Bucket Root Access")
		}
	}

	if !client.SkipBucketEnforcedTLS {
		enabled, err := client.checkIfBucketEnforcedTLS(ctx, l, bucketName)
		if err != nil {
			return false, toUpdate, err
		}

		if !enabled {
			toUpdate.EnforcedTLS = true

			updates = append(updates, "Bucket Enforced TLS")
		}
	}

	if !client.SkipBucketAccessLogging && client.AccessLoggingBucketName != "" {
		enabled, err := client.checkS3AccessLoggingConfiguration(ctx, bucketName)
		if err != nil {
			return false, toUpdate, err
		}

		if !enabled {
			toUpdate.AccessLogging = true

			updates = append(updates, "Bucket Access Logging")
		}
	}

	if !client.SkipBucketPublicAccessBlocking {
		enabled, err := client.checkIfS3PublicAccessBlockingEnabled(ctx, bucketName)
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
		l.Warnf("The remote state S3 bucket %s needs to be updated:", bucketName)

		for _, update := range updates {
			l.Warnf("  - %s", update)
		}

		return true, toUpdate, nil
	}

	return false, toUpdate, nil
}

// CheckIfVersioningEnabled checks if versioning is enabled for the S3 bucket specified in the given config and warn the user if it is not
func (client *Client) CheckIfVersioningEnabled(ctx context.Context, l log.Logger, bucketName string) (bool, error) {
	if exists, err := client.DoesS3BucketExist(ctx, bucketName); err != nil {
		return false, err
	} else if !exists {
		return false, backend.NewBucketDoesNotExistError(bucketName)
	}

	l.Debugf("Verifying AWS S3 bucket versioning %s", bucketName)

	res, err := client.s3Client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucketName)})
	if err != nil {
		return false, errors.New(err)
	}

	// NOTE: There must be a bug in the AWS SDK since res == nil when versioning is not enabled. In the future,
	// check the AWS SDK for updates to see if we can remove "res == nil ||".
	if res == nil || res.Status != types.BucketVersioningStatusEnabled {
		l.Warnf("Versioning is not enabled for the remote state S3 bucket %s. We recommend enabling versioning so that you can roll back to previous versions of your OpenTofu/Terraform state in case of error.", bucketName)
		return false, nil
	}

	return true, nil
}

// CreateS3BucketWithVersioningSSEncryptionAndAccessLogging creates the given S3 bucket and enable versioning for it.
func (client *Client) CreateS3BucketWithVersioningSSEncryptionAndAccessLogging(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	if client.ExtendedRemoteStateConfigS3 == nil {
		return errors.Errorf("client configuration is nil - cannot create S3 bucket")
	}

	cfg := &client.ExtendedRemoteStateConfigS3.RemoteStateConfigS3

	l.Debugf("Create S3 bucket %s with versioning, SSE encryption, and access logging.", cfg.Bucket)

	err := client.CreateS3Bucket(ctx, l, cfg.Bucket)
	if err != nil {
		if accessError := client.checkBucketAccess(ctx, cfg.Bucket, cfg.Key); accessError != nil {
			return accessError
		}

		if isBucketAlreadyOwnedByYouError(err) {
			l.Debugf("Looks like you're already creating bucket %s at the same time. Will not attempt to create it again.", cfg.Bucket)
			return client.WaitUntilS3BucketExists(ctx, l)
		}

		return err
	}

	if err := client.WaitUntilS3BucketExists(ctx, l); err != nil {
		return err
	}

	if client.SkipBucketRootAccess {
		l.Debugf("Root access is disabled for the remote state S3 bucket %s using 'skip_bucket_root_access' config.", cfg.Bucket)
	} else if err := client.EnableRootAccesstoS3Bucket(ctx, l); err != nil {
		return err
	}

	if client.SkipBucketEnforcedTLS {
		l.Debugf("TLS enforcement is disabled for the remote state S3 bucket %s using 'skip_bucket_enforced_tls' config.", cfg.Bucket)
	} else if err := client.EnableEnforcedTLSAccesstoS3Bucket(ctx, l, cfg.Bucket); err != nil {
		return err
	}

	if client.SkipBucketPublicAccessBlocking {
		l.Debugf("Public access blocking is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_public_access_blocking' config.", cfg.Bucket)
	} else if err := client.EnablePublicAccessBlockingForS3Bucket(ctx, l, cfg.Bucket); err != nil {
		return err
	}

	if err := client.TagS3Bucket(ctx, l); err != nil {
		return err
	}

	if client.SkipBucketVersioning {
		l.Debugf("Versioning is disabled for the remote state S3 bucket %s using 'skip_bucket_versioning' config.", cfg.Bucket)
	} else if err := client.EnableVersioningForS3Bucket(ctx, l, cfg.Bucket); err != nil {
		return err
	}

	if client.SkipBucketSSEncryption {
		l.Debugf("Server-Side Encryption is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_ssencryption' config.", cfg.Bucket)
	} else if err := client.EnableSSEForS3BucketWide(ctx, l, cfg.Bucket, client.FetchEncryptionAlgorithm()); err != nil {
		return err
	}

	if client.SkipBucketAccessLogging {
		l.Warnf("Terragrunt configuration option 'skip_bucket_accesslogging' is now deprecated. Access logging for the state bucket %s is disabled by default. To enable access logging for bucket %s, please provide property `accesslogging_bucket_name` in the terragrunt config file. For more details, please refer to the Terragrunt documentation.", cfg.Bucket, cfg.Bucket)
	}

	if client.AccessLoggingBucketName != "" {
		if err := client.configureAccessLogBucket(ctx, l, opts); err != nil {
			// TODO: Remove lint suppression
			return nil //nolint:nilerr
		}
	} else {
		l.Debugf("Access Logging is disabled for the remote state AWS S3 bucket %s", cfg.Bucket)
	}

	if err := client.TagS3BucketAccessLogging(ctx, l); err != nil {
		return err
	}

	return nil
}

func (client *Client) CreateLogsS3BucketIfNecessary(ctx context.Context, l log.Logger, logsBucketName string, opts *options.TerragruntOptions) error {
	if exists, err := client.DoesS3BucketExistWithLogging(ctx, l, logsBucketName); err != nil || exists {
		return err
	}

	if client.failIfBucketCreationRequired {
		return backend.BucketCreationNotAllowed(logsBucketName)
	}

	prompt := fmt.Sprintf("Logs S3 bucket %s for the remote state does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", logsBucketName)

	shouldCreateBucket, err := shell.PromptUserForYesNo(ctx, l, prompt, opts)
	if err != nil {
		return err
	}

	if shouldCreateBucket {
		return client.CreateS3Bucket(ctx, l, logsBucketName)
	}

	return nil
}

func (client *Client) TagS3BucketAccessLogging(ctx context.Context, l log.Logger) error {
	if len(client.AccessLoggingBucketTags) == 0 {
		l.Debugf("No tags specified for bucket %s.", client.AccessLoggingBucketName)
		return nil
	}

	// There must be one entry in the list
	var tagsConverted = convertTags(client.AccessLoggingBucketTags)

	l.Debugf("Tagging S3 bucket with %s", client.AccessLoggingBucketTags)

	putBucketTaggingInput := s3.PutBucketTaggingInput{
		Bucket: aws.String(client.AccessLoggingBucketName),
		Tagging: &types.Tagging{
			TagSet: tagsConverted,
		},
	}

	_, err := client.s3Client.PutBucketTagging(ctx, &putBucketTaggingInput)
	if err != nil {
		if handleS3TaggingMethodNotAllowed(err, l, "access logging bucket") {
			return nil
		}

		return errors.New(err)
	}

	l.Debugf("Tagged S3 bucket with %s", client.AccessLoggingBucketTags)

	return nil
}

// TagS3Bucket tags the S3 bucket with the tags specified in the config.
func (client *Client) TagS3Bucket(ctx context.Context, l log.Logger) error {
	if client.ExtendedRemoteStateConfigS3 == nil {
		return errors.Errorf("client configuration is nil - cannot tag S3 bucket")
	}

	cfg := &client.ExtendedRemoteStateConfigS3.RemoteStateConfigS3

	if len(client.S3BucketTags) == 0 {
		l.Debugf("No tags to apply to S3 bucket %s", cfg.Bucket)
		return nil
	}

	l.Debugf("Tagging S3 bucket %s with %s", cfg.Bucket, client.S3BucketTags)

	tagsConverted := convertTags(client.S3BucketTags)

	putBucketTaggingInput := s3.PutBucketTaggingInput{
		Bucket: aws.String(cfg.Bucket),
		Tagging: &types.Tagging{
			TagSet: tagsConverted,
		},
	}

	_, err := client.s3Client.PutBucketTagging(ctx, &putBucketTaggingInput)
	if err != nil {
		if handleS3TaggingMethodNotAllowed(err, l, cfg.Bucket) {
			return nil
		}

		return errors.New(err)
	}

	l.Debugf("Tagged S3 bucket with %s", client.S3BucketTags)

	return nil
}

func convertTags(tags map[string]string) []types.Tag {
	var tagsConverted = make([]types.Tag, 0, len(tags))

	for k, v := range tags {
		var tag = types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v)}

		tagsConverted = append(tagsConverted, tag)
	}

	return tagsConverted
}

// WaitUntilS3BucketExists waits until the given S3 bucket exists.
//
// AWS is eventually consistent, so after creating an S3 bucket, this method can be used to wait until the information
// about that S3 bucket has propagated everywhere.
func (client *Client) WaitUntilS3BucketExists(ctx context.Context, l log.Logger) error {
	if client.ExtendedRemoteStateConfigS3 == nil {
		return errors.Errorf("client configuration is nil - cannot wait for S3 bucket")
	}

	cfg := &client.ExtendedRemoteStateConfigS3.RemoteStateConfigS3

	l.Debugf("Waiting for bucket %s to be created", cfg.Bucket)

	for retries := range maxRetriesWaitingForS3Bucket {
		if exists, err := client.DoesS3BucketExistWithLogging(ctx, l, cfg.Bucket); err != nil {
			return err
		} else if exists {
			l.Debugf("S3 bucket %s created.", cfg.Bucket)

			return nil
		} else if retries < maxRetriesWaitingForS3Bucket-1 {
			l.Debugf("S3 bucket %s has not been created yet. Sleeping for %s and will check again.", cfg.Bucket, sleepBetweenRetriesWaitingForS3Bucket)
			time.Sleep(sleepBetweenRetriesWaitingForS3Bucket)
		}
	}

	return errors.New(MaxRetriesWaitingForS3BucketExceeded(cfg.Bucket))
}

// CreateS3Bucket creates the S3 bucket specified in the given config.
func (client *Client) CreateS3Bucket(ctx context.Context, l log.Logger, bucket string) error {
	if client.s3Client == nil {
		return errors.Errorf("S3 client is nil - cannot create S3 bucket %s", bucket)
	}

	l.Debugf("Creating S3 bucket %s", bucket)

	input := &s3.CreateBucketInput{
		Bucket:          aws.String(bucket),
		ObjectOwnership: types.ObjectOwnershipObjectWriter,
	}

	// For regions other than us-east-1, we need to specify the location constraint
	// to avoid IllegalLocationConstraintException
	region := client.awsConfig.Region
	if region != "us-east-1" && region != "" {
		l.Debugf("Creating S3 bucket %s in region %s", bucket, region)
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		}
	}

	_, err := client.s3Client.CreateBucket(ctx, input)
	if err != nil {
		return errors.New(err)
	}

	l.Debugf("Created S3 bucket %s", bucket)

	return nil
}

// or is in progress. This usually happens when running many tests in parallel or xxx-all commands.
func isBucketAlreadyOwnedByYouError(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "BucketAlreadyOwnedByYou" || apiErr.ErrorCode() == "OperationAborted"
	}

	return false
}

// isBucketErrorRetriable returns true if the error is temporary and can be retried.
func isBucketErrorRetriable(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "InternalError" || apiErr.ErrorCode() == "OperationAborted" || apiErr.ErrorCode() == "InvalidParameter"
	}

	// If it's not a known AWS error, assume it's retriable
	return true
}

// EnableRootAccesstoS3Bucket adds a policy to allow root access to the bucket.
func (client *Client) EnableRootAccesstoS3Bucket(ctx context.Context, l log.Logger) error {
	if client.ExtendedRemoteStateConfigS3 == nil {
		return errors.Errorf("client configuration is nil - cannot enable root access to S3 bucket")
	}

	if client.s3Client == nil {
		return errors.Errorf("S3 client is nil - cannot enable root access to S3 bucket")
	}

	// Access bucket name safely through defensive checking
	config := client.ExtendedRemoteStateConfigS3

	bucket := config.RemoteStateConfigS3.Bucket
	if bucket == "" {
		return errors.Errorf("S3 bucket name is empty - cannot enable root access to S3 bucket")
	}

	l.Debugf("Enabling root access to S3 bucket %s", bucket)

	if client.awsConfig.Region == "" {
		return errors.Errorf("AWS config region is empty - cannot enable root access to S3 bucket %s", bucket)
	}

	accountID, err := awshelper.GetAWSAccountID(ctx, client.awsConfig)
	if err != nil {
		return errors.Errorf("error getting AWS account ID %s for bucket %s: %w", accountID, bucket, err)
	}

	if accountID == "" {
		return errors.Errorf("AWS account ID is empty - cannot enable root access to S3 bucket %s", bucket)
	}

	partition, err := awshelper.GetAWSPartition(ctx, client.awsConfig)
	if err != nil {
		return errors.Errorf("error getting AWS partition %s for bucket %s: %w", partition, bucket, err)
	}

	if partition == "" {
		return errors.Errorf("AWS partition is empty - cannot enable root access to S3 bucket %s", bucket)
	}

	var policyInBucket awshelper.Policy

	policyOutput, err := client.s3Client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{
		Bucket: aws.String(bucket),
	})

	// If there's no policy, we need to create one
	if err != nil {
		l.Debugf("Policy not exists for bucket %s", bucket)
	}

	if policyOutput != nil && policyOutput.Policy != nil {
		l.Debugf("Policy already exists for bucket %s", bucket)

		policyInBucket, err = awshelper.UnmarshalPolicy(*policyOutput.Policy)
		if err != nil {
			return errors.Errorf("error unmarshalling policy for bucket %s: %w", bucket, err)
		}
	}

	// Ensure Statement is never nil to avoid nil pointer dereference
	if policyInBucket.Statement == nil {
		policyInBucket.Statement = []awshelper.Statement{}
	}

	// Iterate over statements to check if root policy already exists
	for _, statement := range policyInBucket.Statement {
		if statement.Sid == SidRootPolicy {
			l.Debugf("Policy for RootAccess already exists for bucket %s", bucket)
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

	_, err = client.s3Client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(string(policy)),
	})
	if err != nil {
		return errors.Errorf("error putting policy for bucket %s: %w", bucket, err)
	}

	l.Debugf("Enabled root access to bucket %s", bucket)

	return nil
}

func (client *Client) EnableEnforcedTLSAccesstoS3Bucket(ctx context.Context, l log.Logger, bucket string) error {
	partition, err := awshelper.GetAWSPartition(ctx, client.awsConfig)
	if err != nil {
		return errors.Errorf("error getting AWS partition %s for bucket %s: %w", partition, bucket, err)
	}

	policyOutput, err := client.s3Client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		l.Debugf("Policy not exists for bucket %s", bucket)
	}

	var policyInBucket awshelper.Policy
	if policyOutput != nil && policyOutput.Policy != nil {
		policyInBucket, err = awshelper.UnmarshalPolicy(*policyOutput.Policy)
		if err != nil {
			return errors.Errorf("error unmarshalling policy for bucket %s: %w", bucket, err)
		}
	}

	// Ensure Statement is never nil to avoid nil pointer dereference
	if policyInBucket.Statement == nil {
		policyInBucket.Statement = []awshelper.Statement{}
	}

	for _, statement := range policyInBucket.Statement {
		if statement.Sid == SidEnforcedTLSPolicy {
			l.Debugf("Policy for EnforcedTLS already exists for bucket %s", bucket)
			return nil
		}
	}

	enforcedTLSPolicy := awshelper.Policy{
		Version: "2012-10-17",
		Statement: []awshelper.Statement{
			{
				Sid:    SidEnforcedTLSPolicy,
				Effect: "Deny",
				Action: "s3:*",
				Resource: []string{
					"arn:" + partition + ":s3:::" + bucket,
					"arn:" + partition + ":s3:::" + bucket + "/*",
				},
				Principal: map[string][]string{"*": {"*"}},
				Condition: &map[string]any{
					"Bool": map[string]any{"aws:SecureTransport": "false"},
				},
			},
		},
	}

	enforcedTLSPolicy.Statement = append(enforcedTLSPolicy.Statement, policyInBucket.Statement...)

	policy, err := awshelper.MarshalPolicy(enforcedTLSPolicy)
	if err != nil {
		return errors.Errorf("error marshalling policy for bucket %s: %w", bucket, err)
	}

	_, err = client.s3Client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(string(policy)),
	})
	if err != nil {
		return errors.Errorf("error putting policy for bucket %s: %w", bucket, err)
	}

	l.Debugf("Enabled enforced TLS access to bucket %s", bucket)

	return nil
}

func (client *Client) EnablePublicAccessBlockingForS3Bucket(ctx context.Context, l log.Logger, bucketName string) error {
	input := &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
		PublicAccessBlockConfiguration: &types.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(true),
			IgnorePublicAcls:      aws.Bool(true),
			BlockPublicPolicy:     aws.Bool(true),
			RestrictPublicBuckets: aws.Bool(true),
		},
	}

	_, err := client.s3Client.PutPublicAccessBlock(ctx, input)
	if err != nil {
		return errors.New(err)
	}

	l.Debugf("Enabled public access blocking for S3 bucket %s", bucketName)

	return nil
}

func (client *Client) EnableAccessLoggingForS3BucketWide(ctx context.Context, l log.Logger) error {
	if client.ExtendedRemoteStateConfigS3 == nil {
		return errors.Errorf("client configuration is nil - cannot enable access logging for S3 bucket")
	}

	cfg := client.ExtendedRemoteStateConfigS3
	bucket := cfg.RemoteStateConfigS3.Bucket
	logsBucket := cfg.AccessLoggingBucketName
	logsBucketPrefix := cfg.AccessLoggingTargetPrefix

	if logsBucket == "" {
		return errors.Errorf("AccessLoggingBucketName is required for bucket-wide Access Logging on AWS S3 bucket %s", cfg.RemoteStateConfigS3.Bucket)
	}

	if !client.SkipAccessLoggingBucketACL {
		if err := client.configureBucketAccessLoggingACL(ctx, l, logsBucket); err != nil {
			return errors.Errorf("error configuring bucket access logging ACL on S3 bucket %s: %w", cfg.RemoteStateConfigS3.Bucket, err)
		}
	}

	loggingInput := client.CreateS3LoggingInput()
	l.Debugf("Putting bucket logging on S3 bucket %s with TargetBucket %s and TargetPrefix %s\n%s", bucket, logsBucket, logsBucketPrefix, loggingInput)

	if _, err := client.s3Client.PutBucketLogging(ctx, &loggingInput); err != nil {
		return errors.Errorf("error enabling bucket-wide Access Logging on AWS S3 bucket %s: %w", cfg.RemoteStateConfigS3.Bucket, err)
	}

	l.Debugf("Enabled bucket-wide Access Logging on AWS S3 bucket %s", bucket)

	return nil
}

func (client *Client) configureBucketAccessLoggingACL(ctx context.Context, l log.Logger, bucketName string) error {
	l.Debugf("Granting WRITE and READ_ACP permissions to S3 Log Delivery (%s) for bucket %s. This is required for access logging.", s3LogDeliveryGranteeURI, bucketName)

	uri := "uri=" + s3LogDeliveryGranteeURI
	aclInput := s3.PutBucketAclInput{
		Bucket:       aws.String(bucketName),
		GrantWrite:   aws.String(uri),
		GrantReadACP: aws.String(uri),
	}

	if _, err := client.s3Client.PutBucketAcl(ctx, &aclInput); err != nil {
		return errors.Errorf("error granting WRITE and READ_ACP permissions to S3 Log Delivery (%s) for bucket %s: %w", s3LogDeliveryGranteeURI, bucketName, err)
	}

	return client.waitUntilBucketHasAccessLoggingACL(ctx, l, bucketName)
}

func (client *Client) waitUntilBucketHasAccessLoggingACL(ctx context.Context, l log.Logger, bucketName string) error {
	l.Debugf("Waiting for ACL bucket %s to have the updated ACL for access logging.", bucketName)

	maxRetries := 10

	for range maxRetries {
		res, err := client.s3Client.GetBucketAcl(ctx, &s3.GetBucketAclInput{Bucket: aws.String(bucketName)})
		if err != nil {
			return errors.Errorf("error getting ACL for bucket %s: %w", bucketName, err)
		}

		hasReadAcp := false
		hasWrite := false

		for _, grant := range res.Grants {
			if aws.ToString(grant.Grantee.URI) == s3LogDeliveryGranteeURI {
				if string(grant.Permission) == "READ_ACP" {
					hasReadAcp = true
				}

				if string(grant.Permission) == "WRITE" {
					hasWrite = true
				}
			}
		}

		if hasReadAcp && hasWrite {
			l.Debugf("Bucket %s now has the proper ACL permissions for access logging!", bucketName)
			return nil
		}

		l.Debugf("Bucket %s still does not have the ACL permissions for access logging. Will sleep for %v and check again.", bucketName, s3TimeBetweenRetries)
		time.Sleep(s3TimeBetweenRetries)
	}

	return errors.New(MaxRetriesWaitingForS3ACLExceeded(bucketName))
}

// checkBucketAccess checks if the current user has the ability to access the S3 bucket keys.
func (client *Client) checkBucketAccess(ctx context.Context, bucket, key string) error {
	_, err := client.s3Client.GetObject(ctx, &s3.GetObjectInput{Key: aws.String(key), Bucket: aws.String(bucket)})
	if err == nil {
		return nil
	}

	var apiErr smithy.APIError

	if ok := errors.As(err, &apiErr); !ok {
		return errors.Errorf("error checking access to S3 bucket %s: %w", bucket, err)
	}

	return errors.Errorf("error checking access to S3 bucket %s: %w", bucket, err)
}

// DeleteS3BucketIfNecessary deletes the given S3 bucket with all its objects if it exists.
func (client *Client) DeleteS3BucketIfNecessary(ctx context.Context, l log.Logger, bucketName string) error {
	if exists, err := client.DoesS3BucketExistWithLogging(ctx, l, bucketName); err != nil || !exists {
		return err
	}

	description := fmt.Sprintf("Delete S3 bucket %s with retry", bucketName)

	return util.DoWithRetry(ctx, description, s3MaxRetries, s3SleepBetweenRetries, l, log.DebugLevel, func(ctx context.Context) error {
		err := client.DeleteS3BucketWithAllObjects(ctx, l, bucketName)
		if err == nil {
			return nil
		}

		if isBucketErrorRetriable(err) {
			return err
		}
		// return FatalError so that retry loop will not continue
		return util.FatalError{Underlying: err}
	})
}

// DeleteS3BucketWithAllObjects deletes the given S3 bucket with all its objects.
func (client *Client) DeleteS3BucketWithAllObjects(ctx context.Context, l log.Logger, bucketName string) error {
	l.Debugf("Delete S3 bucket %s with all objects.", bucketName)

	if err := client.DeleteS3BucketObjects(ctx, l, bucketName); err != nil {
		return err
	}

	return client.DeleteS3Bucket(ctx, l, bucketName)
}

// DeleteS3BucketObject deletes S3 bucket object by the given key.
func (client *Client) DeleteS3BucketObject(ctx context.Context, l log.Logger, bucketName, key string, versionID *string) error {
	objectInput := &s3.DeleteObjectInput{
		Bucket:    aws.String(bucketName),
		Key:       aws.String(key),
		VersionId: versionID,
	}

	if _, err := client.s3Client.DeleteObject(ctx, objectInput); err != nil {
		return errors.Errorf("failed to delete object: %w", err)
	}

	return nil
}

// DeleteS3BucketV2Objects deletes S3 bucket object by the given key.
func (client *Client) DeleteS3BucketV2Objects(ctx context.Context, l log.Logger, bucketName string) error {
	var v2Input = &s3.ListObjectsV2Input{Bucket: aws.String(bucketName)}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		res, err := client.s3Client.ListObjectsV2(ctx, v2Input)
		if err != nil {
			return errors.Errorf("failed to list objects: %w", err)
		}

		for _, item := range res.Contents {
			if err := client.DeleteS3BucketObject(ctx, l, bucketName, aws.ToString(item.Key), nil); err != nil {
				return err
			}
		}

		if !aws.ToBool(res.IsTruncated) {
			break
		}

		v2Input.ContinuationToken = res.ContinuationToken
	}

	return nil
}

// DeleteS3BucketVersionObjects deletes S3 bucket object versions by the given key.
func (client *Client) DeleteS3BucketVersionObjects(ctx context.Context, l log.Logger, bucketName string, keys ...string) error {
	var versionsInput = &s3.ListObjectVersionsInput{Bucket: aws.String(bucketName)}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		res, err := client.s3Client.ListObjectVersions(ctx, versionsInput)
		if err != nil {
			return errors.Errorf("failed to list version objects: %w", err)
		}

		for _, item := range res.DeleteMarkers {
			if len(keys) != 0 && !slices.Contains(keys, aws.ToString(item.Key)) {
				continue
			}

			if err := client.DeleteS3BucketObject(ctx, l, bucketName, aws.ToString(item.Key), item.VersionId); err != nil {
				return err
			}
		}

		for _, item := range res.Versions {
			if len(keys) != 0 && !slices.Contains(keys, aws.ToString(item.Key)) {
				continue
			}

			if err := client.DeleteS3BucketObject(ctx, l, bucketName, aws.ToString(item.Key), item.VersionId); err != nil {
				return err
			}
		}

		if !aws.ToBool(res.IsTruncated) {
			break
		}

		versionsInput.VersionIdMarker = res.NextVersionIdMarker
		versionsInput.KeyMarker = res.NextKeyMarker
	}

	return nil
}

// DeleteS3BucketObjects deletes the S3 bucket contents.
func (client *Client) DeleteS3BucketObjects(ctx context.Context, l log.Logger, bucketName string) error {
	if err := client.DeleteS3BucketV2Objects(ctx, l, bucketName); err != nil {
		return err
	}

	return client.DeleteS3BucketVersionObjects(ctx, l, bucketName)
}

// DeleteS3Bucket deletes the S3 bucket specified in the given config.
func (client *Client) DeleteS3Bucket(ctx context.Context, l log.Logger, bucketName string) error {
	var (
		cfg         = &client.ExtendedRemoteStateConfigS3.RemoteStateConfigS3
		key         = cfg.Key
		bucketInput = &s3.DeleteBucketInput{Bucket: aws.String(bucketName)}
	)

	l.Debugf("Deleting S3 bucket %s", bucketName)

	if _, err := client.s3Client.DeleteBucket(ctx, bucketInput); err != nil {
		if err := client.checkBucketAccess(ctx, bucketName, key); err != nil {
			return err
		}

		return errors.New(err)
	}

	l.Debugf("Deleted S3 bucket %s", bucketName)

	return client.WaitUntilS3BucketDeleted(ctx, l, bucketName)
}

// WaitUntilS3BucketDeleted waits until the given S3 bucket is deleted.
func (client *Client) WaitUntilS3BucketDeleted(ctx context.Context, l log.Logger, bucketName string) error {
	l.Debugf("Waiting for bucket %s to be deleted", bucketName)

	for retries := range maxRetriesWaitingForS3Bucket {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if exists, err := client.DoesS3BucketExist(ctx, bucketName); err != nil {
			return err
		} else if !exists {
			l.Debugf("S3 bucket %s deleted.", bucketName)
			return nil
		} else if retries < maxRetriesWaitingForS3Bucket-1 {
			l.Debugf("S3 bucket %s has not been deleted yet. Sleeping for %s and will check again.", bucketName, sleepBetweenRetriesWaitingForS3Bucket)
			time.Sleep(sleepBetweenRetriesWaitingForS3Bucket)
		}
	}

	return errors.New(MaxRetriesWaitingForS3BucketExceeded(bucketName))
}

// DeleteS3ObjectIfNecessary deletes the S3 object by the specified key if it exists.
func (client *Client) DeleteS3ObjectIfNecessary(ctx context.Context, l log.Logger, bucketName, key string) error {
	if exists, err := client.DoesS3BucketExistWithLogging(ctx, l, bucketName); err != nil || !exists {
		return err
	}

	if exists, err := client.DoesS3ObjectExist(ctx, bucketName, key); err != nil || !exists {
		return err
	}

	description := fmt.Sprintf("Delete S3 object %s in bucket %s with retry", key, bucketName)

	return util.DoWithRetry(ctx, description, s3MaxRetries, s3SleepBetweenRetries, l, log.DebugLevel, func(ctx context.Context) error {
		if err := client.DeleteS3BucketObject(ctx, l, bucketName, key, nil); err != nil {
			if isBucketErrorRetriable(err) {
				return err
			}
			// return FatalError so that retry loop will not continue
			return util.FatalError{Underlying: err}
		}

		return nil
	})
}

// DoesS3ObjectExist returns true if the specified S3 object exists otherwise false.
func (client *Client) DoesS3ObjectExist(ctx context.Context, bucketName, key string) (bool, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	}

	if _, err := client.s3Client.HeadObject(ctx, input); err != nil {
		var apiErr smithy.APIError
		if ok := errors.As(err, &apiErr); ok {
			if apiErr.ErrorCode() == "NotFound" { // s3.ErrCodeNoSuchKey does not work, aws is missing this error code so we hardwire a string
				return false, nil
			}
		}

		return false, err
	}

	return true, nil
}

func (client *Client) DoesS3ObjectExistWithLogging(ctx context.Context, l log.Logger, bucketName, key string) (bool, error) {
	if client.s3Client == nil {
		return false, errors.Errorf("S3 client is nil - cannot check if S3 bucket %s exists", bucketName)
	}

	l.Debugf("Checking if bucket %s exists", bucketName)

	if exists, err := client.DoesS3ObjectExist(ctx, bucketName, key); err != nil || exists {
		return exists, err
	}

	l.Debugf("Remote state S3 bucket %s object %s does not exist or you don't have permissions to access it.", bucketName, key)

	return false, nil
}

// CreateLockTableIfNecessary creates the lock table in DynamoDB if it doesn't already exist.
func (client *Client) CreateLockTableIfNecessary(ctx context.Context, l log.Logger, tableName string, tags map[string]string) error {
	tableExists, err := client.DoesLockTableExistAndIsActive(ctx, tableName)
	if err != nil {
		return err
	}

	if !tableExists {
		l.Debugf("Lock table %s does not exist in DynamoDB. Will need to create it just this first time.", tableName)
		return client.CreateLockTable(ctx, l, tableName, tags)
	}

	return nil
}

// DeleteTableIfNecessary deletes the given table if it exists.
func (client *Client) DeleteTableIfNecessary(ctx context.Context, l log.Logger, tableName string) error {
	if exists, err := client.DoesLockTableExist(ctx, tableName); err != nil || !exists {
		return err
	}

	return client.DeleteTable(ctx, l, tableName)
}

// DoesLockTableExistAndIsActive returns true if the specified DynamoDB table exists and is active otherwise false.
func (client *Client) DoesLockTableExistAndIsActive(ctx context.Context, tableName string) (bool, error) {
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}

	res, err := client.dynamoClient.DescribeTable(ctx, input)
	if err != nil {
		if isAWSResourceNotFoundError(err) {
			// Table doesn't exist, so it's not active
			return false, nil
		}

		return false, err
	}

	return res.Table.TableStatus == dynamodbtypes.TableStatusActive, nil
}

// DoesLockTableExist returns true if the lock table exists.
func (client *Client) DoesLockTableExist(ctx context.Context, tableName string) (bool, error) {
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}

	_, err := client.dynamoClient.DescribeTable(ctx, input)
	if err != nil {
		if isAWSResourceNotFoundError(err) {
			return false, nil
		} else {
			return false, errors.New(err)
		}
	}

	return true, nil
}

// LockTableCheckSSEncryptionIsOn returns true if the lock table's SSEncryption is turned on
func (client *Client) LockTableCheckSSEncryptionIsOn(ctx context.Context, tableName string) (bool, error) {
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}

	output, err := client.dynamoClient.DescribeTable(ctx, input)
	if err != nil {
		if isAWSResourceNotFoundError(err) {
			// Table doesn't exist, so encryption is not enabled
			return false, nil
		}

		return false, errors.New(err)
	}

	return output.Table.SSEDescription != nil && string(output.Table.SSEDescription.Status) == string(dynamodbtypes.SSEStatusEnabled), nil
}

// CreateLockTable creates a lock table in DynamoDB and wait until it is in "active" state.
// If the table already exists, merely wait until it is in "active" state.
func (client *Client) CreateLockTable(ctx context.Context, l log.Logger, tableName string, tags map[string]string) error {
	tableCreateDeleteSemaphore.Acquire()
	defer tableCreateDeleteSemaphore.Release()

	l.Debugf("Creating table %s in DynamoDB", tableName)

	attributeDefinitions := []dynamodbtypes.AttributeDefinition{
		{AttributeName: aws.String(AttrLockID), AttributeType: dynamodbtypes.ScalarAttributeTypeS},
	}

	keySchema := []dynamodbtypes.KeySchemaElement{
		{AttributeName: aws.String(AttrLockID), KeyType: dynamodbtypes.KeyTypeHash},
	}

	input := &dynamodb.CreateTableInput{
		TableName:            aws.String(tableName),
		BillingMode:          dynamodbtypes.BillingMode(DynamodbPayPerRequestBillingMode),
		AttributeDefinitions: attributeDefinitions,
		KeySchema:            keySchema,
	}

	createTableOutput, err := client.dynamoClient.CreateTable(ctx, input)
	if err != nil {
		if isTableAlreadyBeingCreatedOrUpdatedError(err) {
			l.Debugf("Looks like someone created table %s at the same time. Will wait for it to be in active state.", tableName)
		} else {
			return errors.New(err)
		}
	}

	err = client.waitForTableToBeActive(ctx, l, tableName, MaxRetriesWaitingForTableToBeActive, SleepBetweenTableStatusChecks)
	if err != nil {
		return err
	}

	if createTableOutput != nil && createTableOutput.TableDescription != nil && createTableOutput.TableDescription.TableArn != nil {
		// Do not tag in case somebody else had created the table
		err = client.tagTableIfTagsGiven(ctx, l, tags, createTableOutput.TableDescription.TableArn)
		if err != nil {
			return errors.New(err)
		}
	}

	return nil
}

func (client *Client) tagTableIfTagsGiven(ctx context.Context, l log.Logger, tags map[string]string, tableArn *string) error {
	if len(tags) == 0 {
		l.Debugf("No tags for lock table given.")
		return nil
	}

	// we were able to create the table successfully, now add tags
	l.Debugf("Adding tags to lock table: %s", tags)

	var tagsConverted = make([]dynamodbtypes.Tag, 0, len(tags))

	for k, v := range tags {
		tagsConverted = append(tagsConverted, dynamodbtypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	var input = dynamodb.TagResourceInput{
		ResourceArn: tableArn,
		Tags:        tagsConverted}

	_, err := client.dynamoClient.TagResource(ctx, &input)

	return err
}

// DeleteTable deletes the given table in DynamoDB.
func (client *Client) DeleteTable(ctx context.Context, l log.Logger, tableName string) error {
	tableCreateDeleteSemaphore.Acquire()
	defer tableCreateDeleteSemaphore.Release()

	l.Debugf("Deleting DynamoD table %s", tableName)

	input := &dynamodb.DeleteTableInput{TableName: aws.String(tableName)}

	// It is not always able to delete a table the first attempt, as we can get a 400 from tags still being updated
	// while the table is being deleted.
	//
	// We retry to handle this race condition.
	const (
		maxRetries = 5
		delay      = 2 * time.Second
	)

	for i := range maxRetries {
		_, err := client.dynamoClient.DeleteTable(ctx, input)
		if err == nil {
			return nil
		}

		if isTableAlreadyBeingCreatedOrUpdatedError(err) {
			if i < maxRetries-1 {
				l.Debugf("Table %s is still being updated (likely tags). Will retry deletion after %s (attempt %d/%d)", tableName, delay, i+1, maxRetries)
				time.Sleep(delay)

				continue
			}
		}

		return err
	}

	return errors.
		Errorf("Failed to delete table %s after %d attempts", tableName, maxRetries)
}

// Return true if the given error is the error message returned by AWS when the resource already exists and is being
// updated by someone else
func isTableAlreadyBeingCreatedOrUpdatedError(err error) bool {
	var apiErr smithy.APIError

	ok := errors.As(err, &apiErr)

	return ok && apiErr.ErrorCode() == "ResourceInUseException"
}

// Wait for the given DynamoDB table to be in the "active" state. If it's not in "active" state, sleep for the
// specified amount of time, and try again, up to a maximum of maxRetries retries.
func (client *Client) waitForTableToBeActive(ctx context.Context, l log.Logger, tableName string, maxRetries int, sleepBetweenRetries time.Duration) error {
	return client.WaitForTableToBeActiveWithRandomSleep(ctx, l, tableName, maxRetries, sleepBetweenRetries, sleepBetweenRetries)
}

// WaitForTableToBeActiveWithRandomSleep waits for the given table as described above,
// but sleeps a random amount of time greater than sleepBetweenRetriesMin
// and less than sleepBetweenRetriesMax between tries. This is to avoid an AWS issue where all waiting requests fire at
// the same time, which continually triggered AWS's "subscriber limit exceeded" API error.
func (client *Client) WaitForTableToBeActiveWithRandomSleep(ctx context.Context, l log.Logger, tableName string, maxRetries int, sleepBetweenRetriesMin time.Duration, sleepBetweenRetriesMax time.Duration) error {
	for range maxRetries {
		tableReady, err := client.DoesLockTableExistAndIsActive(ctx, tableName)
		if err != nil {
			return err
		}

		if tableReady {
			l.Debugf("Success! Table %s is now in active state.", tableName)
			return nil
		}

		sleepBetweenRetries := util.GetRandomTime(sleepBetweenRetriesMin, sleepBetweenRetriesMax)
		l.Debugf("Table %s is not yet in active state. Will check again after %s.", tableName, sleepBetweenRetries)
		time.Sleep(sleepBetweenRetries)
	}

	return errors.New(TableActiveRetriesExceeded{TableName: tableName, Retries: maxRetries})
}

// UpdateLockTableSetSSEncryptionOnIfNecessary encrypts the TFState Lock table - If Necessary
func (client *Client) UpdateLockTableSetSSEncryptionOnIfNecessary(ctx context.Context, l log.Logger, tableName string) error {
	tableSSEncrypted, err := client.LockTableCheckSSEncryptionIsOn(ctx, tableName)
	if err != nil {
		return errors.New(err)
	}

	if tableSSEncrypted {
		l.Debugf("Table %s already has encryption enabled", tableName)
		return nil
	}

	tableCreateDeleteSemaphore.Acquire()
	defer tableCreateDeleteSemaphore.Release()

	l.Debugf("Enabling server-side encryption on table %s in AWS DynamoDB", tableName)

	input := &dynamodb.UpdateTableInput{
		SSESpecification: &dynamodbtypes.SSESpecification{
			Enabled: aws.Bool(true),
			SSEType: dynamodbtypes.SSETypeKms,
		},
		TableName: aws.String(tableName),
	}

	if _, err := client.dynamoClient.UpdateTable(ctx, input); err != nil {
		if isTableAlreadyBeingCreatedOrUpdatedError(err) {
			l.Debugf("Looks like someone is already updating table %s at the same time. Will wait for that update to complete.", tableName)
		} else {
			return errors.New(err)
		}
	}

	if err := client.waitForEncryptionToBeEnabled(ctx, l, tableName); err != nil {
		return errors.New(err)
	}

	return client.waitForTableToBeActive(ctx, l, tableName, MaxRetriesWaitingForTableToBeActive, SleepBetweenTableStatusChecks)
}

// Wait until encryption is enabled for the given table
func (client *Client) waitForEncryptionToBeEnabled(ctx context.Context, l log.Logger, tableName string) error {
	l.Debugf("Waiting for encryption to be enabled on table %s", tableName)

	for range maxRetriesWaitingForEncryption {
		tableSSEncrypted, err := client.LockTableCheckSSEncryptionIsOn(ctx, tableName)
		if err != nil {
			return errors.New(err)
		}

		if tableSSEncrypted {
			l.Debugf("Encryption is now enabled for table %s!", tableName)
			return nil
		}

		l.Debugf("Encryption is still not enabled for table %s. Will sleep for %v and try again.", tableName, sleepBetweenRetriesWaitingForEncryption)
		time.Sleep(sleepBetweenRetriesWaitingForEncryption)
	}

	return errors.New(TableEncryptedRetriesExceeded{TableName: tableName, Retries: maxRetriesWaitingForEncryption})
}

// DeleteTableItemIfNecessary deletes the given DynamoDB table key, if the table exists.
func (client *Client) DeleteTableItemIfNecessary(ctx context.Context, l log.Logger, tableName, key string) error {
	if exists, err := client.DoesTableItemExist(ctx, tableName, key); err != nil || !exists {
		return err
	}

	description := fmt.Sprintf("Delete DynamoDB table %s item %s", tableName, key)

	return util.DoWithRetry(ctx, description, s3MaxRetries, s3SleepBetweenRetries, l, log.DebugLevel, func(ctx context.Context) error {
		if err := client.DeleteTableItem(ctx, l, tableName, key); err != nil {
			if isBucketErrorRetriable(err) {
				return err
			}
			// return FatalError so that retry loop will not continue
			return util.FatalError{Underlying: err}
		}

		return nil
	})
}

// DeleteTableItem deletes the given DynamoDB table key.
func (client *Client) DeleteTableItem(ctx context.Context, l log.Logger, tableName, key string) error {
	l.Debugf("Deleting DynamoDB table %s item %s", tableName, key)

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: map[string]dynamodbtypes.AttributeValue{
			AttrLockID: &dynamodbtypes.AttributeValueMemberS{
				Value: key,
			},
		},
	}

	if _, err := client.dynamoClient.DeleteItem(ctx, input); err != nil {
		return errors.Errorf("failed to remove item by key %s of table %s: %w", key, tableName, err)
	}

	return nil
}

// DoesTableItemExist returns true if the given DynamoDB table and its key exist otherwise false.
func (client *Client) DoesTableItemExist(ctx context.Context, tableName, key string) (bool, error) {
	if exists, err := client.DoesLockTableExist(ctx, tableName); err != nil || !exists {
		return false, err
	}

	input := &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]dynamodbtypes.AttributeValue{
			AttrLockID: &dynamodbtypes.AttributeValueMemberS{
				Value: key,
			},
		},
	}

	res, err := client.dynamoClient.GetItem(ctx, input)
	if err != nil {
		return false, errors.Errorf("failed to get item by key %s of table %s: %w", key, tableName, err)
	}

	exists := len(res.Item) != 0

	return exists, nil
}

func (client *Client) DoesTableItemExistWithLogging(ctx context.Context, l log.Logger, tableName, key string) (bool, error) {
	if exists, err := client.DoesTableItemExist(ctx, tableName, key); err != nil || exists {
		return exists, err
	}

	l.Debugf("Remote state DynamoDB table %s item %s does not exist or you don't have permissions to access it.", tableName, key)

	return false, nil
}

// MoveS3Object copies the S3 object at the specified srcKey to dstKey and then removes srcKey.
func (client *Client) MoveS3Object(ctx context.Context, l log.Logger, srcBucketName, srcKey, dstBucketName, dstKey string) error {
	if err := client.CopyS3BucketObject(ctx, l, srcBucketName, srcKey, dstBucketName, dstKey); err != nil {
		return err
	}

	return client.DeleteS3BucketObject(ctx, l, srcBucketName, srcKey, nil)
}

// CreateTableItem creates a new table item `key` in DynamoDB.
func (client *Client) CreateTableItem(ctx context.Context, l log.Logger, tableName, key string) error {
	l.Debugf("Creating DynamoDB %s item %s", tableName, key)

	input := &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: map[string]dynamodbtypes.AttributeValue{
			AttrLockID: &dynamodbtypes.AttributeValueMemberS{Value: key},
		},
	}
	if _, err := client.dynamoClient.PutItem(ctx, input); err != nil {
		return errors.Errorf("failed to create table item: %w", err)
	}

	return nil
}

// EnableVersioningForS3Bucket enables versioning for the S3 bucket specified in the given config.
func (client *Client) EnableVersioningForS3Bucket(ctx context.Context, l log.Logger, bucketName string) error {
	l.Debugf("Enabling versioning for S3 bucket %s", bucketName)
	input := s3.PutBucketVersioningInput{
		Bucket: aws.String(bucketName),
		VersioningConfiguration: &types.VersioningConfiguration{
			Status: types.BucketVersioningStatusEnabled,
		},
	}

	_, err := client.s3Client.PutBucketVersioning(ctx, &input)
	if err != nil {
		return errors.New(err)
	}

	l.Debugf("Enabled versioning for S3 bucket %s", bucketName)

	return nil
}

// EnableSSEForS3BucketWide enables server-side encryption for the S3 bucket specified in the given config.
func (client *Client) EnableSSEForS3BucketWide(ctx context.Context, l log.Logger, bucketName string, algorithm string) error {
	l.Debugf("Enabling server-side encryption for S3 bucket %s", bucketName)

	accountID, err := awshelper.GetAWSAccountID(ctx, client.awsConfig)
	if err != nil {
		return errors.Errorf("error getting AWS account ID %s for bucket %s: %w", accountID, bucketName, err)
	}

	partition, err := awshelper.GetAWSPartition(ctx, client.awsConfig)
	if err != nil {
		return errors.Errorf("error getting AWS partition %s for bucket %s: %w", partition, bucketName, err)
	}

	input := &s3.PutBucketEncryptionInput{
		Bucket: aws.String(bucketName),
		ServerSideEncryptionConfiguration: &types.ServerSideEncryptionConfiguration{
			Rules: []types.ServerSideEncryptionRule{
				{
					ApplyServerSideEncryptionByDefault: &types.ServerSideEncryptionByDefault{
						SSEAlgorithm: types.ServerSideEncryption(algorithm),
					},
					BucketKeyEnabled: aws.Bool(true),
				},
			},
		},
	}

	// If using KMS encryption and a specific KMS key ID is configured, set it
	if algorithm == string(types.ServerSideEncryptionAwsKms) && client.BucketSSEKMSKeyID != "" {
		input.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault.KMSMasterKeyID = aws.String(client.BucketSSEKMSKeyID)
	}

	_, err = client.s3Client.PutBucketEncryption(ctx, input)
	if err != nil {
		return errors.New(err)
	}

	l.Debugf("Enabled server-side encryption for S3 bucket %s", bucketName)

	return nil
}

// checkIfSSEForS3MatchesConfig checks if the SSE configuration matches the expected configuration.
func (client *Client) checkIfSSEForS3MatchesConfig(ctx context.Context, bucketName string) (bool, error) {
	output, err := client.s3Client.GetBucketEncryption(ctx, &s3.GetBucketEncryptionInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "ServerSideEncryptionConfigurationNotFoundError" {
			return false, nil
		}

		return false, errors.New(err)
	}

	expectedAlgorithm := client.FetchEncryptionAlgorithm()

	for _, rule := range output.ServerSideEncryptionConfiguration.Rules {
		if rule.ApplyServerSideEncryptionByDefault == nil {
			continue
		}

		if string(rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm) != expectedAlgorithm {
			continue
		}

		if expectedAlgorithm != string(types.ServerSideEncryptionAwsKms) {
			return true, nil
		}

		if client.BucketSSEKMSKeyID == "" {
			return true, nil
		}

		if rule.ApplyServerSideEncryptionByDefault.KMSMasterKeyID == nil {
			return false, nil
		}

		if aws.ToString(rule.ApplyServerSideEncryptionByDefault.KMSMasterKeyID) != client.BucketSSEKMSKeyID {
			return false, nil
		}

		return true, nil
	}

	return false, nil
}

// checkIfBucketPolicyStatementExists checks if a specific policy statement exists in the bucket policy
func (client *Client) checkIfBucketPolicyStatementExists(ctx context.Context, bucketName, statementSid string) (bool, error) {
	policyOutput, err := client.s3Client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchBucketPolicy" {
			return false, nil
		}

		return false, errors.New(err)
	}

	if policyOutput.Policy == nil {
		return false, nil
	}

	policyInBucket, err := awshelper.UnmarshalPolicy(*policyOutput.Policy)
	if err != nil {
		return false, errors.New(err)
	}

	// Safety check to avoid nil pointer dereference
	if policyInBucket.Statement == nil {
		return false, nil
	}

	for _, statement := range policyInBucket.Statement {
		if statement.Sid == statementSid {
			return true, nil
		}
	}

	return false, nil
}

// checkIfBucketRootAccess checks if the root access policy is enabled for the bucket.
func (client *Client) checkIfBucketRootAccess(ctx context.Context, l log.Logger, bucketName string) (bool, error) {
	l.Debugf("Checking if bucket %s has root access", bucketName)
	return client.checkIfBucketPolicyStatementExists(ctx, bucketName, SidRootPolicy)
}

// checkIfBucketEnforcedTLS checks if the enforced TLS policy is enabled for the bucket.
func (client *Client) checkIfBucketEnforcedTLS(ctx context.Context, l log.Logger, bucketName string) (bool, error) {
	l.Debugf("Checking if bucket %s has enforced TLS", bucketName)
	return client.checkIfBucketPolicyStatementExists(ctx, bucketName, SidEnforcedTLSPolicy)
}

// DoesS3BucketExist checks if the S3 bucket exists and is accessible.
func (client *Client) DoesS3BucketExist(ctx context.Context, bucketName string) (bool, error) {
	input := &s3.HeadBucketInput{Bucket: aws.String(bucketName)}

	_, err := client.s3Client.HeadBucket(ctx, input)
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NotFound" {
			return false, nil
		}

		return false, errors.New(err)
	}

	return true, nil
}

// DoesS3BucketExistWithLogging checks if the S3 bucket exists and logs if not.
func (client *Client) DoesS3BucketExistWithLogging(ctx context.Context, l log.Logger, bucketName string) (bool, error) {
	if client.s3Client == nil {
		return false, errors.Errorf("S3 client is nil - cannot check if S3 bucket %s exists", bucketName)
	}

	l.Debugf("Checking if bucket %s exists", bucketName)

	exists, err := client.DoesS3BucketExist(ctx, bucketName)
	if err != nil || !exists {
		l.Debugf("Remote state S3 bucket %s does not exist or you don't have permissions to access it.", bucketName)
	}

	return exists, err
}

// checkS3AccessLoggingConfiguration checks if access logging is enabled for the S3 bucket.
func (client *Client) checkS3AccessLoggingConfiguration(ctx context.Context, bucketName string) (bool, error) {
	input := &s3.GetBucketLoggingInput{Bucket: aws.String(bucketName)}

	output, err := client.s3Client.GetBucketLogging(ctx, input)
	if err != nil {
		return false, errors.New(err)
	}

	return output.LoggingEnabled != nil, nil
}

// checkIfS3PublicAccessBlockingEnabled checks if public access blocking is enabled for the S3 bucket.
func (client *Client) checkIfS3PublicAccessBlockingEnabled(ctx context.Context, bucketName string) (bool, error) {
	output, err := client.s3Client.GetPublicAccessBlock(ctx, &s3.GetPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchPublicAccessBlockConfiguration" {
			return false, nil
		}

		return false, errors.New(err)
	}

	return output.PublicAccessBlockConfiguration != nil &&
		aws.ToBool(output.PublicAccessBlockConfiguration.BlockPublicAcls) &&
		aws.ToBool(output.PublicAccessBlockConfiguration.IgnorePublicAcls) &&
		aws.ToBool(output.PublicAccessBlockConfiguration.BlockPublicPolicy) &&
		aws.ToBool(output.PublicAccessBlockConfiguration.RestrictPublicBuckets), nil
}

// CopyS3BucketObject copies the S3 object at the specified srcBucketName and srcKey to dstBucketName and dstKey.
func (client *Client) CopyS3BucketObject(ctx context.Context, l log.Logger, srcBucketName, srcKey, dstBucketName, dstKey string) error {
	l.Debugf("Copying S3 bucket object from %s to %s", path.Join(srcBucketName, srcKey), path.Join(dstBucketName, dstKey))

	input := &s3.CopyObjectInput{
		Bucket:     aws.String(dstBucketName),
		Key:        aws.String(dstKey),
		CopySource: aws.String(path.Join(srcBucketName, srcKey)),
	}
	if _, err := client.s3Client.CopyObject(ctx, input); err != nil {
		return errors.Errorf("failed to copy object: %w", err)
	}

	return nil
}

// MoveS3ObjectIfNecessary moves the S3 object at the specified srcBucketName and srcKey to dstBucketName and dstKey, only if it exists and does not already exist at the destination.
func (client *Client) MoveS3ObjectIfNecessary(ctx context.Context, l log.Logger, srcBucketName, srcKey, dstBucketName, dstKey string) error {
	exists, err := client.DoesS3ObjectExistWithLogging(ctx, l, srcBucketName, srcKey)
	if err != nil || !exists {
		return err
	}

	exists, err = client.DoesS3ObjectExist(ctx, dstBucketName, dstKey)
	if err != nil {
		return err
	} else if exists {
		return errors.Errorf("destination S3 bucket %s object %s already exists", dstBucketName, dstKey)
	}

	description := fmt.Sprintf("Move S3 bucket object from %s to %s", path.Join(srcBucketName, srcKey), path.Join(dstBucketName, dstKey))

	return util.DoWithRetry(ctx, description, s3MaxRetries, s3SleepBetweenRetries, l, log.DebugLevel, func(ctx context.Context) error {
		if err := client.MoveS3Object(ctx, l, srcBucketName, srcKey, dstBucketName, dstKey); err != nil {
			if isBucketErrorRetriable(err) {
				return err
			}
			// return FatalError so that retry loop will not continue
			return util.FatalError{Underlying: err}
		}

		return nil
	})
}

// CreateTableItemIfNecessary creates the DynamoDB table item with the specified key, only if it does not already exist.
func (client *Client) CreateTableItemIfNecessary(ctx context.Context, l log.Logger, tableName, key string) error {
	exists, err := client.DoesTableItemExist(ctx, tableName, key)
	if err != nil {
		return err
	} else if exists {
		return errors.Errorf("DynamoDB table %s item %s already exists", tableName, key)
	}

	description := fmt.Sprintf("Create DynamoDB table %s item %s", tableName, key)

	return util.DoWithRetry(ctx, description, s3MaxRetries, s3SleepBetweenRetries, l, log.DebugLevel, func(ctx context.Context) error {
		if err := client.CreateTableItem(ctx, l, tableName, key); err != nil {
			if isBucketErrorRetriable(err) {
				return err
			}
			// return FatalError so that retry loop will not continue
			return util.FatalError{Underlying: err}
		}

		return nil
	})
}

// GetDynamoDBClient returns the DynamoDB client for testing purposes.
func (client *Client) GetDynamoDBClient() *dynamodb.Client {
	return client.dynamoClient
}

// isAWSResourceNotFoundError checks if an error indicates that an AWS resource was not found
func isAWSResourceNotFoundError(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "ResourceNotFoundException"
}

// handleS3TaggingMethodNotAllowed handles MethodNotAllowed errors for S3 bucket tagging operations
// Returns true if the error was handled (caller should return nil), false otherwise
func handleS3TaggingMethodNotAllowed(err error, l log.Logger, bucketName string) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "MethodNotAllowed" {
		l.Warnf("S3 bucket tagging is not supported for bucket %s - skipping tagging (this is normal for some AWS configurations)", bucketName)
		return true
	}

	return false
}
