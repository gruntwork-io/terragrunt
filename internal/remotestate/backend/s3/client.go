package s3

import (
	"context"
	"fmt"
	"path"
	"reflect"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gruntwork-io/terragrunt/awshelper"
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

	*s3.S3
	*dynamodb.DynamoDB
	session *session.Session

	failIfBucketCreationRequired bool
}

func NewClient(l log.Logger, config *ExtendedRemoteStateConfigS3, opts *options.TerragruntOptions) (*Client, error) {
	awsConfig := config.GetAwsSessionConfig()

	session, err := awshelper.CreateAwsSession(l, awsConfig, opts)
	if err != nil {
		return nil, errors.New(err)
	}

	if !config.SkipCredentialsValidation {
		if err = awshelper.ValidateAwsSession(session); err != nil {
			return nil, err
		}
	}

	client := &Client{
		ExtendedRemoteStateConfigS3:  config,
		S3:                           s3.New(session),
		DynamoDB:                     dynamodb.New(session),
		session:                      session,
		failIfBucketCreationRequired: opts.FailIfBucketCreationRequired,
	}

	return client, nil
}

// CreateS3BucketIfNecessary prompts the user to create the given bucket if it doesn't already exist and if the user
// confirms, creates the bucket and enables versioning for it.
func (client *Client) CreateS3BucketIfNecessary(ctx context.Context, l log.Logger, bucketName string, opts *options.TerragruntOptions) error {
	cfg := &client.RemoteStateConfigS3

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

	prompt := fmt.Sprintf("Remote state S3 bucket %s is res of date. Would you like Terragrunt to update it?", bucketName)

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
		} else if err := client.EnableVersioningForS3Bucket(l, bucketName); err != nil {
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

		if err := client.EnableSSEForS3BucketWide(l, bucketName, client.FetchEncryptionAlgorithm()); err != nil {
			l.Errorf("Failed to enable Server-Side Encryption for the remote state AWS S3 bucket %s: %v", bucketName, err)
			return err
		}

		l.Infof("Successfully enabled Server-Side Encryption for the remote state AWS S3 bucket %s.", bucketName)
	}

	if bucketUpdatesRequired.RootAccess {
		if client.SkipBucketRootAccess {
			l.Debugf("Root access is disabled for the remote state S3 bucket %s using 'skip_bucket_root_access' config.", bucketName)
		} else if err := client.EnableRootAccesstoS3Bucket(l); err != nil {
			return err
		}
	}

	if bucketUpdatesRequired.EnforcedTLS {
		if client.SkipBucketEnforcedTLS {
			l.Debugf("Enforced TLS is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_enforced_tls' config.", bucketName)
		} else if err := client.EnableEnforcedTLSAccesstoS3Bucket(l, bucketName); err != nil {
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
		} else if err := client.EnablePublicAccessBlockingForS3Bucket(l, bucketName); err != nil {
			return err
		}
	}

	return nil
}

// configureAccessLogBucket - configure access log bucket.
func (client *Client) configureAccessLogBucket(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	cfg := &client.RemoteStateConfigS3

	l.Debugf("Enabling bucket-wide Access Logging on AWS S3 bucket %s - using as TargetBucket %s", client.RemoteStateConfigS3.Bucket, client.AccessLoggingBucketName)

	if err := client.CreateLogsS3BucketIfNecessary(ctx, l, client.AccessLoggingBucketName, opts); err != nil {
		l.Errorf("Could not create logs bucket %s for AWS S3 bucket %s\n%s", client.AccessLoggingBucketName, cfg.Bucket, err.Error())

		return err
	}

	if !client.SkipAccessLoggingBucketPublicAccessBlocking {
		if err := client.EnablePublicAccessBlockingForS3Bucket(l, client.AccessLoggingBucketName); err != nil {
			l.Errorf("Could not enable public access blocking on %s\n%s", client.AccessLoggingBucketName, err.Error())

			return err
		}
	}

	if err := client.EnableAccessLoggingForS3BucketWide(l); err != nil {
		l.Errorf("Could not enable access logging on %s\n%s", cfg.Bucket, err.Error())

		return err
	}

	if !client.SkipAccessLoggingBucketSSEncryption {
		if err := client.EnableSSEForS3BucketWide(l, client.AccessLoggingBucketName, s3.ServerSideEncryptionAes256); err != nil {
			l.Errorf("Could not enable encryption on %s\n%s", client.AccessLoggingBucketName, err.Error())

			return err
		}
	}

	if !client.SkipAccessLoggingBucketEnforcedTLS {
		if err := client.EnableEnforcedTLSAccesstoS3Bucket(l, client.AccessLoggingBucketName); err != nil {
			l.Errorf("Could not enable TLS access on %s\n%s", client.AccessLoggingBucketName, err.Error())

			return err
		}
	}

	if client.SkipBucketVersioning {
		l.Debugf("Versioning is disabled for the remote state S3 bucket %s using 'skip_bucket_versioning' config.", client.AccessLoggingBucketName)
	} else if err := client.EnableVersioningForS3Bucket(l, client.AccessLoggingBucketName); err != nil {
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
		matches, err := client.checkIfSSEForS3MatchesConfig(l, bucketName)
		if err != nil {
			return false, toUpdate, err
		}

		if !matches {
			toUpdate.SSEEncryption = true

			updates = append(updates, "Bucket Server-Side Encryption")
		}
	}

	if !client.SkipBucketRootAccess {
		enabled, err := client.checkIfBucketRootAccess(l, bucketName)
		if err != nil {
			return false, toUpdate, err
		}

		if !enabled {
			toUpdate.RootAccess = true

			updates = append(updates, "Bucket Root Access")
		}
	}

	if !client.SkipBucketEnforcedTLS {
		enabled, err := client.checkIfBucketEnforcedTLS(l, bucketName)
		if err != nil {
			return false, toUpdate, err
		}

		if !enabled {
			toUpdate.EnforcedTLS = true

			updates = append(updates, "Bucket Enforced TLS")
		}
	}

	if !client.SkipBucketAccessLogging && client.AccessLoggingBucketName != "" {
		enabled, err := client.checkS3AccessLoggingConfiguration(l, bucketName)
		if err != nil {
			return false, toUpdate, err
		}

		if !enabled {
			toUpdate.AccessLogging = true

			updates = append(updates, "Bucket Access Logging")
		}
	}

	if !client.SkipBucketPublicAccessBlocking {
		enabled, err := client.checkIfS3PublicAccessBlockingEnabled(l, bucketName)
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

	res, err := client.GetBucketVersioningWithContext(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucketName)})
	if err != nil {
		return false, errors.New(err)
	}

	// NOTE: There must be a bug in the AWS SDK since res == nil when versioning is not enabled. In the future,
	// check the AWS SDK for updates to see if we can remove "res == nil ||".
	if res == nil || res.Status == nil || *res.Status != s3.BucketVersioningStatusEnabled {
		l.Warnf("Versioning is not enabled for the remote state S3 bucket %s. We recommend enabling versioning so that you can roll back to previous versions of your OpenTofu/Terraform state in case of error.", bucketName)
		return false, nil
	}

	return true, nil
}

// CreateS3BucketWithVersioningSSEncryptionAndAccessLogging creates the given S3 bucket and enable versioning for it.
func (client *Client) CreateS3BucketWithVersioningSSEncryptionAndAccessLogging(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	cfg := &client.RemoteStateConfigS3

	l.Debugf("Create S3 bucket %s with versioning, SSE encryption, and access logging.", cfg.Bucket)

	err := client.CreateS3Bucket(l, cfg.Bucket)

	if err != nil {
		if accessError := client.checkBucketAccess(cfg.Bucket, cfg.Key); accessError != nil {
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
	} else if err := client.EnableRootAccesstoS3Bucket(l); err != nil {
		return err
	}

	if client.SkipBucketEnforcedTLS {
		l.Debugf("TLS enforcement is disabled for the remote state S3 bucket %s using 'skip_bucket_enforced_tls' config.", cfg.Bucket)
	} else if err := client.EnableEnforcedTLSAccesstoS3Bucket(l, cfg.Bucket); err != nil {
		return err
	}

	if client.SkipBucketPublicAccessBlocking {
		l.Debugf("Public access blocking is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_public_access_blocking' config.", cfg.Bucket)
	} else if err := client.EnablePublicAccessBlockingForS3Bucket(l, cfg.Bucket); err != nil {
		return err
	}

	if err := client.TagS3Bucket(l); err != nil {
		return err
	}

	if client.SkipBucketVersioning {
		l.Debugf("Versioning is disabled for the remote state S3 bucket %s using 'skip_bucket_versioning' config.", cfg.Bucket)
	} else if err := client.EnableVersioningForS3Bucket(l, cfg.Bucket); err != nil {
		return err
	}

	if client.SkipBucketSSEncryption {
		l.Debugf("Server-Side Encryption is disabled for the remote state AWS S3 bucket %s using 'skip_bucket_ssencryption' config.", cfg.Bucket)
	} else if err := client.EnableSSEForS3BucketWide(l, cfg.Bucket, client.FetchEncryptionAlgorithm()); err != nil {
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

	if err := client.TagS3BucketAccessLogging(l); err != nil {
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
		return client.CreateS3Bucket(l, logsBucketName)
	}

	return nil
}

func (client *Client) TagS3BucketAccessLogging(l log.Logger) error {
	if len(client.AccessLoggingBucketTags) == 0 {
		l.Debugf("No tags specified for bucket %s.", client.AccessLoggingBucketName)
		return nil
	}

	// There must be one entry in the list
	var tagsConverted = convertTags(client.AccessLoggingBucketTags)

	l.Debugf("Tagging S3 bucket with %s", client.AccessLoggingBucketTags)

	putBucketTaggingInput := s3.PutBucketTaggingInput{
		Bucket: aws.String(client.AccessLoggingBucketName),
		Tagging: &s3.Tagging{
			TagSet: tagsConverted,
		},
	}

	_, err := client.PutBucketTagging(&putBucketTaggingInput)
	if err != nil {
		return errors.New(err)
	}

	l.Debugf("Tagged S3 bucket with %s", client.AccessLoggingBucketTags)

	return nil
}

func (client *Client) TagS3Bucket(l log.Logger) error {
	cfg := &client.RemoteStateConfigS3

	if len(client.S3BucketTags) == 0 {
		l.Debugf("No tags specified for bucket %s.", cfg.Bucket)
		return nil
	}

	// There must be one entry in the list
	var tagsConverted = convertTags(client.S3BucketTags)

	l.Debugf("Tagging S3 bucket with %s", client.S3BucketTags)

	putBucketTaggingInput := s3.PutBucketTaggingInput{
		Bucket: aws.String(cfg.Bucket),
		Tagging: &s3.Tagging{
			TagSet: tagsConverted,
		},
	}

	_, err := client.PutBucketTagging(&putBucketTaggingInput)
	if err != nil {
		return errors.New(err)
	}

	l.Debugf("Tagged S3 bucket with %s", client.S3BucketTags)

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
func (client *Client) WaitUntilS3BucketExists(ctx context.Context, l log.Logger) error {
	cfg := &client.RemoteStateConfigS3

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
func (client *Client) CreateS3Bucket(l log.Logger, bucket string) error {
	l.Debugf("Creating S3 bucket %s", bucket)
	// https://github.com/aws/aws-sdk-go/blob/v1.44.245/service/s3/api.go#L41760
	_, err := client.CreateBucket(&s3.CreateBucketInput{Bucket: aws.String(bucket), ObjectOwnership: aws.String("ObjectWriter")})
	if err != nil {
		return errors.New(err)
	}

	l.Debugf("Created S3 bucket %s", bucket)

	return nil
}

// Determine if this is an error that implies you've already made a request to create the S3 bucket and it succeeded
// or is in progress. This usually happens when running many tests in parallel or xxx-all commands.
func isBucketAlreadyOwnedByYouError(err error) bool {
	var awsErr awserr.Error
	ok := errors.As(err, &awsErr)

	return ok && (awsErr.Code() == "BucketAlreadyOwnedByYou" || awsErr.Code() == "OperationAborted")
}

// isBucketErrorRetriable returns true if the error is temporary and can be retried.
func isBucketErrorRetriable(err error) bool {
	var awsErr awserr.Error

	ok := errors.As(err, &awsErr)
	if !ok {
		return true
	}

	return awsErr.Code() == "InternalError" || awsErr.Code() == "OperationAborted" || awsErr.Code() == "InvalidParameter"
}

// EnableRootAccesstoS3Bucket adds a policy to allow root access to the bucket.
func (client *Client) EnableRootAccesstoS3Bucket(l log.Logger) error {
	bucket := client.RemoteStateConfigS3.Bucket
	l.Debugf("Enabling root access to S3 bucket %s", bucket)

	accountID, err := awshelper.GetAWSAccountID(client.session)
	if err != nil {
		return errors.Errorf("error getting AWS account ID %s for bucket %s: %w", accountID, bucket, err)
	}

	partition, err := awshelper.GetAWSPartition(client.session)
	if err != nil {
		return errors.Errorf("error getting AWS partition %s for bucket %s: %w", partition, bucket, err)
	}

	var policyInBucket awshelper.Policy

	policyOutput, err := client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(bucket),
	})

	// If there's no policy, we need to create one
	if err != nil {
		l.Debugf("Policy not exists for bucket %s", bucket)
	}

	if policyOutput.Policy != nil {
		l.Debugf("Policy already exists for bucket %s", bucket)

		policyInBucket, err = awshelper.UnmarshalPolicy(*policyOutput.Policy)
		if err != nil {
			return errors.Errorf("error unmarshalling policy for bucket %s: %w", bucket, err)
		}
	}

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

	_, err = client.PutBucketPolicy(&s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(string(policy)),
	})
	if err != nil {
		return errors.Errorf("error putting policy for bucket %s: %w", bucket, err)
	}

	l.Debugf("Enabled root access to bucket %s", bucket)

	return nil
}

// Helper function to check if the root access policy is enabled for the bucket
func (client *Client) checkIfBucketRootAccess(l log.Logger, bucketName string) (bool, error) {
	l.Debugf("Checking if bucket %s is have root access", bucketName)

	policyOutput, err := client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		// NoSuchBucketPolicy error is considered as no policy.
		var awsErr awserr.Error
		if ok := errors.As(err, &awsErr); ok && awsErr.Code() == "NoSuchBucketPolicy" {
			return false, nil
		}

		l.Debugf("Could not get policy for bucket %s", bucketName)

		return false, errors.Errorf("error checking if bucket %s is have root access: %w", bucketName, err)
	}

	// If the bucket has no policy, it is not enforced
	if policyOutput == nil {
		return true, nil
	}

	policyInBucket, err := awshelper.UnmarshalPolicy(*policyOutput.Policy)
	if err != nil {
		return false, errors.Errorf("error unmarshalling policy for bucket %s: %w", bucketName, err)
	}

	for _, statement := range policyInBucket.Statement {
		if statement.Sid == SidRootPolicy {
			l.Debugf("Policy for RootAccess already exists for bucket %s", bucketName)
			return true, nil
		}
	}

	l.Debugf("Root access to bucket %s is not enabled", bucketName)

	return false, nil
}

// DoesS3BucketExist checks if the S3 bucket specified in the given config exists.
//
// Returns true if the S3 bucket specified in the given config exists and the current user has the ability to access
// it.
func (client *Client) DoesS3BucketExist(ctx context.Context, bucketName string) (bool, error) {
	input := &s3.HeadBucketInput{Bucket: aws.String(bucketName)}

	if _, err := client.HeadBucketWithContext(ctx, input); err != nil {
		var awsErr awserr.Error
		if ok := errors.As(err, &awsErr); ok && awsErr.Code() == "NotFound" {
			return false, nil
		}

		return false, errors.Errorf("error checking access to S3 bucket %s: %w", bucketName, err)
	}

	return true, nil
}

func (client *Client) DoesS3BucketExistWithLogging(ctx context.Context, l log.Logger, bucketName string) (bool, error) {
	if exists, err := client.DoesS3BucketExist(ctx, bucketName); err != nil || exists {
		return exists, err
	}

	l.Debugf("Remote state S3 bucket %s does not exist or you don't have permissions to access it.", bucketName)

	return false, nil
}

func (client *Client) checkIfBucketEnforcedTLS(l log.Logger, bucketName string) (bool, error) {
	l.Debugf("Checking if bucket %s is enforced with TLS", bucketName)

	policyOutput, err := client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		// S3 API error codes:
		// http://docs.aws.amazon.com/AmazonS3/latest/API/ErrorResponses.html
		var awsErr awserr.Error
		if ok := errors.As(err, &awsErr); ok {
			// Enforced TLS policy if is not found bucket policy
			if awsErr.Code() == "NoSuchBucketPolicy" {
				l.Debugf("Could not get policy for bucket %s", bucketName)
				return false, nil
			}
		}

		return false, errors.Errorf("error checking if bucket %s is enforced with TLS: %w", bucketName, err)
	}

	if policyOutput.Policy == nil {
		return true, nil
	}

	policyInBucket, err := awshelper.UnmarshalPolicy(*policyOutput.Policy)
	if err != nil {
		return false, errors.Errorf("error unmarshalling policy for bucket %s: %w", bucketName, err)
	}

	for _, statement := range policyInBucket.Statement {
		if statement.Sid == SidEnforcedTLSPolicy {
			l.Debugf("Policy for EnforcedTLS already exists for bucket %s", bucketName)
			return true, nil
		}
	}

	l.Debugf("Bucket %s is not enforced with TLS Policy", bucketName)

	return false, nil
}

// EnableEnforcedTLSAccesstoS3Bucket adds a policy to enforce TLS based access to the bucket.
func (client *Client) EnableEnforcedTLSAccesstoS3Bucket(l log.Logger, bucket string) error {
	l.Debugf("Enabling enforced TLS access for S3 bucket %s", bucket)

	partition, err := awshelper.GetAWSPartition(client.session)
	if err != nil {
		return errors.New(err)
	}

	var policyInBucket awshelper.Policy

	policyOutput, err := client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(bucket),
	})
	// If there's no policy, we need to create one
	if err != nil {
		l.Debugf("Policy not exists for bucket %s", bucket)
	}

	if policyOutput.Policy != nil {
		l.Debugf("Policy already exists for bucket %s", bucket)

		policyInBucket, err = awshelper.UnmarshalPolicy(*policyOutput.Policy)
		if err != nil {
			return errors.Errorf("error unmarshalling policy for bucket %s: %w", bucket, err)
		}
	}

	for _, statement := range policyInBucket.Statement {
		if statement.Sid == SidEnforcedTLSPolicy {
			l.Debugf("Policy for EnforceTLS already exists for bucket %s", bucket)
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
				Condition: &map[string]any{
					"Bool": map[string]any{
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

	_, err = client.PutBucketPolicy(&s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(string(policy)),
	})
	if err != nil {
		return errors.Errorf("error putting policy for bucket %s: %w", bucket, err)
	}

	l.Debugf("Enabled enforced TLS access for bucket %s", bucket)

	return nil
}

// Helper function to check if the enforced TLS policy is enabled for the bucket

// EnableVersioningForS3Bucket enables versioning for the S3 bucket specified in the given config.
func (client *Client) EnableVersioningForS3Bucket(l log.Logger, bucketName string) error {
	l.Debugf("Enabling versioning on S3 bucket %s", bucketName)
	input := s3.PutBucketVersioningInput{
		Bucket:                  aws.String(bucketName),
		VersioningConfiguration: &s3.VersioningConfiguration{Status: aws.String(s3.BucketVersioningStatusEnabled)},
	}

	_, err := client.PutBucketVersioning(&input)
	if err != nil {
		return errors.Errorf("error enabling versioning on S3 bucket %s: %w", bucketName, err)
	}

	l.Debugf("Enabled versioning on S3 bucket %s", bucketName)

	return nil
}

// EnableSSEForS3BucketWide enables bucket-wide Server-Side Encryption for the AWS S3 bucket specified in the given config.
func (client *Client) EnableSSEForS3BucketWide(l log.Logger, bucketName string, algorithm string) error {
	cfg := &client.RemoteStateConfigS3

	l.Debugf("Enabling bucket-wide SSE on AWS S3 bucket %s", bucketName)

	accountID, err := awshelper.GetAWSAccountID(client.session)
	if err != nil {
		return errors.New(err)
	}

	partition, err := awshelper.GetAWSPartition(client.session)
	if err != nil {
		return errors.New(err)
	}

	defEnc := &s3.ServerSideEncryptionByDefault{
		SSEAlgorithm: aws.String(algorithm),
	}
	if algorithm == s3.ServerSideEncryptionAwsKms && client.BucketSSEKMSKeyID != "" {
		defEnc.KMSMasterKeyID = aws.String(client.BucketSSEKMSKeyID)
	} else if algorithm == s3.ServerSideEncryptionAwsKms {
		kmsKeyID := fmt.Sprintf("arn:%s:kms:%s:%s:alias/aws/s3", partition, cfg.Region, accountID)
		defEnc.KMSMasterKeyID = aws.String(kmsKeyID)
	}

	rule := &s3.ServerSideEncryptionRule{ApplyServerSideEncryptionByDefault: defEnc}
	rules := []*s3.ServerSideEncryptionRule{rule}
	serverConfig := &s3.ServerSideEncryptionConfiguration{Rules: rules}
	input := &s3.PutBucketEncryptionInput{Bucket: aws.String(bucketName), ServerSideEncryptionConfiguration: serverConfig}

	_, err = client.PutBucketEncryption(input)
	if err != nil {
		return errors.Errorf("error enabling bucket-wide SSE on AWS S3 bucket %s: %w", bucketName, err)
	}

	l.Debugf("Enabled bucket-wide SSE on AWS S3 bucket %s", bucketName)

	return nil
}

func (client *Client) checkIfSSEForS3MatchesConfig(l log.Logger, bucketName string) (bool, error) {
	l.Debugf("Checking if SSE is enabled for AWS S3 bucket %s", bucketName)

	input := &s3.GetBucketEncryptionInput{Bucket: aws.String(bucketName)}

	output, err := client.GetBucketEncryption(input)
	if err != nil {
		l.Debugf("Error checking if SSE is enabled for AWS S3 bucket %s: %s", bucketName, err.Error())

		return false, errors.Errorf("error checking if SSE is enabled for AWS S3 bucket %s: %w", bucketName, err)
	}

	if output.ServerSideEncryptionConfiguration == nil {
		return false, nil
	}

	for _, rule := range output.ServerSideEncryptionConfiguration.Rules {
		if rule.ApplyServerSideEncryptionByDefault != nil && rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm != nil {
			if *rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm == client.FetchEncryptionAlgorithm() {
				return true, nil
			}

			return false, nil
		}
	}

	return false, nil
}

// EnableAccessLoggingForS3BucketWide enables bucket-wide Access Logging for the AWS S3 bucket specified in the given config.
func (client *Client) EnableAccessLoggingForS3BucketWide(l log.Logger) error {
	cfg := &client.RemoteStateConfigS3

	bucket := cfg.Bucket
	logsBucket := client.AccessLoggingBucketName
	logsBucketPrefix := client.AccessLoggingTargetPrefix

	if !client.SkipAccessLoggingBucketACL {
		if err := client.configureBucketAccessLoggingACL(l, logsBucket); err != nil {
			return errors.Errorf("error configuring bucket access logging ACL on S3 bucket %s: %w", cfg.Bucket, err)
		}
	}

	loggingInput := client.CreateS3LoggingInput()
	l.Debugf("Putting bucket logging on S3 bucket %s with TargetBucket %s and TargetPrefix %s\n%s", bucket, logsBucket, logsBucketPrefix, loggingInput)

	if _, err := client.PutBucketLogging(&loggingInput); err != nil {
		return errors.Errorf("error enabling bucket-wide Access Logging on AWS S3 bucket %s: %w", cfg.Bucket, err)
	}

	l.Debugf("Enabled bucket-wide Access Logging on AWS S3 bucket %s", bucket)

	return nil
}

func (client *Client) checkS3AccessLoggingConfiguration(l log.Logger, bucketName string) (bool, error) {
	l.Debugf("Checking if Access Logging is enabled for AWS S3 bucket %s", bucketName)

	input := &s3.GetBucketLoggingInput{Bucket: aws.String(bucketName)}

	output, err := client.GetBucketLogging(input)
	if err != nil {
		l.Debugf("Error checking if Access Logging is enabled for AWS S3 bucket %s: %s", bucketName, err.Error())
		return false, errors.Errorf("error checking if Access Logging is enabled for AWS S3 bucket %s: %w", bucketName, err)
	}

	if output.LoggingEnabled == nil {
		return false, nil
	}

	loggingInput := client.CreateS3LoggingInput()

	if !reflect.DeepEqual(output.LoggingEnabled, loggingInput.BucketLoggingStatus.LoggingEnabled) {
		return false, nil
	}

	return true, nil
}

// EnablePublicAccessBlockingForS3Bucket blocks all public access policies on the bucket and objects.
// These settings ensure that a misconfiguration of the
// bucket or objects will not accidentally enable public access to those items. See
// https://docs.aws.amazon.com/AmazonS3/latest/dev/access-control-block-public-access.html for more information.
func (client *Client) EnablePublicAccessBlockingForS3Bucket(l log.Logger, bucketName string) error {
	l.Debugf("Blocking all public access to S3 bucket %s", bucketName)
	_, err := client.PutPublicAccessBlock(
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

	l.Debugf("Blocked all public access to S3 bucket %s", bucketName)

	return nil
}

func (client *Client) checkIfS3PublicAccessBlockingEnabled(l log.Logger, bucketName string) (bool, error) {
	l.Debugf("Checking if S3 bucket %s is configured to block public access", bucketName)

	output, err := client.GetPublicAccessBlock(&s3.GetPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		var awsErr awserr.Error
		if ok := errors.As(err, &awsErr); ok {
			// Enforced block public access if is not found bucket policy
			if awsErr.Code() == "NoSuchPublicAccessBlockConfiguration" {
				l.Debugf("Could not get public access block for bucket %s", bucketName)
				return false, nil
			}
		}

		return false, errors.Errorf("error checking if S3 bucket %s is configured to block public access: %w", bucketName, err)
	}

	return awshelper.ValidatePublicAccessBlock(output)
}

// configureBucketAccessLoggingACL grants WRITE and READ_ACP permissions to
// the Log Delivery Group for the S3 bucket.
//
// To enable access logging in an S3 bucket, you must grant WRITE and READ_ACP permissions to the Log Delivery
// Group. For more info, see:
// https://docs.aws.amazon.com/AmazonS3/latest/dev/enable-logging-programming.html
func (client *Client) configureBucketAccessLoggingACL(l log.Logger, bucketName string) error {
	l.Debugf("Granting WRITE and READ_ACP permissions to S3 Log Delivery (%s) for bucket %s. This is required for access logging.", s3LogDeliveryGranteeURI, bucketName)

	uri := "uri=" + s3LogDeliveryGranteeURI
	aclInput := s3.PutBucketAclInput{
		Bucket:       aws.String(bucketName),
		GrantWrite:   aws.String(uri),
		GrantReadACP: aws.String(uri),
	}

	if _, err := client.PutBucketAcl(&aclInput); err != nil {
		return errors.Errorf("error granting WRITE and READ_ACP permissions to S3 Log Delivery (%s) for bucket %s: %w", s3LogDeliveryGranteeURI, bucketName, err)
	}

	return client.waitUntilBucketHasAccessLoggingACL(l, bucketName)
}

func (client *Client) waitUntilBucketHasAccessLoggingACL(l log.Logger, bucketName string) error {
	l.Debugf("Waiting for ACL bucket %s to have the updated ACL for access logging.", bucketName)

	maxRetries := 10

	for range maxRetries {
		res, err := client.GetBucketAcl(&s3.GetBucketAclInput{Bucket: aws.String(bucketName)})
		if err != nil {
			return errors.Errorf("error getting ACL for bucket %s: %w", bucketName, err)
		}

		hasReadAcp := false
		hasWrite := false

		for _, grant := range res.Grants {
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
			l.Debugf("Bucket %s now has the proper ACL permissions for access logging!", bucketName)
			return nil
		}

		l.Debugf("Bucket %s still does not have the ACL permissions for access logging. Will sleep for %v and check again.", bucketName, s3TimeBetweenRetries)
		time.Sleep(s3TimeBetweenRetries)
	}

	return errors.New(MaxRetriesWaitingForS3ACLExceeded(bucketName))
}

// checkBucketAccess checks if the current user has the ability to access the S3 bucket keys.
func (client *Client) checkBucketAccess(bucket, key string) error {
	_, err := client.GetObject(&s3.GetObjectInput{Key: aws.String(key), Bucket: aws.String(bucket)})
	if err == nil {
		return nil
	}

	var awsErr awserr.Error

	if ok := errors.As(err, &awsErr); !ok {
		return errors.New(err)
	}

	// filter permissions errors
	if awsErr.Code() == "NoSuchBucket" || awsErr.Code() == "NoSuchKey" {
		return nil
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

func (client *Client) DeleteS3BucketObject(ctx context.Context, l log.Logger, bucketName, key string, versionID *string) error {
	if versionID != nil {
		l.Debugf("Deleting S3 bucket %s object %s version %s", bucketName, key, *versionID)
	} else {
		l.Debugf("Deleting S3 bucket %s object %s", bucketName, key)
	}

	objectInput := &s3.DeleteObjectInput{
		Bucket:    aws.String(bucketName),
		Key:       aws.String(key),
		VersionId: versionID,
	}

	if _, err := client.DeleteObjectWithContext(ctx, objectInput); err != nil {
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

		res, err := client.ListObjectsV2WithContext(ctx, v2Input)
		if err != nil {
			return errors.Errorf("failed to list objects: %w", err)
		}

		for _, item := range res.Contents {
			if err := client.DeleteS3BucketObject(ctx, l, bucketName, aws.StringValue(item.Key), nil); err != nil {
				return err
			}
		}

		if !aws.BoolValue(res.IsTruncated) {
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

		res, err := client.ListObjectVersionsWithContext(ctx, versionsInput)
		if err != nil {
			return errors.Errorf("failed to list version objects: %w", err)
		}

		for _, item := range res.DeleteMarkers {
			if len(keys) != 0 && !slices.Contains(keys, aws.StringValue(item.Key)) {
				continue
			}

			if err := client.DeleteS3BucketObject(ctx, l, bucketName, aws.StringValue(item.Key), item.VersionId); err != nil {
				return err
			}
		}

		for _, item := range res.Versions {
			if len(keys) != 0 && !slices.Contains(keys, aws.StringValue(item.Key)) {
				continue
			}

			if err := client.DeleteS3BucketObject(ctx, l, bucketName, aws.StringValue(item.Key), item.VersionId); err != nil {
				return err
			}
		}

		if !aws.BoolValue(res.IsTruncated) {
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
		cfg         = &client.RemoteStateConfigS3
		key         = cfg.Key
		bucketInput = &s3.DeleteBucketInput{Bucket: aws.String(bucketName)}
	)

	l.Debugf("Deleting S3 bucket %s", bucketName)

	if _, err := client.DeleteBucketWithContext(ctx, bucketInput); err != nil {
		if err := client.checkBucketAccess(bucketName, key); err != nil {
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

	if _, err := client.HeadObjectWithContext(ctx, input); err != nil {
		var awsErr awserr.Error
		if ok := errors.As(err, &awsErr); ok {
			if awsErr.Code() == "NotFound" { // s3.ErrCodeNoSuchKey does not work, aws is missing this error code so we hardwire a string
				return false, nil
			}
		}

		return false, err
	}

	return true, nil
}

func (client *Client) DoesS3ObjectExistWithLogging(ctx context.Context, l log.Logger, bucketName, key string) (bool, error) {
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

// DoesLockTableExistAndIsActive returns true if the lock table exists in DynamoDB and is in "active" state.
func (client *Client) DoesLockTableExistAndIsActive(ctx context.Context, tableName string) (bool, error) {
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}

	output, err := client.DescribeTableWithContext(ctx, input)
	if err != nil {
		var awsErr awserr.Error
		if ok := errors.As(err, &awsErr); ok && awsErr.Code() == "ResourceNotFoundException" {
			return false, nil
		} else {
			return false, errors.New(err)
		}
	}

	return *output.Table.TableStatus == dynamodb.TableStatusActive, nil
}

// DoesLockTableExist returns true if the lock table exists.
func (client *Client) DoesLockTableExist(ctx context.Context, tableName string) (bool, error) {
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}

	_, err := client.DescribeTableWithContext(ctx, input)
	if err != nil {
		var awsErr awserr.Error
		if ok := errors.As(err, &awsErr); ok && awsErr.Code() == "ResourceNotFoundException" {
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

	output, err := client.DescribeTableWithContext(ctx, input)
	if err != nil {
		return false, errors.New(err)
	}

	return output.Table.SSEDescription != nil && aws.StringValue(output.Table.SSEDescription.Status) == dynamodb.SSEStatusEnabled, nil
}

// CreateLockTable creates a lock table in DynamoDB and wait until it is in "active" state.
// If the table already exists, merely wait until it is in "active" state.
func (client *Client) CreateLockTable(ctx context.Context, l log.Logger, tableName string, tags map[string]string) error {
	tableCreateDeleteSemaphore.Acquire()
	defer tableCreateDeleteSemaphore.Release()

	l.Debugf("Creating table %s in DynamoDB", tableName)

	attributeDefinitions := []*dynamodb.AttributeDefinition{
		{AttributeName: aws.String(AttrLockID), AttributeType: aws.String(dynamodb.ScalarAttributeTypeS)},
	}

	keySchema := []*dynamodb.KeySchemaElement{
		{AttributeName: aws.String(AttrLockID), KeyType: aws.String(dynamodb.KeyTypeHash)},
	}

	input := &dynamodb.CreateTableInput{
		TableName:            aws.String(tableName),
		BillingMode:          aws.String(DynamodbPayPerRequestBillingMode),
		AttributeDefinitions: attributeDefinitions,
		KeySchema:            keySchema,
	}

	createTableOutput, err := client.CreateTableWithContext(ctx, input)

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

	var tagsConverted = make([]*dynamodb.Tag, 0, len(tags))

	for k, v := range tags {
		tagsConverted = append(tagsConverted, &dynamodb.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	var input = dynamodb.TagResourceInput{
		ResourceArn: tableArn,
		Tags:        tagsConverted}

	_, err := client.TagResourceWithContext(ctx, &input)

	return err
}

// DeleteTable deletes the given table in DynamoDB.
func (client *Client) DeleteTable(ctx context.Context, l log.Logger, tableName string) error {
	const (
		maxRetries    = 5
		minRetryDelay = time.Second
	)

	tableCreateDeleteSemaphore.Acquire()
	defer tableCreateDeleteSemaphore.Release()

	l.Debugf("Deleting DynamoD table %s", tableName)

	req, _ := client.DeleteTableRequest(&dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
	req.SetContext(ctx)

	// It is not always able to delete a table the first attempt, error: `StatusCode: 400, Attempt to change a resource which is still in use: Table tags are being updated: terragrunt_test_*`
	req.Retryer = &Retryer{DefaultRetryer: awsclient.DefaultRetryer{
		NumMaxRetries: maxRetries,
		MinRetryDelay: minRetryDelay,
	}}

	return req.Send()
}

// Return true if the given error is the error message returned by AWS when the resource already exists and is being
// updated by someone else
func isTableAlreadyBeingCreatedOrUpdatedError(err error) bool {
	var awsErr awserr.Error
	ok := errors.As(err, &awsErr)

	return ok && awsErr.Code() == "ResourceInUseException"
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
		SSESpecification: &dynamodb.SSESpecification{
			Enabled: aws.Bool(true),
			SSEType: aws.String("KMS"),
		},
		TableName: aws.String(tableName),
	}

	if _, err := client.UpdateTableWithContext(ctx, input); err != nil {
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
		Key: map[string]*dynamodb.AttributeValue{
			AttrLockID: {
				S: aws.String(key),
			},
		},
	}

	if _, err := client.DeleteItemWithContext(ctx, input); err != nil {
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
		Key: map[string]*dynamodb.AttributeValue{
			AttrLockID: {
				S: aws.String(key),
			},
		},
	}

	res, err := client.GetItemWithContext(ctx, input)
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

// CopyS3BucketObject copies the S3 object at the specified `srcBucketName` and `srcKey` to the `dstBucketName` and `dstKey`.
func (client *Client) CopyS3BucketObject(ctx context.Context, l log.Logger, srcBucketName, srcKey, dstBucketName, dstKey string) error {
	l.Debugf("Copying S3 bucket object from %s to %s", path.Join(srcBucketName, srcKey), path.Join(dstBucketName, dstKey))

	input := &s3.CopyObjectInput{
		Bucket:     aws.String(dstBucketName),
		Key:        aws.String(dstKey),
		CopySource: aws.String(path.Join(srcBucketName, srcKey)),
	}

	if _, err := client.CopyObjectWithContext(ctx, input); err != nil {
		return errors.Errorf("failed to copy object: %w", err)
	}

	return nil
}

// MoveS3Object copies the S3 object at the specified srcKey to dstKey and then removes srcKey.
func (client *Client) MoveS3Object(ctx context.Context, l log.Logger, srcBucketName, srcKey, dstBucketName, dstKey string) error {
	if err := client.CopyS3BucketObject(ctx, l, srcBucketName, srcKey, dstBucketName, dstKey); err != nil {
		return err
	}

	return client.DeleteS3BucketObject(ctx, l, srcBucketName, srcKey, nil)
}

// MoveS3ObjectIfNecessary moves the S3 object at the specified srcBucketName and srcKey to dstBucketName and dstKey.
func (client *Client) MoveS3ObjectIfNecessary(ctx context.Context, l log.Logger, srcBucketName, srcKey, dstBucketName, dstKey string) error { // nolint: dupl
	if exists, err := client.DoesS3ObjectExistWithLogging(ctx, l, srcBucketName, srcKey); err != nil || !exists {
		return err
	}

	if exists, err := client.DoesS3ObjectExist(ctx, dstBucketName, dstKey); err != nil {
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

// CreateTableItemIfNecessary creates the DynamoDB table item with the specified key.
func (client *Client) CreateTableItemIfNecessary(ctx context.Context, l log.Logger, tableName, key string) error { // nolint: dupl
	if exists, err := client.DoesTableItemExist(ctx, tableName, key); err != nil {
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

// CreateTableItem creates a new table item `key`.
func (client *Client) CreateTableItem(ctx context.Context, l log.Logger, tableName, key string) error {
	l.Debugf("Creating DynamoDB %s item %s", tableName, key)

	input := &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: map[string]*dynamodb.AttributeValue{
			AttrLockID: {
				S: aws.String(key),
			},
		},
	}

	if _, err := client.PutItemWithContext(ctx, input); err != nil {
		return errors.Errorf("failed to create table item: %w", err)
	}

	return nil
}
