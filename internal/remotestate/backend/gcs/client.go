package gcs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
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
}

// NewClient inits GCS client.
func NewClient(ctx context.Context, config *ExtendedRemoteStateConfigGCS) (*Client, error) {
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
		var account = struct {
			PrivateKeyID string `json:"private_key_id"`
			PrivateKey   string `json:"private_key"`
			ClientEmail  string `json:"client_email"`
			ClientID     string `json:"client_id"`
		}{}

		// to mirror how Terraform works, we have to accept either the file path or the contents
		creds := os.Getenv("GOOGLE_CREDENTIALS")

		contents, err := util.FileOrData(creds)
		if err != nil {
			return nil, errors.Errorf("Error loading credentials: %w", err)
		}

		if err := json.Unmarshal([]byte(contents), &account); err != nil {
			return nil, errors.Errorf("Error parsing credentials '%s': %w", contents, err)
		}

		conf := jwt.Config{
			Email:      account.ClientEmail,
			PrivateKey: []byte(account.PrivateKey),
			// We need the FullControl scope to be able to add metadata such as labels
			Scopes:   []string{storage.ScopeFullControl},
			TokenURL: tokenURL,
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
	}

	return client, nil
}

// CreateGCSBucketIfNecessary prompts the user to create the given bucket if it doesn't already exist and if the user
// confirms, creates the bucket and enables versioning for it.
func (client *Client) CreateGCSBucketIfNecessary(ctx context.Context, l log.Logger, bucketName string, opts *options.TerragruntOptions) error {
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

	l.Debugf("Remote state GCS bucket %s does not exist. Attempting to create it", bucketName)

	if opts.FailIfBucketCreationRequired {
		return backend.BucketCreationNotAllowed(bucketName)
	}

	prompt := fmt.Sprintf("Remote state GCS bucket %s does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", bucketName)

	shouldCreateBucket, err := shell.PromptUserForYesNo(ctx, l, prompt, opts)
	if err != nil {
		return err
	}

	if shouldCreateBucket {
		// To avoid any eventual consistency issues with creating a GCS bucket we use a retry loop.
		description := "Create GCS bucket " + bucketName

		return util.DoWithRetry(ctx, description, gcpMaxRetries, gcpSleepBetweenRetries, l, log.DebugLevel, func(ctx context.Context) error {
			return client.CreateGCSBucketWithVersioning(ctx, l, bucketName)
		})
	}

	return nil
}

// CheckIfGCSVersioningEnabled checks if versioning is enabled for the GCS bucket specified in the given config and warn the user if it is not
func (client *Client) CheckIfGCSVersioningEnabled(ctx context.Context, l log.Logger, bucketName string) (bool, error) {
	bucket := client.Bucket(bucketName)

	if !client.DoesGCSBucketExist(ctx, bucketName) {
		return false, backend.NewBucketDoesNotExistError(bucketName)
	}

	attrs, err := bucket.Attrs(ctx)
	if err != nil {
		// ErrBucketNotExist
		return false, errors.New(err)
	}

	if !attrs.VersioningEnabled {
		l.Warnf("Versioning is not enabled for the remote state GCS bucket %s. We recommend enabling versioning so that you can roll back to previous versions of your OpenTofu/Terraform state in case of error.", bucketName)
	}

	return attrs.VersioningEnabled, nil
}

// CreateGCSBucketWithVersioning creates the given GCS bucket and enables versioning for it.
func (client *Client) CreateGCSBucketWithVersioning(ctx context.Context, l log.Logger, bucketName string) error {
	if err := client.CreateGCSBucket(ctx, l, bucketName); err != nil {
		return err
	}

	if err := client.WaitUntilGCSBucketExists(ctx, l, bucketName); err != nil {
		return err
	}

	if err := client.AddLabelsToGCSBucket(ctx, l, bucketName, client.GCSBucketLabels); err != nil {
		return err
	}

	return nil
}

// AddLabelsToGCSBucket adds the given labels to the GCS bucket.
func (client *Client) AddLabelsToGCSBucket(ctx context.Context, l log.Logger, bucketName string, labels map[string]string) error {
	if len(labels) == 0 {
		l.Debugf("No labels specified for bucket %s.", bucketName)
		return nil
	}

	l.Debugf("Adding labels to GCS bucket with %s", labels)

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
func (client *Client) CreateGCSBucket(ctx context.Context, l log.Logger, bucketName string) error {
	l.Debugf("Creating GCS bucket %s in project %s", bucketName, client.Project)

	// The project ID to which the bucket belongs. This is only used when creating a new bucket during initialization.
	// Since buckets have globally unique names, the project ID is not required to access the bucket during normal
	// operation.
	projectID := client.Project

	bucket := client.Bucket(bucketName)

	bucketAttrs := &storage.BucketAttrs{}

	if client.Location != "" {
		l.Debugf("Creating GCS bucket in location %s.", client.Location)
		bucketAttrs.Location = client.Location
	}

	if client.SkipBucketVersioning {
		l.Debugf("Versioning is disabled for the remote state GCS bucket %s using 'skip_bucket_versioning' config.", bucketName)
	} else {
		l.Debugf("Enabling versioning on GCS bucket %s", bucketName)

		bucketAttrs.VersioningEnabled = true
	}

	if client.EnableBucketPolicyOnly {
		l.Debugf("Enabling uniform bucket-level access on GCS bucket %s", bucketName)

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
func (client *Client) WaitUntilGCSBucketExists(ctx context.Context, l log.Logger, bucketName string) error {
	l.Debugf("Waiting for bucket %s to be created", bucketName)

	for retries := range maxRetriesWaitingForGcsBucket {
		if client.DoesGCSBucketExist(ctx, bucketName) {
			l.Debugf("GCS bucket %s created.", bucketName)
			return nil
		}

		if retries < maxRetriesWaitingForGcsBucket-1 {
			l.Debugf("GCS bucket %s has not been created yet. Sleeping for %s and will check again.", bucketName, sleepBetweenRetriesWaitingForGcsBucket)
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
func (client *Client) DeleteGCSBucketIfNecessary(ctx context.Context, l log.Logger, bucketName string) error {
	if !client.DoesGCSBucketExist(ctx, bucketName) {
		return nil
	}

	description := fmt.Sprintf("Delete GCS bucket %s with retry", bucketName)

	return util.DoWithRetry(ctx, description, gcpMaxRetries, gcpSleepBetweenRetries, l, log.DebugLevel, func(ctx context.Context) error {
		if err := client.DeleteGCSObjects(ctx, l, bucketName, "", true); err != nil {
			return err
		}

		return client.DeleteGCSBucket(ctx, l, bucketName)
	})
}

func (client *Client) DeleteGCSBucket(ctx context.Context, l log.Logger, bucketName string) error {
	bucket := client.Bucket(bucketName)

	l.Debugf("Deleting GCS bucket %s", bucketName)

	if err := bucket.Delete(ctx); err != nil {
		return errors.Errorf("error deleting GCS bucket %s: %w", bucketName, err)
	}

	l.Debugf("Deleted GCS bucket %s", bucketName)

	return client.WaitUntilGCSBucketDeleted(ctx, l, bucketName)
}

// WaitUntilGCSBucketDeleted waits for the GCS bucket specified in the given config to be deleted.
func (client *Client) WaitUntilGCSBucketDeleted(ctx context.Context, l log.Logger, bucketName string) error {
	l.Debugf("Waiting for bucket %s to be deleted", bucketName)

	for retries := range maxRetriesWaitingForGcsBucket {
		if !client.DoesGCSBucketExist(ctx, bucketName) {
			l.Debugf("GCS bucket %s deleted.", bucketName)
			return nil
		} else if retries < maxRetriesWaitingForGcsBucket-1 {
			l.Debugf("GCS bucket %s has not been deleted yet. Sleeping for %s and will check again.", bucketName, sleepBetweenRetriesWaitingForGcsBucket)
			time.Sleep(sleepBetweenRetriesWaitingForGcsBucket)
		}
	}

	return errors.New(MaxRetriesWaitingForGCSBucketExceeded(bucketName))
}

// DeleteGCSObjectIfNecessary deletes the bucket objects with the given prefix if they exist.
func (client *Client) DeleteGCSObjectIfNecessary(ctx context.Context, l log.Logger, bucketName, prefix string) error {
	if !client.DoesGCSBucketExist(ctx, bucketName) {
		return nil
	}

	description := fmt.Sprintf("Delete GCS objects with prefix %s in bucket %s with retry", prefix, bucketName)

	return util.DoWithRetry(ctx, description, gcpMaxRetries, gcpSleepBetweenRetries, l, log.DebugLevel, func(ctx context.Context) error {
		return client.DeleteGCSObjects(ctx, l, bucketName, prefix, false)
	})
}

// DeleteGCSObjects deletes the bucket objects with the given prefix.
func (client *Client) DeleteGCSObjects(ctx context.Context, l log.Logger, bucketName, prefix string, withVersions bool) error {
	bucket := client.Bucket(bucketName)

	it := bucket.Objects(ctx, &storage.Query{
		Prefix:   prefix,
		Versions: withVersions,
	})

	for {
		attrs, err := it.Next()
		if err != nil {
			if errors.Is(err, iterator.Done) {
				break
			}

			return errors.Errorf("failed to get GCS object attrs: %w", err)
		}

		l.Debugf("Deleting GCS object %s with generation %d in bucket %s", attrs.Name, attrs.Generation, bucketName)

		if err := bucket.Object(attrs.Name).Generation(attrs.Generation).Delete(ctx); err != nil {
			return errors.Errorf("failed to delete object %s with generation %d in bucket %s: %w", attrs.Name, attrs.Generation, bucketName, err)
		}
	}

	return nil
}

// MoveGCSObjectIfNecessary moves the GCS object at the specified srcBucketName and srcKey to dstBucketName and dstKey.
func (client *Client) MoveGCSObjectIfNecessary(ctx context.Context, l log.Logger, srcBucketName, srcKey, dstBucketName, dstKey string) error {
	if exists, err := client.DoesGCSObjectExistWithLogging(ctx, l, srcBucketName, srcKey); err != nil || !exists {
		return err
	}

	if exists, err := client.DoesGCSObjectExist(ctx, dstBucketName, dstKey); err != nil {
		return err
	} else if exists {
		return errors.Errorf("destination GCS bucket %s object %s already exists", dstBucketName, dstKey)
	}

	description := fmt.Sprintf("Move GCS bucket object from %s to %s", path.Join(srcBucketName, srcKey), path.Join(dstBucketName, dstKey))

	return util.DoWithRetry(ctx, description, gcpMaxRetries, gcpSleepBetweenRetries, l, log.DebugLevel, func(ctx context.Context) error {
		return client.MoveGCSObject(ctx, l, srcBucketName, srcKey, dstBucketName, dstKey)
	})
}

// DoesGCSObjectExist returns true if the specified GCS object exists otherwise false.
func (client *Client) DoesGCSObjectExist(ctx context.Context, bucketName, key string) (bool, error) {
	bucket := client.Bucket(bucketName)

	obj := bucket.Object(key)

	if _, err := obj.Attrs(ctx); err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (client *Client) DoesGCSObjectExistWithLogging(ctx context.Context, l log.Logger, bucketName, key string) (bool, error) {
	if exists, err := client.DoesGCSObjectExist(ctx, bucketName, key); err != nil || exists {
		return exists, err
	}

	l.Debugf("Remote state GCS bucket %s object %s does not exist or you don't have permissions to access it.", bucketName, key)

	return false, nil
}

// MoveGCSObject copies the GCS object at the specified srcKey to dstKey and then removes srcKey.
func (client *Client) MoveGCSObject(ctx context.Context, l log.Logger, srcBucketName, srcKey, dstBucketName, dstKey string) error {
	if err := client.CopyGCSBucketObject(ctx, l, srcBucketName, srcKey, dstBucketName, dstKey); err != nil {
		return err
	}

	return client.DeleteGCSObjects(ctx, l, srcBucketName, srcKey, false)
}

// CopyGCSBucketObject copies the GCS object at the specified srcKey to dstKey.
func (client *Client) CopyGCSBucketObject(ctx context.Context, l log.Logger, srcBucketName, srcKey, dstBucketName, dstKey string) error {
	l.Debugf("Copying GCS bucket object from %s to %s", path.Join(srcBucketName, srcKey), path.Join(dstBucketName, dstKey))

	src := client.Bucket(srcBucketName).Object(srcKey)
	dst := client.Bucket(dstBucketName).Object(dstKey)

	if _, err := dst.CopierFrom(src).Run(ctx); err != nil {
		return errors.Errorf("failed to copy object: %w", err)
	}

	return nil
}
