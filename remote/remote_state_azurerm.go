package remote

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-06-01/resources"
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2021-02-01/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/mitchellh/mapstructure"
	"github.com/sirupsen/logrus"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

/*
 * We use this construct to separate the config key 'azurerm_storage_account_tags' from the others, as they
 * are specific to the azurerm backend, but only used by terragrunt to tag the azurerm storage account in case it
 * has to create them.
 */
type ExtendedRemoteStateConfigAzureRM struct {
	remoteStateConfigAzureRM RemoteStateConfigAzureRM
	Location                 string             `mapstructure:"location"`
	Tags                     map[string]*string `mapstructure:"tags"`
	SKU                      string             `mapstructure:"sku"`
	Kind                     string             `mapstructure:"kind"`
	AccessTier               string             `mapstructure:"access_tier"`
	SkipCreate               bool               `mapstructure:"skip_create"`
}

// A representation of the configuration options available for AzureRM remote state
// See: https://www.terraform.io/docs/language/settings/backends/azurerm.html
//
type RemoteStateConfigAzureRM struct {
	// All these options configure the storage account, container, and blob
	TenantId           string `mapstructure:"tenant_id"`
	SubscriptionId     string `mapstructure:"subscription_id"`
	ResourceGroupName  string `mapstructure:"resource_group_name"`
	StorageAccountName string `mapstructure:"storage_account_name"`
	ContainerName      string `mapstructure:"container_name"`
	Snapshot           bool   `mapstructure:"snapshot"`
	Key                string `mapstructure:"key"`

	// // These options are all supported by the Terraform AzureRM provider, however this implementation of the
	// // Terragrunt "azurerm" backend provider does not support creating a storage account container that
	// // uses Azure AD auth, Azure Stack, or environments other than "public".
	// UseMSI                    string `mapstructure:"use_msi"`
	// MSIEndpoint               string `mapstructure:"msi_endpoint"`
	// UseAzureADAuth            bool   `mapstructure:"use_azuread_auth"`
	// AccessKey                 string `mapstructure:"access_key"`
	// SASToken                  string `mapstructure:"sas_token"`
	// ClientId                  string `mapstructure:"client_id"`
	// ClientSecret              string `mapstructure:"client_secret"`
	// ClientCertificatePassword string `mapstructure:"client_certificate_password"`
	// ClientCertificatePath     string `mapstructure:"client_certificate_path"`

	// Endpoint    string `mapstructure:"endpoint"`    // Set when using Azure Stack
	// Environment string `mapstructure:"environment"` // Set when using an environment other than "public"
}

type AzureRMClients struct {
	resourceGroups  *resources.GroupsClient
	storageAccounts *storage.AccountsClient
	blobContainers  *storage.BlobContainersClient
}

// These are settings that can appear in the remote_state config that are ONLY used by Terragrunt and NOT forwarded
// to the underlying Terraform backend configuration.
var terragruntAzureRMOnlyConfigs = []string{
	"location",
	"tags",
	"sku",
	"kind",
	"access_tier",
	"skip_versioning",
	"skip_create",
	"skip_azure_rbac",
}

type AzureRMInitializer struct{}

// Returns true if:
//
// 1. Any of the existing backend settings are different than the current config
// 2. The configured AzureRM storage account does not exist
func (armInitializer AzureRMInitializer) NeedsInitialization(remoteState *RemoteState, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if remoteState.DisableInit {
		return false, nil
	}

	if !armConfigValuesEqual(remoteState.Config, existingBackend, terragruntOptions) {
		return true, nil
	}

	armConfig, err := parseAzureRMConfig(remoteState.Config)
	if err != nil {
		return false, err
	}

	armClients, err := CreateAzureRMClients(*armConfig)
	if err != nil {
		return false, err
	}

	storageAccountExists, err := DoesStorageAccountExist(armClients, armConfig)
	if err != nil {
		return false, err
	}

	blobContainerExists, err := DoesBlobContainerExist(armClients, armConfig)
	if err != nil {
		return false, err
	}

	if !storageAccountExists || !blobContainerExists {
		return true, nil
	}

	return false, nil
}

// Return true if the given config is in any way different than what is configured for the backend
func armConfigValuesEqual(config map[string]interface{}, existingBackend *TerraformBackend, terragruntOptions *options.TerragruntOptions) bool {
	if existingBackend == nil {
		return len(config) == 0
	}

	if existingBackend.Type != "azurerm" {
		terragruntOptions.Logger.Debugf("Backend type has changed from azurerm to %s", existingBackend.Type)
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

	// Construct a new map excluding custom Azure labels that are only used in Terragrunt config and not in Terraform's backend
	comparisonConfig := make(map[string]interface{})
	for key, value := range config {
		comparisonConfig[key] = value
	}

	for _, key := range terragruntAzureRMOnlyConfigs {
		delete(comparisonConfig, key)
	}

	if !terraformStateConfigEqual(existingBackend.Config, comparisonConfig) {
		terragruntOptions.Logger.Debugf("Backend config changed from %s to %s", existingBackend.Config, config)
		return false
	}

	return true
}

// Initialize the remote state AzureRM storage account specified in the given config. This function will validate the config
// parameters, create the AzureRM storage account if it doesn't already exist, and check that versioning is enabled.
func (armInitializer AzureRMInitializer) Initialize(remoteState *RemoteState, terragruntOptions *options.TerragruntOptions) error {
	armConfigExtended, err := parseExtendedAzureRMConfig(remoteState.Config)
	if err != nil {
		return err
	}

	if err := validateAzureRMConfig(armConfigExtended, terragruntOptions); err != nil {
		return err
	}

	var armConfig = armConfigExtended.remoteStateConfigAzureRM

	armClients, err := CreateAzureRMClients(armConfig)

	if err != nil {
		return err
	}

	// If storage_account_name is specified and skip_create is false then check if the Storage Account needs to be created
	if !armConfigExtended.SkipCreate && armConfig.StorageAccountName != "" {
		if err := createResourceGroup(armClients, armConfigExtended, terragruntOptions); err != nil {
			return err
		}
		if err := createStorageAccountIfNecessary(armClients, armConfigExtended, terragruntOptions); err != nil {
			return err
		}
		if err := createBlobContainerIfNecessary(armClients, armConfigExtended, terragruntOptions); err != nil {
			return err
		}
	}

	return nil
}

func (armInitializer AzureRMInitializer) GetTerraformInitArgs(config map[string]interface{}) map[string]interface{} {
	var filteredConfig = make(map[string]interface{})

	for key, val := range config {
		if util.ListContainsElement(terragruntAzureRMOnlyConfigs, key) {
			continue
		}

		filteredConfig[key] = val
	}

	return filteredConfig
}

// Parse the given map into a AzureRM config
func parseAzureRMConfig(config map[string]interface{}) (*RemoteStateConfigAzureRM, error) {
	var armConfig RemoteStateConfigAzureRM
	if err := mapstructure.Decode(config, &armConfig); err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return &armConfig, nil
}

// Parse the given map into a AzureRM config
func parseExtendedAzureRMConfig(config map[string]interface{}) (*ExtendedRemoteStateConfigAzureRM, error) {
	var armConfig RemoteStateConfigAzureRM
	var extendedConfig ExtendedRemoteStateConfigAzureRM

	if err := mapstructure.Decode(config, &armConfig); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if err := mapstructure.Decode(config, &extendedConfig); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	extendedConfig.remoteStateConfigAzureRM = armConfig

	return &extendedConfig, nil
}

// Validate all the parameters of the given AzureRM remote state configuration
func validateAzureRMConfig(extendedConfig *ExtendedRemoteStateConfigAzureRM, terragruntOptions *options.TerragruntOptions) error {
	var config = extendedConfig.remoteStateConfigAzureRM

	if config.SubscriptionId == "" {
		return errors.WithStackTrace(MissingRequiredAzureRMRemoteStateConfig("subscription_id"))
	}

	if config.ResourceGroupName == "" {
		return errors.WithStackTrace(MissingRequiredAzureRMRemoteStateConfig("resource_group_name"))
	}

	if extendedConfig.Location == "" {
		return errors.WithStackTrace(MissingRequiredAzureRMRemoteStateConfig("location"))
	}

	if extendedConfig.AccessTier == "" {
		return errors.WithStackTrace(MissingRequiredAzureRMRemoteStateConfig("access_tier"))
	}

	if extendedConfig.SKU == "" {
		return errors.WithStackTrace(MissingRequiredAzureRMRemoteStateConfig("sku"))
	}

	if extendedConfig.Kind == "" {
		return errors.WithStackTrace(MissingRequiredAzureRMRemoteStateConfig("kind"))
	}

	return nil
}

// If the storage account specified in the given config doesn't already exist, prompt the user to create it, and if the user
// confirms, create the storage account and enable versioning for it.
func createStorageAccountIfNecessary(armClients *AzureRMClients, config *ExtendedRemoteStateConfigAzureRM, terragruntOptions *options.TerragruntOptions) error {
	doesStorageAccountExist, err := DoesStorageAccountExist(armClients, &config.remoteStateConfigAzureRM)
	if err != nil {
		return err
	}

	if !doesStorageAccountExist {
		terragruntOptions.Logger.Debugf("Remote state Azure storage account %s does not exist. Attempting to create it", config.remoteStateConfigAzureRM.StorageAccountName)

		if err := validateAzureRMConfig(config, terragruntOptions); err != nil {
			return err
		}

		prompt := fmt.Sprintf("Remote state Azure Storage Account %s does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", config.remoteStateConfigAzureRM.StorageAccountName)
		shouldCreateStorageAccount, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
		if err != nil {
			return err
		}

		if shouldCreateStorageAccount {
			// To avoid any eventual consistency issues with creating a Azure storage account we use a retry loop.
			description := fmt.Sprintf("Create Azure Storage Account %s", config.remoteStateConfigAzureRM.StorageAccountName)
			maxRetries := 3
			sleepBetweenRetries := 10 * time.Second

			return util.DoWithRetry(description, maxRetries, sleepBetweenRetries, terragruntOptions.Logger, logrus.DebugLevel, func() error {
				return createStorageAccount(armClients, config, terragruntOptions)
			})
		}
	}

	return nil
}

func createBlobContainerIfNecessary(armClients *AzureRMClients, extendedConfig *ExtendedRemoteStateConfigAzureRM, terragruntOptions *options.TerragruntOptions) error {
	config := extendedConfig.remoteStateConfigAzureRM

	doesBlobContainerExist, err := DoesBlobContainerExist(armClients, &config)
	if err != nil {
		return err
	}

	if !doesBlobContainerExist {
		terragruntOptions.Logger.Debugf("Remote state blob container %s does not exist. Attempting to create it", config.ContainerName)

		if err := validateAzureRMConfig(extendedConfig, terragruntOptions); err != nil {
			return err
		}

		prompt := fmt.Sprintf("Remote state Blob Container %s does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?", config.ContainerName)
		shouldCreateBlobContainer, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
		if err != nil {
			return err
		}

		if shouldCreateBlobContainer {
			// To avoid any eventual consistency issues with creating a Azure storage account we use a retry loop.
			description := fmt.Sprintf("Create Azure Storage Account %s", config.ContainerName)
			maxRetries := 3
			sleepBetweenRetries := 10 * time.Second

			return util.DoWithRetry(description, maxRetries, sleepBetweenRetries, terragruntOptions.Logger, logrus.DebugLevel, func() error {
				return createBlobContainer(armClients, extendedConfig, terragruntOptions)
			})
		}
	}

	return nil
}

func GetStorageAccountSku(name string) storage.SkuName {
	skus := make(map[string]storage.SkuName)
	for _, sku := range storage.PossibleSkuNameValues() {
		skus[string(sku)] = sku
	}
	return skus[name]
}

func GetStorageAccountKind(name string) storage.Kind {
	kinds := make(map[string]storage.Kind)
	for _, kind := range storage.PossibleKindValues() {
		kinds[string(kind)] = kind
	}
	return kinds[name]
}

func GetStorageAccountAccessTier(name string) storage.AccessTier {
	accessTiers := make(map[string]storage.AccessTier)
	for _, accessTier := range storage.PossibleAccessTierValues() {
		accessTiers[string(accessTier)] = accessTier
	}
	return accessTiers[name]
}

func createResourceGroup(armClients *AzureRMClients, config *ExtendedRemoteStateConfigAzureRM, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Creating Azure resource group %s in subscription: %s / region: %s", config.remoteStateConfigAzureRM.SubscriptionId, config.remoteStateConfigAzureRM.ResourceGroupName, config.Location)

	resourceGroupName := config.remoteStateConfigAzureRM.ResourceGroupName
	location := config.Location

	ctx := context.Background()

	_, err := armClients.resourceGroups.CreateOrUpdate(ctx, resourceGroupName, resources.Group{
		Location: &location,
	})

	if err != nil {
		return err
	}

	return nil
}

// Create the Azure Storage Account specified in the given config
func createStorageAccount(armClients *AzureRMClients, extendedConfig *ExtendedRemoteStateConfigAzureRM, terragruntOptions *options.TerragruntOptions) error {
	config := extendedConfig.remoteStateConfigAzureRM

	resourceGroupName := config.ResourceGroupName
	storageAccountName := config.StorageAccountName
	location := extendedConfig.Location
	tags := extendedConfig.Tags
	sku := GetStorageAccountSku(extendedConfig.SKU)
	kind := GetStorageAccountKind(extendedConfig.Kind)
	accessTier := GetStorageAccountAccessTier(extendedConfig.AccessTier)

	ctx := context.Background()

	params := storage.AccountCreateParameters{
		Sku: &storage.Sku{
			Name: sku,
			Tier: storage.SkuTierStandard,
		},
		Kind:     kind,
		Location: &location,
		AccountPropertiesCreateParameters: &storage.AccountPropertiesCreateParameters{
			AccessTier: accessTier,
		},
		Tags: tags,
	}

	json, _ := params.MarshalJSON()
	terragruntOptions.Logger.Debugf("Creating Azure storage account with parameters: %s", json)

	handle, createErr := armClients.storageAccounts.Create(ctx, resourceGroupName, storageAccountName, params)
	if createErr != nil {
		return createErr
	}

	waitErr := handle.WaitForCompletionRef(ctx, armClients.storageAccounts.Client)
	if waitErr != nil {
		return waitErr
	}

	return nil
}

// Create the Azure Storage Account specified in the given config
func createBlobContainer(armClients *AzureRMClients, extendedConfig *ExtendedRemoteStateConfigAzureRM, terragruntOptions *options.TerragruntOptions) error {
	config := extendedConfig.remoteStateConfigAzureRM

	resourceGroupName := config.ResourceGroupName
	storageAccountName := config.StorageAccountName
	containerName := config.ContainerName

	terragruntOptions.Logger.Debugf("Creating Blob Container %s", containerName)

	ctx := context.Background()

	_, createErr := armClients.blobContainers.Create(ctx, resourceGroupName, storageAccountName, containerName, storage.BlobContainer{})
	if createErr != nil {
		return createErr
	}

	return nil
}

// DoesStorageAccountExist returns true if the AzureRM storage account specified in the given config exists and the current user has the
// ability to access it.
func DoesStorageAccountExist(armClients *AzureRMClients, config *RemoteStateConfigAzureRM) (bool, error) {
	ctx := context.Background()

	storageAccountResourceType := "Microsoft.Storage/storageAccounts"
	accountCheckNameAvailabilityParameters := storage.AccountCheckNameAvailabilityParameters{
		Name: &config.StorageAccountName,
		Type: &storageAccountResourceType,
	}

	result, err := armClients.storageAccounts.CheckNameAvailability(ctx, accountCheckNameAvailabilityParameters)
	if err != nil {
		// If the name availability check fails with an error, assume the storage account exists (just to be safe)
		return true, err
	}

	if !(*result.NameAvailable) {
		// The requested name is not available. The storage account must either already exist or it is taken by someone else.
		// Storage Account names must be globally unique due to DNS uniqueness constraints.
		return true, nil
	}

	return false, nil // Fell through, this name must be available (i.e. the requested storage account must not exist)
}

// DoesBlobContainerExist returns true if the AzureRM storage account container specified in the given config exists and the current
// user has the ability to access it.
func DoesBlobContainerExist(armClients *AzureRMClients, config *RemoteStateConfigAzureRM) (bool, error) {
	ctx := context.Background()

	resourceGroupName := config.ResourceGroupName
	storageAccountName := config.StorageAccountName
	containerName := config.ContainerName

	_, err := armClients.blobContainers.Get(ctx, resourceGroupName, storageAccountName, containerName)
	if err != nil {
		return false, nil // error, container must not exist
	}

	return true, nil // fell through, container must exist
}

// CreateAzureRMClients creates an authenticated client for AzureRM
func CreateAzureRMClients(armConfig RemoteStateConfigAzureRM) (*AzureRMClients, error) {
	var armClients *AzureRMClients
	var authorizer autorest.Authorizer
	var err error

	// Allow environment variables to override credentials sourced from the Azure CLI credentials file
	authorizer, err = auth.NewAuthorizerFromEnvironment()
	if err != nil {
		authorizer, err = auth.NewAuthorizerFromCLI()
	}

	if err != nil {
		return nil, fmt.Errorf("authentication to the Azure Resource Manager failed")
	}

	resourceGroupsClient := resources.NewGroupsClient(armConfig.SubscriptionId)
	storageAccountsClient := storage.NewAccountsClient(armConfig.SubscriptionId)
	blobContainersClient := storage.NewBlobContainersClient(armConfig.SubscriptionId)

	resourceGroupsClient.Authorizer = authorizer
	storageAccountsClient.Authorizer = authorizer
	blobContainersClient.Authorizer = authorizer

	resourceGroupsClient.AddToUserAgent("terragrunt-cli")
	storageAccountsClient.AddToUserAgent("terragrunt-cli")
	blobContainersClient.AddToUserAgent("terragrunt-cli")

	armClients = &AzureRMClients{
		resourceGroups:  &resourceGroupsClient,
		storageAccounts: &storageAccountsClient,
		blobContainers:  &blobContainersClient,
	}

	return armClients, nil
}

// Custom error types

type MissingRequiredAzureRMRemoteStateConfig string

func (configName MissingRequiredAzureRMRemoteStateConfig) Error() string {
	return fmt.Sprintf("Missing required AzureRM remote state configuration %s", string(configName))
}
