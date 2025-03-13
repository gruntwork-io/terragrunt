package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	"google.golang.org/api/impersonate"

	"maps"

	"cloud.google.com/go/storage"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/option"
)

/* ExtendedRemoteStateConfigGCS is a struct that contains the GCS specific configuration options.
 *
 * We use this construct to separate the config key 'gcs_bucket_labels' from the others, as they
 * are specific to the gcs backend, but only used by terragrunt to tag the gcs bucket in case it
 * has to create them.
 */
type ExtendedRemoteStateConfigGCS struct {
	remoteStateConfigGCS RemoteStateConfigGCS

	Project                string            `mapstructure:"project"`
	Location               string            `mapstructure:"location"`
	GCSBucketLabels        map[string]string `mapstructure:"gcs_bucket_labels"`
	SkipBucketVersioning   bool              `mapstructure:"skip_bucket_versioning"`
	SkipBucketCreation     bool              `mapstructure:"skip_bucket_creation"`
	EnableBucketPolicyOnly bool              `mapstructure:"enable_bucket_policy_only"`
}

// These are settings that can appear in the remote_state config that are ONLY used by Terragrunt and NOT forwarded
// to the underlying Terraform backend configuration.
var terragruntGCSOnlyConfigs = []string{
	"project",
	"location",
	"gcs_bucket_labels",
	"skip_bucket_versioning",
	"skip_bucket_creation",
	"enable_bucket_policy_only",
}

// RemoteStateConfigGCS is a representation of the configuration
// options available for GCS remote state.
type RemoteStateConfigGCS struct {
	Bucket        string `mapstructure:"bucket"`
	Credentials   string `mapstructure:"credentials"`
	AccessToken   string `mapstructure:"access_token"`
	Prefix        string `mapstructure:"prefix"`
	Path          string `mapstructure:"path"`
	EncryptionKey string `mapstructure:"encryption_key"`

	ImpersonateServiceAccount          string   `mapstructure:"impersonate_service_account"`
	ImpersonateServiceAccountDelegates []string `mapstructure:"impersonate_service_account_delegates"`
}

// accountFile represents the structure of the Google account file JSON file.
type accountFile struct {
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	ClientID     string `json:"client_id"`
}

const MaxRetriesWaitingForGcsBucket = 12
const SleepBetweenRetriesWaitingForGcsBucket = 5 * time.Second

const (
	gcpMaxRetries          = 3
	gcpSleepBetweenRetries = 10 * time.Second
)

type GCSInitializer struct{}

// NeedsInitialization returns true if the GCS bucket specified in the given config does not exist or if the bucket
// exists but versioning is not enabled.
//
// Returns true if:
//
// 1. Any of the existing backend settings are different than the current config
// 2. The configured GCS bucket does not exist
func (initializer GCSInitializer) NeedsInitialization(remoteState *RemoteState, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if remoteState.DisableInit {
		return false, nil
	}

	project := remoteState.Config["project"]

	if !GCSConfigValuesEqual(remoteState.Config, existingBackend, terragruntOptions) {
		return true, nil
	}

	if project != nil {
		remoteState.Config["project"] = project
	}

	gcsConfig, err := parseGCSConfig(remoteState.Config)
	if err != nil {
		return false, err
	}

	ctx := context.Background()

	gcsClient, err := CreateGCSClient(ctx, *gcsConfig)
	if err != nil {
		return false, err
	}

	bucketHandle := gcsClient.Bucket(gcsConfig.Bucket)

	if !DoesGCSBucketExist(ctx, bucketHandle) {
		return true, nil
	}

	if project != nil {
		delete(remoteState.Config, "project")
	}

	return false, nil
}

// GCSConfigValuesEqual returns true if the given config is in any way different
// than what is configured for the backend.
func GCSConfigValuesEqual(config map[string]any, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) bool {
	if existingBackend == nil {
		return len(config) == 0
	}

	if existingBackend.Type != "gcs" {
		terragruntOptions.Logger.Debugf("Backend type has changed from gcs to %s", existingBackend.Type)
		return false
	}

	if len(config) == 0 && len(existingBackend.Config) == 0 {
		return true
	}

	// If other keys in config are bools, DeepEqual also will consider the maps to be different.
	for key, value := range existingBackend.Config {
		if util.KindOf(existingBackend.Config[key]) == reflect.String && util.KindOf(config[key]) == reflect.Bool {
			if convertedValue, err := strconv.ParseBool(value.(string)); err == nil {
				existingBackend.Config[key] = convertedValue
			}
		}
	}

	// Construct a new map excluding custom GCS labels that are only used in Terragrunt config and not in Terraform's backend
	comparisonConfig := make(map[string]any)
	maps.Copy(comparisonConfig, config)

	for _, key := range terragruntGCSOnlyConfigs {
		delete(comparisonConfig, key)
	}

	if !terraformStateConfigEqual(existingBackend.Config, comparisonConfig) {
		terragruntOptions.Logger.Debugf("Backend config changed from %s to %s", existingBackend.Config, config)
		return false
	}

	return true
}

// buildInitializerCacheKey returns a unique key for the given GCS config that can be used to cache the initialization
func (initializer GCSInitializer) buildInitializerCacheKey(gcsConfig *RemoteStateConfigGCS) string {
	return gcsConfig.Bucket
}

// Initialize the remote state GCS bucket specified in the given config. This function will validate the config
// parameters, create the GCS bucket if it doesn't already exist, and check that versioning is enabled.
func (initializer GCSInitializer) Initialize(ctx context.Context, remoteState *RemoteState, terragruntOptions *options.TerragruntOptions) error {
	gcsConfigExtended, err := ParseExtendedGCSConfig(remoteState.Config)
	if err != nil {
		return err
	}

	if !gcsConfigExtended.SkipBucketCreation {
		if err := ValidateGCSConfig(ctx, gcsConfigExtended); err != nil {
			return err
		}
	}

	var gcsConfig = gcsConfigExtended.remoteStateConfigGCS

	cacheKey := initializer.buildInitializerCacheKey(&gcsConfig)
	if initialized, hit := initializedRemoteStateCache.Get(ctx, cacheKey); initialized && hit {
		terragruntOptions.Logger.Debugf("GCS bucket %s has already been confirmed to be initialized, skipping initialization checks", gcsConfig.Bucket)
		return nil
	}

	// ensure that only one goroutine can initialize bucket
	return stateAccessLock.StateBucketUpdate(gcsConfig.Bucket, func() error {
		// check if another goroutine has already initialized the bucket
		if initialized, hit := initializedRemoteStateCache.Get(ctx, cacheKey); initialized && hit {
			terragruntOptions.Logger.Debugf("GCS bucket %s has already been confirmed to be initialized, skipping initialization checks", gcsConfig.Bucket)
			return nil
		}

		// TODO: Remove lint suppression
		gcsClient, err := CreateGCSClient(ctx, gcsConfig) //nolint:contextcheck
		if err != nil {
			return err
		}

		// If bucket is specified and skip_bucket_creation is false then check if Bucket needs to be created
		if !gcsConfigExtended.SkipBucketCreation && gcsConfig.Bucket != "" {
			if err := createGCSBucketIfNecessary(ctx, gcsClient, gcsConfigExtended, terragruntOptions); err != nil {
				return err
			}
		}
		// If bucket is specified and skip_bucket_versioning is false then warn user if versioning is disabled on bucket
		if !gcsConfigExtended.SkipBucketVersioning && gcsConfig.Bucket != "" {
			// TODO: Remove lint suppression
			if err := checkIfGCSVersioningEnabled(gcsClient, &gcsConfig, terragruntOptions); err != nil { //nolint:contextcheck
				return err
			}
		}

		initializedRemoteStateCache.Put(ctx, cacheKey, true)

		return nil
	})
}

// GetTerraformInitArgs returns the subset of the given config that should be passed to terraform init
// when initializing the remote state.
func (initializer GCSInitializer) GetTerraformInitArgs(config map[string]any) map[string]any {
	var filteredConfig = make(map[string]any)

	for key, val := range config {
		if util.ListContainsElement(terragruntGCSOnlyConfigs, key) {
			continue
		}

		filteredConfig[key] = val
	}

	return filteredConfig
}

// Parse the given map into a GCS config
func parseGCSConfig(config map[string]any) (*RemoteStateConfigGCS, error) {
	var gcsConfig RemoteStateConfigGCS
	if err := mapstructure.Decode(config, &gcsConfig); err != nil {
		return nil, errors.New(err)
	}

	return &gcsConfig, nil
}

// ParseExtendedGCSConfig parses the given map into a GCS config.
func ParseExtendedGCSConfig(config map[string]any) (*ExtendedRemoteStateConfigGCS, error) {
	var (
		gcsConfig      RemoteStateConfigGCS
		extendedConfig ExtendedRemoteStateConfigGCS
	)

	if err := mapstructure.Decode(config, &gcsConfig); err != nil {
		return nil, errors.New(err)
	}

	if err := mapstructure.Decode(config, &extendedConfig); err != nil {
		return nil, errors.New(err)
	}

	extendedConfig.remoteStateConfigGCS = gcsConfig

	return &extendedConfig, nil
}

// ValidateGCSConfig validates the configuration for GCS remote state.
func ValidateGCSConfig(ctx context.Context, extendedConfig *ExtendedRemoteStateConfigGCS) error {
	config := extendedConfig.remoteStateConfigGCS

	// Bucket is always a required configuration parameter when not skipping bucket creation
	// so we check it here to make sure we have handle to the bucket
	// before we start validating the rest of the configuration.
	if config.Bucket == "" {
		return errors.New(MissingRequiredGCSRemoteStateConfig("bucket"))
	}

	// Create a GCS client to check bucket existence
	gcsClient, err := CreateGCSClient(ctx, config)
	if err != nil {
		return fmt.Errorf("error creating GCS client: %w", err)
	}

	defer func() {
		if closeErr := gcsClient.Close(); closeErr != nil {
			log.Warnf("Error closing GCS client: %v", closeErr)
		}
	}()

	bucketHandle := gcsClient.Bucket(config.Bucket)

	if err := ValidateGCSConfigWithHandle(ctx, bucketHandle, extendedConfig); err != nil {
		return err
	}

	return nil
}

// ValidateGCSConfigWithHandle validates the configuration for GCS remote state.
func ValidateGCSConfigWithHandle(ctx context.Context, bucketHandle BucketHandle, extendedConfig *ExtendedRemoteStateConfigGCS) error {
	config := extendedConfig.remoteStateConfigGCS

	// Bucket is always a required configuration parameter
	if config.Bucket == "" {
		return errors.New(MissingRequiredGCSRemoteStateConfig("bucket"))
	}

	// If both project and location are provided, the configuration is valid
	if extendedConfig.Project != "" && extendedConfig.Location != "" {
		return nil
	}

	// Check if the bucket exists
	bucketExists := DoesGCSBucketExist(ctx, bucketHandle)
	if bucketExists {
		return nil
	}

	// At this point, the bucket doesn't exist and we need both project and location
	if extendedConfig.Project == "" {
		return errors.New(MissingRequiredGCSRemoteStateConfig("project"))
	}

	if extendedConfig.Location == "" {
		return errors.New(MissingRequiredGCSRemoteStateConfig("location"))
	}

	return nil
}

// If the bucket specified in the given config doesn't already exist, prompt the user to create it, and if the user
// confirms, create the bucket and enable versioning for it.
func createGCSBucketIfNecessary(ctx context.Context, gcsClient *storage.Client, config *ExtendedRemoteStateConfigGCS, terragruntOptions *options.TerragruntOptions) error {
	bucketHandle := gcsClient.Bucket(config.remoteStateConfigGCS.Bucket)

	if !DoesGCSBucketExist(ctx, bucketHandle) {
		terragruntOptions.Logger.Debugf("Remote state GCS bucket %s does not exist. Attempting to create it", config.remoteStateConfigGCS.Bucket)

		// A project must be specified in order for terragrunt to automatically create a storage bucket.
		if config.Project == "" {
			return errors.New(MissingRequiredGCSRemoteStateConfig("project"))
		}

		// A location must be specified in order for terragrunt to automatically create a storage bucket.
		if config.Location == "" {
			return errors.New(MissingRequiredGCSRemoteStateConfig("location"))
		}

		if terragruntOptions.FailIfBucketCreationRequired {
			return BucketCreationNotAllowed(config.remoteStateConfigGCS.Bucket)
		}

		prompt := fmt.Sprintf("Remote state GCS bucket %s does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", config.remoteStateConfigGCS.Bucket)

		shouldCreateBucket, err := shell.PromptUserForYesNo(ctx, prompt, terragruntOptions)
		if err != nil {
			return err
		}

		if shouldCreateBucket {
			// To avoid any eventual consistency issues with creating a GCS bucket we use a retry loop.
			description := "Create GCS bucket " + config.remoteStateConfigGCS.Bucket

			return util.DoWithRetry(ctx, description, gcpMaxRetries, gcpSleepBetweenRetries, terragruntOptions.Logger, log.DebugLevel, func(ctx context.Context) error {
				// TODO: Remove lint suppression
				return CreateGCSBucketWithVersioning(gcsClient, config, terragruntOptions) //nolint:contextcheck
			})
		}
	}

	return nil
}

// Check if versioning is enabled for the GCS bucket specified in the given config and warn the user if it is not
func checkIfGCSVersioningEnabled(gcsClient *storage.Client, config *RemoteStateConfigGCS, terragruntOptions *options.TerragruntOptions) error {
	ctx := context.Background()
	bucket := gcsClient.Bucket(config.Bucket)

	attrs, err := bucket.Attrs(ctx)
	if err != nil {
		// ErrBucketNotExist
		return errors.New(err)
	}

	if !attrs.VersioningEnabled {
		terragruntOptions.Logger.Warnf("Versioning is not enabled for the remote state GCS bucket %s. We recommend enabling versioning so that you can roll back to previous versions of your Terraform state in case of error.", config.Bucket)
	}

	return nil
}

// CreateGCSBucketWithVersioning creates the given GCS bucket and enables versioning for it.
func CreateGCSBucketWithVersioning(gcsClient *storage.Client, config *ExtendedRemoteStateConfigGCS, terragruntOptions *options.TerragruntOptions) error {
	err := CreateGCSBucket(gcsClient, config, terragruntOptions)

	if err != nil {
		return err
	}

	if err := WaitUntilGCSBucketExists(gcsClient, &config.remoteStateConfigGCS, terragruntOptions); err != nil {
		return err
	}

	if err := AddLabelsToGCSBucket(gcsClient, config, terragruntOptions); err != nil {
		return err
	}

	return nil
}

func AddLabelsToGCSBucket(gcsClient *storage.Client, config *ExtendedRemoteStateConfigGCS, terragruntOptions *options.TerragruntOptions) error {
	if len(config.GCSBucketLabels) == 0 {
		terragruntOptions.Logger.Debugf("No labels specified for bucket %s.", config.remoteStateConfigGCS.Bucket)
		return nil
	}

	terragruntOptions.Logger.Debugf("Adding labels to GCS bucket with %s", config.GCSBucketLabels)

	ctx := context.Background()
	bucket := gcsClient.Bucket(config.remoteStateConfigGCS.Bucket)

	bucketAttrs := storage.BucketAttrsToUpdate{}

	for key, value := range config.GCSBucketLabels {
		bucketAttrs.SetLabel(key, value)
	}

	_, err := bucket.Update(ctx, bucketAttrs)

	if err != nil {
		return errors.New(err)
	}

	return nil
}

// CreateGCSBucket creates the GCS bucket specified in the given config.
func CreateGCSBucket(gcsClient *storage.Client, config *ExtendedRemoteStateConfigGCS, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Creating GCS bucket %s in project %s", config.remoteStateConfigGCS.Bucket, config.Project)

	// The project ID to which the bucket belongs. This is only used when creating a new bucket during initialization.
	// Since buckets have globally unique names, the project ID is not required to access the bucket during normal
	// operation.
	projectID := config.Project

	ctx := context.Background()
	bucket := gcsClient.Bucket(config.remoteStateConfigGCS.Bucket)

	bucketAttrs := &storage.BucketAttrs{}

	if config.Location != "" {
		terragruntOptions.Logger.Debugf("Creating GCS bucket in location %s.", config.Location)
		bucketAttrs.Location = config.Location
	}

	if config.SkipBucketVersioning {
		terragruntOptions.Logger.Debugf("Versioning is disabled for the remote state GCS bucket %s using 'skip_bucket_versioning' config.", config.remoteStateConfigGCS.Bucket)
	} else {
		terragruntOptions.Logger.Debugf("Enabling versioning on GCS bucket %s", config.remoteStateConfigGCS.Bucket)

		bucketAttrs.VersioningEnabled = true
	}

	if config.EnableBucketPolicyOnly {
		terragruntOptions.Logger.Debugf("Enabling uniform bucket-level access on GCS bucket %s", config.remoteStateConfigGCS.Bucket)

		bucketAttrs.BucketPolicyOnly = storage.BucketPolicyOnly{Enabled: true}
	}

	if err := bucket.Create(ctx, projectID, bucketAttrs); err != nil {
		return fmt.Errorf("error creating GCS bucket %s: %w", config.remoteStateConfigGCS.Bucket, err)
	}

	return nil
}

// WaitUntilGCSBucketExists waits for the GCS bucket specified in the given config to be created.
//
// GCP is eventually consistent, so after creating a GCS bucket, this method can be used to wait until the information
// about that GCS bucket has propagated everywhere.
func WaitUntilGCSBucketExists(gcsClient *storage.Client, config *RemoteStateConfigGCS, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Waiting for bucket %s to be created", config.Bucket)

	bucketHandle := gcsClient.Bucket(config.Bucket)

	for retries := range MaxRetriesWaitingForGcsBucket {
		if DoesGCSBucketExist(context.Background(), bucketHandle) {
			terragruntOptions.Logger.Debugf("GCS bucket %s created.", config.Bucket)
			return nil
		} else if retries < MaxRetriesWaitingForGcsBucket-1 {
			terragruntOptions.Logger.Debugf("GCS bucket %s has not been created yet. Sleeping for %s and will check again.", config.Bucket, SleepBetweenRetriesWaitingForGcsBucket)
			time.Sleep(SleepBetweenRetriesWaitingForGcsBucket)
		}
	}

	return errors.New(MaxRetriesWaitingForS3BucketExceeded(config.Bucket))
}

// DoesGCSBucketExist returns true if the GCS bucket specified in the given config exists and the current user has the
// ability to access it.
func DoesGCSBucketExist(ctx context.Context, bucketHandle BucketHandle) bool {
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

type BucketHandle interface {
	Attrs(ctx context.Context) (*storage.BucketAttrs, error)
	Objects(ctx context.Context, q *storage.Query) *storage.ObjectIterator
}

// CreateGCSClient creates an authenticated client for GCS
func CreateGCSClient(ctx context.Context, gcsConfigRemote RemoteStateConfigGCS) (*storage.Client, error) {
	var opts []option.ClientOption

	if gcsConfigRemote.Credentials != "" {
		opts = append(opts, option.WithCredentialsFile(gcsConfigRemote.Credentials))
	} else if gcsConfigRemote.AccessToken != "" {
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: gcsConfigRemote.AccessToken,
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

	if gcsConfigRemote.ImpersonateServiceAccount != "" {
		ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: gcsConfigRemote.ImpersonateServiceAccount,
			Scopes:          []string{storage.ScopeFullControl},
			Delegates:       gcsConfigRemote.ImpersonateServiceAccountDelegates,
		})
		if err != nil {
			return nil, err
		}

		opts = append(opts, option.WithTokenSource(ts))
	}

	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// Custom error types

type MissingRequiredGCSRemoteStateConfig string

func (configName MissingRequiredGCSRemoteStateConfig) Error() string {
	return "Missing required GCS remote state configuration " + string(configName)
}
