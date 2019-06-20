package remote

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/mitchellh/mapstructure"
)

/*
 * We use this construct to separate the config key 'gcs_bucket_labels' from the others, as they
 * are specific to the gcs backend, but only used by terragrunt to tag the gcs bucket in case it
 * has to create them.
 */
type ExtendedRemoteStateConfigGCS struct {
	remoteStateConfigGCS RemoteStateConfigGCS

	Project              string            `mapstructure:"project"`
	Location             string            `mapstructure:"location"`
	GCSBucketLabels      map[string]string `mapstructure:"gcs_bucket_labels"`
	SkipBucketVersioning bool              `mapstructure:"skip_bucket_versioning"`
}

// These are settings that can appear in the remote_state config that are ONLY used by Terragrunt and NOT forwarded
// to the underlying Terraform backend configuration.
var terragruntGCSOnlyConfigs = []string{
	"project",
	"location",
	"gcs_bucket_labels",
	"skip_bucket_versioning",
}

// A representation of the configuration options available for GCS remote state
type RemoteStateConfigGCS struct {
	Bucket        string `mapstructure:"bucket"`
	Credentials   string `mapstructure:"credentials"`
	Prefix        string `mapstructure:"prefix"`
	Path          string `mapstructure:"path"`
	EncryptionKey string `mapstructure:"encryption_key"`
}

const MAX_RETRIES_WAITING_FOR_GCS_BUCKET = 12
const SLEEP_BETWEEN_RETRIES_WAITING_FOR_GCS_BUCKET = 5 * time.Second

type GCSInitializer struct{}

// Returns true if:
//
// 1. Any of the existing backend settings are different than the current config
// 2. The configured GCS bucket does not exist
func (gcsInitializer GCSInitializer) NeedsInitialization(config map[string]interface{}, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if !gcsConfigValuesEqual(config, existingBackend, terragruntOptions) {
		return true, nil
	}

	gcsConfig, err := parseGCSConfig(config)
	if err != nil {
		return false, err
	}

	gcsClient, err := CreateGCSClient()
	if err != nil {
		return false, err
	}

	if !DoesGCSBucketExist(gcsClient, gcsConfig) {
		return true, nil
	}

	return false, nil
}

// Return true if the given config is in any way different than what is configured for the backend
func gcsConfigValuesEqual(config map[string]interface{}, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) bool {
	if existingBackend == nil {
		return len(config) == 0
	}

	if existingBackend.Type != "gcs" {
		terragruntOptions.Logger.Printf("Backend type has changed from gcs to %s", existingBackend.Type)
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

	// Delete custom GCS labels that are only used in Terragrunt config and not in Terraform's backend
	for _, key := range terragruntGCSOnlyConfigs {
		delete(config, key)
	}

	if !terraformStateConfigEqual(existingBackend.Config, config) {
		terragruntOptions.Logger.Printf("Backend config has changed from %s to %s", existingBackend.Config, config)
		return false
	}

	return true
}

// Initialize the remote state GCS bucket specified in the given config. This function will validate the config
// parameters, create the GCS bucket if it doesn't already exist, and check that versioning is enabled.
func (gcsInitializer GCSInitializer) Initialize(config map[string]interface{}, terragruntOptions *options.TerragruntOptions) error {
	gcsConfigExtended, err := parseExtendedGCSConfig(config)
	if err != nil {
		return err
	}

	if err := validateGCSConfig(gcsConfigExtended, terragruntOptions); err != nil {
		return err
	}

	var gcsConfig = gcsConfigExtended.remoteStateConfigGCS

	gcsClient, err := CreateGCSClient()
	if err != nil {
		return err
	}

	if err := createGCSBucketIfNecessary(gcsClient, gcsConfigExtended, terragruntOptions); err != nil {
		return err
	}

	if err := checkIfGCSVersioningEnabled(gcsClient, &gcsConfig, terragruntOptions); err != nil {
		return err
	}

	return nil
}

func (gcsInitializer GCSInitializer) GetTerraformInitArgs(config map[string]interface{}) map[string]interface{} {
	var filteredConfig = make(map[string]interface{})

	for key, val := range config {
		if util.ListContainsElement(terragruntGCSOnlyConfigs, key) {
			continue
		}

		filteredConfig[key] = val
	}

	return filteredConfig
}

// Parse the given map into a GCS config
func parseGCSConfig(config map[string]interface{}) (*RemoteStateConfigGCS, error) {
	var gcsConfig RemoteStateConfigGCS
	if err := mapstructure.Decode(config, &gcsConfig); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return &gcsConfig, nil
}

// Parse the given map into a GCS config
func parseExtendedGCSConfig(config map[string]interface{}) (*ExtendedRemoteStateConfigGCS, error) {
	var gcsConfig RemoteStateConfigGCS
	var extendedConfig ExtendedRemoteStateConfigGCS

	if err := mapstructure.Decode(config, &gcsConfig); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if err := mapstructure.Decode(config, &extendedConfig); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	extendedConfig.remoteStateConfigGCS = gcsConfig

	return &extendedConfig, nil
}

// Validate all the parameters of the given GCS remote state configuration
func validateGCSConfig(extendedConfig *ExtendedRemoteStateConfigGCS, terragruntOptions *options.TerragruntOptions) error {
	// A project must be specified in order for terragrunt to automatically create a storage bucket.
	if extendedConfig.Project == "" {
		return errors.WithStackTrace(MissingRequiredGCSRemoteStateConfig("project"))
	}

	var config = extendedConfig.remoteStateConfigGCS

	if config.Bucket == "" {
		return errors.WithStackTrace(MissingRequiredGCSRemoteStateConfig("bucket"))
	}

	if config.Prefix == "" {
		return errors.WithStackTrace(MissingRequiredGCSRemoteStateConfig("prefix"))
	}

	return nil
}

// If the bucket specified in the given config doesn't already exist, prompt the user to create it, and if the user
// confirms, create the bucket and enable versioning for it.
func createGCSBucketIfNecessary(gcsClient *storage.Client, config *ExtendedRemoteStateConfigGCS, terragruntOptions *options.TerragruntOptions) error {
	if !DoesGCSBucketExist(gcsClient, &config.remoteStateConfigGCS) {
		prompt := fmt.Sprintf("Remote state GCS bucket %s does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", config.remoteStateConfigGCS.Bucket)
		shouldCreateBucket, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
		if err != nil {
			return err
		}

		if shouldCreateBucket {
			// To avoid any eventual consistency issues with creating a GCS bucket we use a retry loop.
			description := fmt.Sprintf("Create GCS bucket %s", config.remoteStateConfigGCS.Bucket)
			maxRetries := 3
			sleepBetweenRetries := 10 * time.Second

			return util.DoWithRetry(description, maxRetries, sleepBetweenRetries, terragruntOptions.Logger, func() error {
				return CreateGCSBucketWithVersioning(gcsClient, config, terragruntOptions)
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
		return errors.WithStackTrace(err)
	}

	if attrs.VersioningEnabled == false {
		terragruntOptions.Logger.Printf("WARNING: Versioning is not enabled for the remote state GCS bucket %s. We recommend enabling versioning so that you can roll back to previous versions of your Terraform state in case of error.", config.Bucket)
	}

	return nil
}

// CreateGCSBucketWithVersioning creates the given GCS bucket and enables versioning for it.
func CreateGCSBucketWithVersioning(gcsClient *storage.Client, config *ExtendedRemoteStateConfigGCS, terragruntOptions *options.TerragruntOptions) error {
	err := CreateGCSBucket(gcsClient, config, terragruntOptions)

	if err != nil {
		if isBucketAlreadyOwnedByYourError(err) {
			terragruntOptions.Logger.Printf("Looks like someone is creating bucket %s at the same time. Will not attempt to create it again.", config.remoteStateConfigGCS.Bucket)
			return WaitUntilGCSBucketExists(gcsClient, &config.remoteStateConfigGCS, terragruntOptions)
		}

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
	if config.GCSBucketLabels == nil || len(config.GCSBucketLabels) == 0 {
		terragruntOptions.Logger.Printf("No labels specified for bucket %s.", config.remoteStateConfigGCS.Bucket)
		return nil
	}

	terragruntOptions.Logger.Printf("Adding labels to GCS bucket with %s", config.GCSBucketLabels)

	ctx := context.Background()
	bucket := gcsClient.Bucket(config.remoteStateConfigGCS.Bucket)

	bucketAttrs := *&storage.BucketAttrsToUpdate{}

	for key, value := range config.GCSBucketLabels {
		bucketAttrs.SetLabel(key, value)
	}

	_, err := bucket.Update(ctx, bucketAttrs)

	if err != nil {
		return errors.WithStackTrace(err)
	}

	return nil

}

// Create the GCS bucket specified in the given config
func CreateGCSBucket(gcsClient *storage.Client, config *ExtendedRemoteStateConfigGCS, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Creating GCS bucket %s in project %s", config.remoteStateConfigGCS.Bucket, config.Project)

	// The project ID to which the bucket belongs. This is only used when creating a new bucket during initialization.
	// Since buckets have globally unique names, the project ID is not required to access the bucket during normal
	// operation.
	projectID := config.Project

	ctx := context.Background()
	bucket := gcsClient.Bucket(config.remoteStateConfigGCS.Bucket)

	bucketAttrs := &storage.BucketAttrs{}

	if config.Location != "" {
		terragruntOptions.Logger.Printf("Creating GCS bucket in location %s.", config.Location)
	}

	if config.SkipBucketVersioning {
		terragruntOptions.Logger.Printf("Versioning is disabled for the remote state GCS bucket %s using 'skip_bucket_versioning' config.", config.remoteStateConfigGCS.Bucket)
	} else {
		terragruntOptions.Logger.Printf("Enabling versioning on GCS bucket %s", config.remoteStateConfigGCS.Bucket)
		bucketAttrs.VersioningEnabled = true
	}

	err := bucket.Create(ctx, projectID, bucketAttrs)
	return errors.WithStackTrace(err)
}

// GCP is eventually consistent, so after creating a GCS bucket, this method can be used to wait until the information
// about that GCS bucket has propagated everywhere.
func WaitUntilGCSBucketExists(gcsClient *storage.Client, config *RemoteStateConfigGCS, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Waiting for bucket %s to be created", config.Bucket)
	for retries := 0; retries < MAX_RETRIES_WAITING_FOR_GCS_BUCKET; retries++ {
		if DoesGCSBucketExist(gcsClient, config) {
			terragruntOptions.Logger.Printf("GCS bucket %s created.", config.Bucket)
			return nil
		} else if retries < MAX_RETRIES_WAITING_FOR_GCS_BUCKET-1 {
			terragruntOptions.Logger.Printf("GCS bucket %s has not been created yet. Sleeping for %s and will check again.", config.Bucket, SLEEP_BETWEEN_RETRIES_WAITING_FOR_GCS_BUCKET)
			time.Sleep(SLEEP_BETWEEN_RETRIES_WAITING_FOR_GCS_BUCKET)
		}
	}

	return errors.WithStackTrace(MaxRetriesWaitingForS3BucketExceeded(config.Bucket))
}

// DoesGCSBucketExist returns true if the GCS bucket specified in the given config exists and the current user has the
// ability to access it.
func DoesGCSBucketExist(gcsClient *storage.Client, config *RemoteStateConfigGCS) bool {
	ctx := context.Background()

	// Creates a Bucket instance.
	bucket := gcsClient.Bucket(config.Bucket)

	// TODO - the code below attempts to determine whether the storage bucket exists by making a making a number of API
	// calls, then attemping to list the contents of the bucket. It was adapted from Google's own integration tests and
	// should be improved once the appropriate API call is added. For more info see:
	// https://github.com/GoogleCloudPlatform/google-cloud-go/blob/de879f7be552d57556875b8aaa383bce9396cc8c/storage/integration_test.go#L1231
	if _, err := bucket.Attrs(ctx); err != nil {
		// ErrBucketNotExist
		return false
	}

	it := bucket.Objects(ctx, nil)
	if _, err := it.Next(); err == storage.ErrBucketNotExist {
		return false
	}

	return true
}

// CreateGCSClient creates an authenticated client for GCS
func CreateGCSClient() (*storage.Client, error) {
	ctx := context.Background()

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// Custom error types

type MissingRequiredGCSRemoteStateConfig string

func (configName MissingRequiredGCSRemoteStateConfig) Error() string {
	return fmt.Sprintf("Missing required GCS remote state configuration %s", string(configName))
}

type MaxRetriesWaitingForGCSBucketExceeded string

func (err MaxRetriesWaitingForGCSBucketExceeded) Error() string {
	return fmt.Sprintf("Exceeded max retries (%d) waiting for GCS bucket %s", MAX_RETRIES_WAITING_FOR_GCS_BUCKET, string(err))
}

type MaxRetriesWaitingForGCSACLExceeded string

func (err MaxRetriesWaitingForGCSACLExceeded) Error() string {
	return fmt.Sprintf("Exceeded max retries waiting for GCS bucket %s to have proper ACL for access logging", string(err))
}
