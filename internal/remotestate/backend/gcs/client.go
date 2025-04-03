package gcs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"golang.org/x/oauth2"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const (
	maxRetriesWaitingForGcsBucket          = 12
	sleepBetweenRetriesWaitingForGcsBucket = 5 * time.Second

	gcpMaxRetries          = 3
	gcpSleepBetweenRetries = 10 * time.Second

	tokenURL = "https://oauth2.googleapis.com/token"
)

type Client struct {
	*ExtendedRemoteStateConfigGCS
	*storage.Client
	logger log.Logger
}

// NewClient inits GCS client.
func NewClient(ctx context.Context, config *ExtendedRemoteStateConfigGCS, logger log.Logger) (*Client, error) {
	var opts []option.ClientOption

	var credOpts []option.ClientOption

	gcsConfig := config.RemoteStateConfigGCS

	if gcsConfig.Credentials != "" {
		credOpts = append(credOpts, option.WithCredentialsFile(gcsConfig.Credentials))
	} else if gcsConfig.AccessToken != "" {
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: gcsConfig.AccessToken,
		})
		credOpts = append(credOpts, option.WithTokenSource(tokenSource))
	} else if oauthAccessToken := os.Getenv("GOOGLE_OAUTH_ACCESS_TOKEN"); oauthAccessToken != "" {
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: oauthAccessToken,
		})
		credOpts = append(credOpts, option.WithTokenSource(tokenSource))
	} else if os.Getenv("GOOGLE_CREDENTIALS") != "" {
		// to mirror how Terraform works, we have to accept either the file path or the contents
		creds := os.Getenv("GOOGLE_CREDENTIALS")

		contents, err := util.FileOrData(creds)
		if err != nil {
			return nil, errors.Errorf("Error loading credentials: %w", err)
		}

		if !json.Valid([]byte(contents)) {
			return nil, errors.Errorf("The string provided in credentials is not valid json")
		}

		credOpts = append(credOpts, option.WithCredentialsJSON([]byte(contents)))
	}

	if gcsConfig.ImpersonateServiceAccount != "" {
		ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: gcsConfig.ImpersonateServiceAccount,
			Scopes:          []string{storage.ScopeFullControl},
			Delegates:       gcsConfig.ImpersonateServiceAccountDelegates,
		}, credOpts...)

		if err != nil {
			return nil, err
		}

		opts = append(opts, option.WithTokenSource(ts))
	} else {
		opts = append(opts, credOpts...)
	}

	gcsClient, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}

	client := &Client{
		ExtendedRemoteStateConfigGCS: config,
		Client:                       gcsClient,
		logger:                       logger,
	}

	return client, nil
}

// CreateGCSBucketIfNecessary prompts the user to create the given bucket if it doesn't already exist and if the user
// confirms, creates the bucket and enables versioning for it.
func (client *Client) CreateGCSBucketIfNecessary(ctx context.Context, bucketName string, opts *options.TerragruntOptions) error {
	if client.DoesGCSBucketExist(ctx, bucketName) {
		return nil
	}

	// A project must be specified in order for terragrunt to automatically create a storage bucket.
	if client.Project == "" {
		return errors.New(MissingRequiredGCSRemoteStateConfig("project"))
	}

	// A location must be specified in order for terragrunt to automatically create a storage bucket.
	if client.Location == "" {
		return errors.New(MissingRequiredGCSRemoteStateConfig("location"))
	}

	client.logger.Debugf("Remote state GCS bucket %s does not exist. Attempting to create it", bucketName)

	if opts.FailIfBucketCreationRequired {
		return backend.BucketCreationNotAllowed(bucketName)
	}

	prompt := fmt.Sprintf("Remote state GCS bucket %s does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", bucketName)

	shouldCreateBucket, err := shell.PromptUserForYesNo(ctx, prompt, opts)
	if err != nil {
		return err
	}

	if shouldCreateBucket {
		// To avoid any eventual consistency issues with creating a GCS bucket we use a retry loop.
		description := "Create GCS bucket " + bucketName

		return util.DoWithRetry(ctx, description, gcpMaxRetries, gcpSleepBetweenRetries, client.logger, log.DebugLevel, func(ctx context.Context) error {
			return client.CreateGCSBucketWithVersioning(ctx, bucketName)
		})
	}

	return nil
}

// CheckIfGCSVersioningEnabled checks if versioning is enabled for the GCS bucket specified in the given config and warn the user if it is not
func (client *Client) CheckIfGCSVersioningEnabled(ctx context.Context, bucketName string) (bool, error) {
	bucket := client.Bucket(bucketName)

	attrs, err := bucket.Attrs(ctx)
	if err != nil {
		// ErrBucketNotExist
		return false, errors.New(err)
	}

	if !attrs.VersioningEnabled {
		client.logger.Warnf("Versioning is not enabled for the remote state GCS bucket %s. We recommend enabling versioning so that you can roll back to previous versions of your OpenTofu/Terraform state in case of error.", bucketName)
	}

	return attrs.VersioningEnabled, nil
}

// CreateGCSBucketWithVersioning creates the given GCS bucket and enables versioning for it.
func (client *Client) CreateGCSBucketWithVersioning(ctx context.Context, bucketName string) error {
	if err := client.CreateGCSBucket(ctx, bucketName); err != nil {
		return err
	}

	if err := client.WaitUntilGCSBucketExists(ctx, bucketName); err != nil {
		return err
	}

	if err := client.AddLabelsToGCSBucket(ctx, bucketName, client.GCSBucketLabels); err != nil {
		return err
	}

	return nil
}

// AddLabelsToGCSBucket adds the given labels to the GCS bucket.
func (client *Client) AddLabelsToGCSBucket(ctx context.Context, bucketName string, labels map[string]string) error {
	if len(labels) == 0 {
		client.logger.Debugf("No labels specified for bucket %s.", bucketName)
		return nil
	}

	client.logger.Debugf("Adding labels to GCS bucket with %s", labels)

	bucket := client.Bucket(bucketName)

	bucketAttrs := storage.BucketAttrsToUpdate{}

	for key, value := range labels {
		bucketAttrs.SetLabel(key, value)
	}

	_, err := bucket.Update(ctx, bucketAttrs)

	if err != nil {
		return errors.New(err)
	}

	return nil
}

// CreateGCSBucket creates the GCS bucket specified in the given config.
func (client *Client) CreateGCSBucket(ctx context.Context, bucketName string) error {
	client.logger.Debugf("Creating GCS bucket %s in project %s", bucketName, client.Project)

	// The project ID to which the bucket belongs. This is only used when creating a new bucket during initialization.
	// Since buckets have globally unique names, the project ID is not required to access the bucket during normal
	// operation.
	projectID := client.Project

	bucket := client.Bucket(bucketName)

	bucketAttrs := &storage.BucketAttrs{}

	if client.Location != "" {
		client.logger.Debugf("Creating GCS bucket in location %s.", client.Location)
		bucketAttrs.Location = client.Location
	}

	if client.SkipBucketVersioning {
		client.logger.Debugf("Versioning is disabled for the remote state GCS bucket %s using 'skip_bucket_versioning' config.", bucketName)
	} else {
		client.logger.Debugf("Enabling versioning on GCS bucket %s", bucketName)

		bucketAttrs.VersioningEnabled = true
	}

	if client.EnableBucketPolicyOnly {
		client.logger.Debugf("Enabling uniform bucket-level access on GCS bucket %s", bucketName)

		bucketAttrs.BucketPolicyOnly = storage.BucketPolicyOnly{Enabled: true}
	}

	if err := bucket.Create(ctx, projectID, bucketAttrs); err != nil {
		return errors.Errorf("error creating GCS bucket %s: %w", bucketName, err)
	}

	return nil
}

// WaitUntilGCSBucketExists waits for the GCS bucket specified in the given config to be created.
//
// GCP is eventually consistent, so after creating a GCS bucket, this method can be used to wait until the information
// about that GCS bucket has propagated everywhere.
func (client *Client) WaitUntilGCSBucketExists(ctx context.Context, bucketName string) error {
	client.logger.Debugf("Waiting for bucket %s to be created", bucketName)

	for retries := range maxRetriesWaitingForGcsBucket {
		if client.DoesGCSBucketExist(ctx, bucketName) {
			client.logger.Debugf("GCS bucket %s created.", bucketName)
			return nil
		}

		if retries < maxRetriesWaitingForGcsBucket-1 {
			client.logger.Debugf("GCS bucket %s has not been created yet. Sleeping for %s and will check again.", bucketName, sleepBetweenRetriesWaitingForGcsBucket)
			time.Sleep(sleepBetweenRetriesWaitingForGcsBucket)
		}
	}

	return errors.New(MaxRetriesWaitingForGCSBucketExceeded(bucketName))
}

// DoesGCSBucketExist returns true if the GCS bucket specified in the given config exists and the current user has the
// ability to access it.
func (client *Client) DoesGCSBucketExist(ctx context.Context, bucketName string) bool {
	bucketHandle := client.Bucket(bucketName)

	// TODO - the code below attempts to determine whether the storage bucket exists by making a making a number of API
	// calls, then attempting to list the contents of the bucket. It was adapted from Google's own integration tests and
	// should be improved once the appropriate API call is added. For more info see:
	// https://github.com/GoogleCloudPlatform/google-cloud-go/blob/de879f7be552d57556875b8aaa383bce9396cc8c/storage/integration_test.go#L1231
	if _, err := bucketHandle.Attrs(ctx); err != nil {
		// ErrBucketNotExist
		return false
	}

	it := bucketHandle.Objects(ctx, nil)
	if _, err := it.Next(); errors.Is(err, storage.ErrBucketNotExist) {
		return false
	}

	return true
}

// DeleteGCSBucketIfNecessary deletes the given GCS bucket with all its objects if it exists.
func (client *Client) DeleteGCSBucketIfNecessary(ctx context.Context, bucketName string) error {
	if !client.DoesGCSBucketExist(ctx, bucketName) {
		return nil
	}

	description := fmt.Sprintf("Delete GCS bucket %s with retry", bucketName)

	return util.DoWithRetry(ctx, description, gcpMaxRetries, gcpSleepBetweenRetries, client.logger, log.DebugLevel, func(ctx context.Context) error {
		if err := client.DeleteGCSObjects(ctx, bucketName, ""); err != nil {
			return err
		}

		return client.DeleteGCSBucket(ctx, bucketName)
	})
}

func (client *Client) DeleteGCSBucket(ctx context.Context, bucketName string) error {
	bucket := client.Bucket(bucketName)

	client.logger.Debugf("Deleting GCS bucket %s", bucketName)

	if err := bucket.Delete(ctx); err != nil {
		return errors.Errorf("error deleting GCS bucket %s: %w", bucketName, err)
	}

	client.logger.Debugf("Deleted GCS bucket %s", bucketName)

	return client.WaitUntilGCSBucketDeleted(ctx, bucketName)
}

// WaitUntilGCSBucketDeleted waits for the GCS bucket specified in the given config to be deleted.
func (client *Client) WaitUntilGCSBucketDeleted(ctx context.Context, bucketName string) error {
	client.logger.Debugf("Waiting for bucket %s to be deleted", bucketName)

	for retries := range maxRetriesWaitingForGcsBucket {
		if !client.DoesGCSBucketExist(ctx, bucketName) {
			client.logger.Debugf("GCS bucket %s deleted.", bucketName)
			return nil
		} else if retries < maxRetriesWaitingForGcsBucket-1 {
			client.logger.Debugf("GCS bucket %s has not been deleted yet. Sleeping for %s and will check again.", bucketName, sleepBetweenRetriesWaitingForGcsBucket)
			time.Sleep(sleepBetweenRetriesWaitingForGcsBucket)
		}
	}

	return errors.New(MaxRetriesWaitingForGCSBucketExceeded(bucketName))
}

// DeleteGCSObjectIfNecessary deletes the bucket objects with the given prefix if they exist.
func (client *Client) DeleteGCSObjectIfNecessary(ctx context.Context, bucketName, prefix string) error {
	if !client.DoesGCSBucketExist(ctx, bucketName) {
		return nil
	}

	description := fmt.Sprintf("Delete GCS objects with prefix %s in bucket %s with retry", prefix, bucketName)

	return util.DoWithRetry(ctx, description, gcpMaxRetries, gcpSleepBetweenRetries, client.logger, log.DebugLevel, func(ctx context.Context) error {
		return client.DeleteGCSObjects(ctx, bucketName, prefix)
	})
}

// DeleteGCSObjects deletes the bucket objects with the given prefix.
func (client *Client) DeleteGCSObjects(ctx context.Context, bucketName, prefix string) error {
	bucket := client.Bucket(bucketName)

	it := bucket.Objects(ctx, &storage.Query{
		Prefix:   prefix,
		Versions: true,
	})

	for {
		attrs, err := it.Next()
		if err != nil {
			if errors.Is(err, iterator.Done) {
				break
			}

			return errors.Errorf("failed to get GCS object attrs: %w", err)
		}

		client.logger.Debugf("Deleting GCS object %s with generation %d in bucket %s", attrs.Name, attrs.Generation, bucketName)

		if err := bucket.Object(attrs.Name).Generation(attrs.Generation).Delete(ctx); err != nil {
			return errors.Errorf("failed to delete object %s with generation %d in bucket %s: %w", attrs.Name, attrs.Generation, bucketName, err)
		}
	}

	return nil
}
