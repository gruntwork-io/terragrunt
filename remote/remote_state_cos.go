package remote

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/mitchellh/mapstructure"
	"github.com/sirupsen/logrus"
	"github.com/tencentyun/cos-go-sdk-v5"
)

/*
 * We use this construct to separate the config key 'cos_bucket_labels' from the others, as they
 * are specific to the cos backend, but only used by terragrunt to tag the cos bucket in case it
 * has to create them.
 */
type ExtendedRemoteStateConfigCOS struct {
	remoteStateConfigCOS RemoteStateConfigCOS

	COSBucketLabels      map[string]string `mapstructure:"cos_bucket_labels"`
	SkipBucketVersioning bool              `mapstructure:"skip_bucket_versioning"`
	SkipBucketCreation   bool              `mapstructure:"skip_bucket_creation"`
}

// These are settings that can appear in the remote_state config that are ONLY used by Terragrunt and NOT forwarded
// to the underlying Terraform backend configuration.
var terragruntCOSOnlyConfigs = []string{
	"cos_bucket_labels",
	"skip_bucket_versioning",
	"skip_bucket_creation",
}

// A representation of the configuration options available for cos remote state
type RemoteStateConfigCOS struct {
	Bucket    string `mapstructure:"bucket"`
	SecretId  string `mapstructure:"secret_id"`
	SecretKey string `mapstructure:"secret_key"`
	Region    string `mapstructure:"region"`
	Prefix    string `mapstructure:"prefix"`
	Key       string `mapstructure:"key"`
	Encrypt   string `mapstructure:"encrypt"`
	Acl       string `mapstructure:"acl"`
}

const MAX_RETRIES_WAITING_FOR_COS_BUCKET = 12
const SLEEP_BETWEEN_RETRIES_WAITING_FOR_COS_BUCKET = 5 * time.Second

type COSInitializer struct{}

// Returns true if:
//
// 1. Any of the existing backend settings are different than the current config
// 2. The configured COS bucket does not exist
func (cosInitializer COSInitializer) NeedsInitialization(remoteState *RemoteState, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if remoteState.DisableInit {
		return false, nil
	}

	if !cosConfigValuesEqual(remoteState.Config, existingBackend, terragruntOptions) {
		return true, nil
	}

	cosConfig, err := parseCOSConfig(remoteState.Config)

	cosClient, err := CreateCOSClient(*cosConfig)
	if err != nil {
		return false, err
	}

	if !DoesCOSBucketExist(cosClient, cosConfig) {
		return true, nil
	}

	return false, nil
}

// Return true if the given config is in any way different than what is configured for the backend
func cosConfigValuesEqual(config map[string]interface{}, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) bool {
	if existingBackend == nil {
		return len(config) == 0
	}

	if existingBackend.Type != "cos" {
		terragruntOptions.Logger.Debugf("Backend type has changed from cos to %s", existingBackend.Type)
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

	// Construct a new map excluding custom COS labels that are only used in Terragrunt config and not in Terraform's backend
	comparisonConfig := make(map[string]interface{})
	for key, value := range config {
		comparisonConfig[key] = value
	}

	for _, key := range terragruntCOSOnlyConfigs {
		delete(comparisonConfig, key)
	}

	if !terraformStateConfigEqual(existingBackend.Config, comparisonConfig) {
		terragruntOptions.Logger.Debugf("Backend config changed from %s to %s", existingBackend.Config, config)
		return false
	}

	return true
}

// Initialize the remote state COS bucket specified in the given config. This function will validate the config
// parameters, create the COS bucket if it doesn't already exist, and check that versioning is enabled.
func (cosInitializer COSInitializer) Initialize(remoteState *RemoteState, terragruntOptions *options.TerragruntOptions) error {
	cosConfigExtended, err := ParseExtendedCOSConfig(remoteState.Config)
	if err != nil {
		return err
	}

	if err := validateCOSConfig(cosConfigExtended, terragruntOptions); err != nil {
		return err
	}

	var cosConfig = cosConfigExtended.remoteStateConfigCOS

	cosClient, err := CreateCOSClient(cosConfig)
	if err != nil {
		return err
	}

	// If bucket is specified and skip_bucket_creation is false then check if Bucket needs to be created
	if !cosConfigExtended.SkipBucketCreation && cosConfig.Bucket != "" {
		if err := createCOSBucketIfNecessary(cosClient, cosConfigExtended, terragruntOptions); err != nil {
			return err
		}
	}

	if !cosConfigExtended.SkipBucketVersioning && cosConfig.Bucket != "" {
		if err := checkIfCOSVersioningEnabled(cosClient, &cosConfig, terragruntOptions); err != nil {
			return err
		}
	}

	return nil
}

func (cosInitializer COSInitializer) GetTerraformInitArgs(config map[string]interface{}) map[string]interface{} {
	var filteredConfig = make(map[string]interface{})

	for key, val := range config {

		if util.ListContainsElement(terragruntOnlyConfigs, key) {
			continue
		}

		filteredConfig[key] = val
	}

	return filteredConfig
}

// Parse the given map into a COS config
func parseCOSConfig(config map[string]interface{}) (*RemoteStateConfigCOS, error) {
	var cosConfig RemoteStateConfigCOS
	if err := mapstructure.Decode(config, &cosConfig); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return &cosConfig, nil
}

// Parse the given map into a COS config
func ParseExtendedCOSConfig(config map[string]interface{}) (*ExtendedRemoteStateConfigCOS, error) {
	var cosConfig RemoteStateConfigCOS
	var extendedConfig ExtendedRemoteStateConfigCOS

	if err := mapstructure.Decode(config, &cosConfig); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if err := mapstructure.Decode(config, &extendedConfig); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	extendedConfig.remoteStateConfigCOS = cosConfig

	return &extendedConfig, nil
}

// COS is eventually consistent, so after creating a COS bucket, this method can be used to wait until the information
// about that COS bucket has propagated everywhere.
func WaitUntilCOSBucketExists(cosClient *cos.Client, config *RemoteStateConfigCOS, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Waiting for bucket %s to be created", config.Bucket)
	for retries := 0; retries < MAX_RETRIES_WAITING_FOR_COS_BUCKET; retries++ {
		if DoesCOSBucketExist(cosClient, config) {
			terragruntOptions.Logger.Debugf("COS bucket %s created.", config.Bucket)
			return nil
		} else if retries < MAX_RETRIES_WAITING_FOR_COS_BUCKET-1 {
			terragruntOptions.Logger.Debugf("COS bucket %s has not been created yet. Sleeping for %s and will check again.", config.Bucket, SLEEP_BETWEEN_RETRIES_WAITING_FOR_COS_BUCKET)
			time.Sleep(SLEEP_BETWEEN_RETRIES_WAITING_FOR_COS_BUCKET)
		}
	}

	return errors.WithStackTrace(MaxRetriesWaitingForCOSBucketExceeded(config.Bucket))
}

// DoesCOSBucketExist returns true if the COS bucket specified in the given config exists and the current user has the
// ability to access it.
func DoesCOSBucketExist(cosClient *cos.Client, config *RemoteStateConfigCOS) bool {
	ctx := context.Background()

	// Check Bucket is Exist.
	if exist, err := cosClient.Bucket.IsExist(ctx); exist && err == nil {
		return true
	}

	return false
}

// CreateCOSClient creates an authenticated client for COS
func CreateCOSClient(cosConfigRemote RemoteStateConfigCOS) (*cos.Client, error) {

	u, err := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", cosConfigRemote.Bucket, cosConfigRemote.Region))
	if err != nil {
		return nil, err
	}
	client := cos.NewClient(
		&cos.BaseURL{BucketURL: u},
		&http.Client{
			Timeout: 60 * time.Second,
			Transport: &cos.AuthorizationTransport{
				SecretID:  cosConfigRemote.SecretId,
				SecretKey: cosConfigRemote.SecretKey,
			},
		},
	)

	return client, nil
}

// Validate all the parameters of the given COS remote state configuration
func validateCOSConfig(extendedConfig *ExtendedRemoteStateConfigCOS, terragruntOptions *options.TerragruntOptions) error {
	var config = extendedConfig.remoteStateConfigCOS

	if config.Prefix == "" {
		return errors.WithStackTrace(MissingRequiredCOSRemoteStateConfig("prefix"))
	}

	return nil
}

// If the bucket specified in the given config doesn't already exist, prompt the user to create it, and if the user
// confirms, create the bucket and enable versioning for it.
func createCOSBucketIfNecessary(cosClient *cos.Client, config *ExtendedRemoteStateConfigCOS, terragruntOptions *options.TerragruntOptions) error {
	if !DoesCOSBucketExist(cosClient, &config.remoteStateConfigCOS) {
		terragruntOptions.Logger.Debugf("Remote state COS bucket %s does not exist. Attempting to create it", config.remoteStateConfigCOS.Bucket)

		prompt := fmt.Sprintf("Remote state COS bucket %s does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", config.remoteStateConfigCOS.Bucket)
		shouldCreateBucket, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
		if err != nil {
			return err
		}

		if shouldCreateBucket {
			// To avoid any eventual consistency issues with creating a cos bucket we use a retry loop.
			description := fmt.Sprintf("Create COS bucket %s", config.remoteStateConfigCOS.Bucket)
			maxRetries := 3
			sleepBetweenRetries := 10 * time.Second

			return util.DoWithRetry(description, maxRetries, sleepBetweenRetries, terragruntOptions.Logger, logrus.DebugLevel, func() error {
				return CreateCOSBucketWithVersioning(cosClient, config, terragruntOptions)
			})
		}
	}

	return nil
}

// Check if versioning is enabled for the COS bucket specified in the given config and warn the user if it is not
func checkIfCOSVersioningEnabled(cosClient *cos.Client, config *RemoteStateConfigCOS, terragruntOptions *options.TerragruntOptions) error {
	ctx := context.Background()
	res, _, err := cosClient.Bucket.GetVersioning(ctx)

	if err != nil {
		// ErrBucketNotExist
		return errors.WithStackTrace(err)
	}

	if res.Status == "Suspended" {
		terragruntOptions.Logger.Warnf("Versioning is not enabled for the remote state COS bucket %s. We recommend enabling versioning so that you can roll back to previous versions of your Terraform state in case of error.", config.Bucket)
	}

	return nil
}

// CreateCOSBucketWithVersioning creates the given COS bucket and enables versioning for it.
func CreateCOSBucketWithVersioning(cosClient *cos.Client, config *ExtendedRemoteStateConfigCOS, terragruntOptions *options.TerragruntOptions) error {
	err := CreateCOSBucket(cosClient, config, terragruntOptions)

	if err != nil {
		return err
	}

	if err := WaitUntilCOSBucketExists(cosClient, &config.remoteStateConfigCOS, terragruntOptions); err != nil {
		return err
	}

	if config.SkipBucketVersioning {
		terragruntOptions.Logger.Debugf("Versioning is disabled for the remote state COS bucket %s using 'skip_bucket_versioning' config.", config.remoteStateConfigCOS.Bucket)
	} else {
		terragruntOptions.Logger.Debugf("Enabling versioning on COS bucket %s", config.remoteStateConfigCOS.Bucket)
		option := &cos.BucketPutVersionOptions{
			Status: "Enabled",
		}
		_, err = cosClient.Bucket.PutVersioning(context.Background(), option)
	}

	if err != nil {
		return err
	}

	if err := AddLabelsToCOSBucket(cosClient, config, terragruntOptions); err != nil {
		return err
	}

	return nil
}

func AddLabelsToCOSBucket(cosClient *cos.Client, config *ExtendedRemoteStateConfigCOS, terragruntOptions *options.TerragruntOptions) error {
	if config.COSBucketLabels == nil || len(config.COSBucketLabels) == 0 {
		terragruntOptions.Logger.Debugf("No labels specified for bucket %s.", config.remoteStateConfigCOS.Bucket)
		return nil
	}

	terragruntOptions.Logger.Debugf("Adding labels to COS bucket with %s", config.COSBucketLabels)

	ctx := context.Background()

	var tagSet []cos.BucketTaggingTag
	for key, value := range config.COSBucketLabels {
		tagSet = append(tagSet, cos.BucketTaggingTag{
			Key:   key,
			Value: value,
		})
	}

	opt := &cos.BucketPutTaggingOptions{
		TagSet: tagSet,
	}

	_, err := cosClient.Bucket.PutTagging(ctx, opt)

	if err != nil {
		return errors.WithStackTrace(err)
	}

	return nil

}

// Create the COS bucket specified in the given config
func CreateCOSBucket(cosClient *cos.Client, config *ExtendedRemoteStateConfigCOS, terragruntOptions *options.TerragruntOptions) error {

	ctx := context.Background()
	opt := &cos.BucketPutOptions{
		XCosACL: "private",
	}

	_, err := cosClient.Bucket.Put(ctx, opt)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	return errors.WithStackTrace(err)
}

// Custom error types

type MaxRetriesWaitingForCOSBucketExceeded string

func (err MaxRetriesWaitingForCOSBucketExceeded) Error() string {
	return fmt.Sprintf("Exceeded max retries (%d) waiting for bucket COS bucket %s", MAX_RETRIES_WAITING_FOR_COS_BUCKET, string(err))
}

type MissingRequiredCOSRemoteStateConfig string

func (configName MissingRequiredCOSRemoteStateConfig) Error() string {
	return fmt.Sprintf("Missing required COS remote state configuration %s", string(configName))
}
