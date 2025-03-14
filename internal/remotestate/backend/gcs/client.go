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
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
)

const (
	maxRetriesWaitingForGcsBucket          = 12
	sleepBetweenRetriesWaitingForGcsBucket = 5 * time.Second

	gcpMaxRetries          = 3
	gcpSleepBetweenRetries = 10 * time.Second
)

type Client struct {
	*ExtendedRemoteStateConfigGCS
	*storage.Client
	logger log.Logger
}

func NewClient(ctx context.Context, config *ExtendedRemoteStateConfigGCS, logger log.Logger) (*Client, error) {
	var opts []option.ClientOption

	gcsConfig := config.RemoteStateConfigGCS

	if gcsConfig.Credentials != "" {
		opts = append(opts, option.WithCredentialsFile(gcsConfig.Credentials))
	} else if gcsConfig.AccessToken != "" {
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: gcsConfig.AccessToken,
		})
		opts = append(opts, option.WithTokenSource(tokenSource))
	} else if oauthAccessToken := os.Getenv("GOOGLE_OAUTH_ACCESS_TOKEN"); oauthAccessToken != "" {
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: oauthAccessToken,
		})
		opts = append(opts, option.WithTokenSource(tokenSource))
	} else if os.Getenv("GOOGLE_CREDENTIALS") != "" {
		var account accountFile
		// to mirror how Terraform works, we have to accept either the file path or the contents
		creds := os.Getenv("GOOGLE_CREDENTIALS")

		contents, err := util.FileOrData(creds)
		if err != nil {
			return nil, fmt.Errorf("Error loading credentials: %w", err)
		}

		if err := json.Unmarshal([]byte(contents), &account); err != nil {
			return nil, fmt.Errorf("Error parsing credentials '%s': %w", contents, err)
		}

		if err := json.Unmarshal([]byte(contents), &account); err != nil {
			return nil, fmt.Errorf("Error parsing credentials '%s': %w", contents, err)
		}

		conf := jwt.Config{
			Email:      account.ClientEmail,
			PrivateKey: []byte(account.PrivateKey),
			// We need the FullControl scope to be able to add metadata such as labels
			Scopes:   []string{storage.ScopeFullControl},
			TokenURL: "https://oauth2.googleapis.com/token",
		}

		opts = append(opts, option.WithHTTPClient(conf.Client(ctx)))
	}

	if gcsConfig.ImpersonateServiceAccount != "" {
		ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: gcsConfig.ImpersonateServiceAccount,
			Scopes:          []string{storage.ScopeFullControl},
			Delegates:       gcsConfig.ImpersonateServiceAccountDelegates,
		})
		if err != nil {
			return nil, err
		}

		opts = append(opts, option.WithTokenSource(ts))
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

// If the bucket specified in the given config doesn't already exist, prompt the user to create it, and if the user
// confirms, create the bucket and enable versioning for it.
func (client *Client) createGCSBucketIfNecessary(ctx context.Context, bucketName string, opts *options.TerragruntOptions) error {
	if client.DoesGCSBucketExist(ctx, bucketName) {
		return nil
	}

	client.logger.Debugf("Remote state GCS bucket %s does not exist. Attempting to create it", bucketName)

	// A project must be specified in order for terragrunt to automatically create a storage bucket.
	if client.Project == "" {
		return errors.New(MissingRequiredGCSRemoteStateConfig("project"))
	}

	// A location must be specified in order for terragrunt to automatically create a storage bucket.
	if client.Location == "" {
		return errors.New(MissingRequiredGCSRemoteStateConfig("location"))
	}

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

// Check if versioning is enabled for the GCS bucket specified in the given config and warn the user if it is not
func (client *Client) checkIfGCSVersioningEnabled(bucketName string) error {
	ctx := context.Background()
	bucket := client.Bucket(bucketName)

	attrs, err := bucket.Attrs(ctx)
	if err != nil {
		// ErrBucketNotExist
		return errors.New(err)
	}

	if !attrs.VersioningEnabled {
		client.logger.Warnf("Versioning is not enabled for the remote state GCS bucket %s. We recommend enabling versioning so that you can roll back to previous versions of your Terraform state in case of error.", bucketName)
	}

	return nil
}

// CreateGCSBucketWithVersioning creates the given GCS bucket and enables versioning for it.
func (client *Client) CreateGCSBucketWithVersioning(ctx context.Context, bucketName string) error {
	if err := client.CreateGCSBucket(bucketName); err != nil {
		return err
	}

	if err := client.WaitUntilGCSBucketExists(ctx, bucketName); err != nil {
		return err
	}

	if err := client.AddLabelsToGCSBucket(bucketName, client.GCSBucketLabels); err != nil {
		return err
	}

	return nil
}

func (client *Client) AddLabelsToGCSBucket(bucketName string, labels map[string]string) error {
	if len(labels) == 0 {
		client.logger.Debugf("No labels specified for bucket %s.", bucketName)
		return nil
	}

	client.logger.Debugf("Adding labels to GCS bucket with %s", labels)

	ctx := context.Background()
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
func (client *Client) CreateGCSBucket(bucketName string) error {
	client.logger.Debugf("Creating GCS bucket %s in project %s", bucketName, client.Project)

	// The project ID to which the bucket belongs. This is only used when creating a new bucket during initialization.
	// Since buckets have globally unique names, the project ID is not required to access the bucket during normal
	// operation.
	projectID := client.Project

	ctx := context.Background()
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
		return fmt.Errorf("error creating GCS bucket %s: %w", bucketName, err)
	}

	return nil
}

// WaitUntilGCSBucketExists waits for the GCS bucket specified in the given config to be created.
//
// GCP is eventually consistent, so after creating a GCS bucket, this method can be used to wait until the information
// about that GCS bucket has propagated everywhere.
func (client *Client) WaitUntilGCSBucketExists(ctx context.Context, bucketName string) error {
	client.logger.Debugf("Waiting for bucket %s to be created", bucketName)

	for retries := 0; retries < maxRetriesWaitingForGcsBucket; retries++ {
		if client.DoesGCSBucketExist(ctx, bucketName) {
			client.logger.Debugf("GCS bucket %s created.", bucketName)
			return nil
		} else if retries < maxRetriesWaitingForGcsBucket-1 {
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

// accountFile represents the structure of the Google account file JSON file.
type accountFile struct {
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	ClientID     string `json:"client_id"`
}
