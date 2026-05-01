// Package azurehelper -- storage account management.
//
// StorageAccountClient wraps Azure's armstorage AccountsClient and
// BlobServicesClient to expose the small set of operations Terragrunt's
// remote-state bootstrap needs: existence check, idempotent create,
// blob versioning + soft-delete configuration, key listing, and delete.
package azurehelper

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"

	tgerrors "github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Default storage account values applied by StorageAccountConfig.withDefaults
// when the corresponding field is empty. Mirrors azurerm_storage reference.
const (
	defaultAccountKind     = "StorageV2"
	defaultAccountTier     = "Standard"
	defaultReplicationType = "LRS"
	defaultAccessTier      = "Hot"
	defaultSoftDeleteDays  = 7
)

// StorageAccountConfig is the input to (*StorageAccountClient).Create.
//
// Only Name, ResourceGroupName, and Location are required. The remaining
// fields fall back to the package defaults documented above.
type StorageAccountConfig struct {
	// Tags are applied to the created storage account. The "created-by"
	// tag is added automatically when missing.
	Tags map[string]string

	// Name is the storage account name (3-24 lowercase alphanumeric).
	Name string

	// ResourceGroupName is the resource group that will own the account.
	ResourceGroupName string

	// Location is the Azure region (e.g. "eastus").
	Location string

	// AccountKind is the storage account kind (e.g. "StorageV2"). Default: StorageV2.
	AccountKind string

	// AccountTier is "Standard" or "Premium". Default: Standard.
	AccountTier string

	// ReplicationType is e.g. "LRS", "GRS", "ZRS". Default: LRS.
	ReplicationType string

	// AccessTier is "Hot" or "Cool". Default: Hot.
	AccessTier string

	// EnableVersioning configures blob versioning after the account exists.
	EnableVersioning bool

	// AllowBlobPublicAccess controls whether containers may permit anonymous access.
	AllowBlobPublicAccess bool
}

// StorageAccountClient manages an Azure storage account via the ARM
// control plane. It is bound at construction to a single subscription,
// resource group, and account name.
type StorageAccountClient struct {
	accounts       *armstorage.AccountsClient
	blobServices   *armstorage.BlobServicesClient
	subscriptionID string
	resourceGroup  string
	accountName    string
}

// NewStorageAccountClient constructs a StorageAccountClient bound to the
// account named by cfg.AccountName in cfg.ResourceGroup. cfg must have a
// non-nil Credential (SAS-token and access-key auth methods cannot reach
// the ARM control plane).
func NewStorageAccountClient(cfg *AzureConfig) (*StorageAccountClient, error) {
	if cfg == nil {
		return nil, tgerrors.Errorf("azure config is required")
	}

	if cfg.SubscriptionID == "" {
		return nil, tgerrors.Errorf("subscription_id is required for storage account management")
	}

	if cfg.Credential == nil {
		return nil, tgerrors.Errorf("storage account management requires a token credential (SAS-token / access-key auth is data-plane only)")
	}

	if cfg.AccountName == "" {
		return nil, tgerrors.Errorf("storage account name is required")
	}

	if cfg.ResourceGroup == "" {
		return nil, tgerrors.Errorf("resource group name is required for storage account management")
	}

	armOpts := &arm.ClientOptions{ClientOptions: cfg.ClientOptions}

	accounts, err := armstorage.NewAccountsClient(cfg.SubscriptionID, cfg.Credential, armOpts)
	if err != nil {
		return nil, tgerrors.Errorf("creating armstorage accounts client: %w", err)
	}

	blobServices, err := armstorage.NewBlobServicesClient(cfg.SubscriptionID, cfg.Credential, armOpts)
	if err != nil {
		return nil, tgerrors.Errorf("creating armstorage blob services client: %w", err)
	}

	return &StorageAccountClient{
		accounts:       accounts,
		blobServices:   blobServices,
		subscriptionID: cfg.SubscriptionID,
		resourceGroup:  cfg.ResourceGroup,
		accountName:    cfg.AccountName,
	}, nil
}

// AccountName returns the storage account name this client targets.
func (c *StorageAccountClient) AccountName() string { return c.accountName }

// ResourceGroup returns the resource group containing the account.
func (c *StorageAccountClient) ResourceGroup() string { return c.resourceGroup }

// Exists returns true if the storage account exists in the configured
// resource group. A 404 from ARM yields (false, nil); any other error
// is wrapped and returned.
func (c *StorageAccountClient) Exists(ctx context.Context) (bool, error) {
	_, err := c.accounts.GetProperties(ctx, c.resourceGroup, c.accountName, nil)
	if err == nil {
		return true, nil
	}

	if isStatusCode(err, http.StatusNotFound) {
		return false, nil
	}

	return false, WrapError(err, fmt.Sprintf("get storage account %q", c.accountName))
}

// Create creates the storage account described by in. cfg.Name and
// cfg.ResourceGroupName, when non-empty, must equal the values bound to
// the client; mismatches return an error so callers cannot accidentally
// target a different account.
//
// The call blocks until ARM reports the create operation as complete.
func (c *StorageAccountClient) Create(ctx context.Context, l log.Logger, in *StorageAccountConfig) error {
	if in == nil {
		return tgerrors.Errorf("StorageAccountConfig is required")
	}

	if in.Name != "" && in.Name != c.accountName {
		return tgerrors.Errorf("StorageAccountConfig.Name %q does not match client account name %q", in.Name, c.accountName)
	}

	if in.ResourceGroupName != "" && in.ResourceGroupName != c.resourceGroup {
		return tgerrors.Errorf("StorageAccountConfig.ResourceGroupName %q does not match client resource group %q", in.ResourceGroupName, c.resourceGroup)
	}

	if in.Location == "" {
		return tgerrors.Errorf("Location is required to create storage account %q", c.accountName)
	}

	in.withDefaults()

	skuName := armstorage.SKUName(in.AccountTier + "_" + in.ReplicationType)
	kind := armstorage.Kind(in.AccountKind)
	accessTier := accessTierValue(in.AccessTier)

	params := armstorage.AccountCreateParameters{
		Kind:     &kind,
		Location: to.Ptr(in.Location),
		SKU:      &armstorage.SKU{Name: &skuName},
		Tags:     stringMapPtr(in.Tags),
		Properties: &armstorage.AccountPropertiesCreateParameters{
			AccessTier:            accessTier,
			AllowBlobPublicAccess: to.Ptr(in.AllowBlobPublicAccess),
		},
	}

	l.Debugf("azurehelper: creating storage account %q in %q (%s, %s)", c.accountName, c.resourceGroup, kind, skuName)

	poller, err := c.accounts.BeginCreate(ctx, c.resourceGroup, c.accountName, params, nil)
	if err != nil {
		return WrapError(err, fmt.Sprintf("begin create storage account %q", c.accountName))
	}

	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return WrapError(err, fmt.Sprintf("create storage account %q", c.accountName))
	}

	return nil
}

// Delete deletes the storage account. Returns nil if the account does
// not exist (idempotent).
func (c *StorageAccountClient) Delete(ctx context.Context, l log.Logger) error {
	l.Debugf("azurehelper: deleting storage account %q in %q", c.accountName, c.resourceGroup)

	if _, err := c.accounts.Delete(ctx, c.resourceGroup, c.accountName, nil); err != nil {
		if isStatusCode(err, http.StatusNotFound) {
			return nil
		}

		return WrapError(err, fmt.Sprintf("delete storage account %q", c.accountName))
	}

	return nil
}

// EnableVersioning turns on blob versioning for the account's default
// blob service. Calling on an account that already has versioning
// enabled is a no-op from the caller's perspective.
func (c *StorageAccountClient) EnableVersioning(ctx context.Context, l log.Logger) error {
	l.Debugf("azurehelper: enabling blob versioning on %q", c.accountName)

	props := armstorage.BlobServiceProperties{
		BlobServiceProperties: &armstorage.BlobServicePropertiesProperties{
			IsVersioningEnabled: to.Ptr(true),
		},
	}

	if _, err := c.blobServices.SetServiceProperties(ctx, c.resourceGroup, c.accountName, props, nil); err != nil {
		return WrapError(err, fmt.Sprintf("enable versioning on %q", c.accountName))
	}

	return nil
}

// IsVersioningEnabled returns true if blob versioning is on.
func (c *StorageAccountClient) IsVersioningEnabled(ctx context.Context) (bool, error) {
	resp, err := c.blobServices.GetServiceProperties(ctx, c.resourceGroup, c.accountName, nil)
	if err != nil {
		return false, WrapError(err, fmt.Sprintf("get blob service properties for %q", c.accountName))
	}

	if resp.BlobServiceProperties.BlobServiceProperties == nil {
		return false, nil
	}

	v := resp.BlobServiceProperties.BlobServiceProperties.IsVersioningEnabled

	return v != nil && *v, nil
}

// EnableSoftDelete configures container and blob soft-delete with the
// supplied retention. retentionDays must be 1-365; values outside that
// range are clamped to defaultSoftDeleteDays and the clamping is logged
// at WARN so the caller can spot a typo.
func (c *StorageAccountClient) EnableSoftDelete(ctx context.Context, l log.Logger, retentionDays int) error {
	if retentionDays < 1 || retentionDays > 365 {
		l.Warnf("azurehelper: soft-delete retention %d out of range [1,365] for %q; clamping to %d",
			retentionDays, c.accountName, defaultSoftDeleteDays)

		retentionDays = defaultSoftDeleteDays
	}

	l.Debugf("azurehelper: enabling soft delete on %q (retention=%d days)", c.accountName, retentionDays)

	days := int32(retentionDays) // bounded to [1,365] above

	props := armstorage.BlobServiceProperties{
		BlobServiceProperties: &armstorage.BlobServicePropertiesProperties{
			DeleteRetentionPolicy: &armstorage.DeleteRetentionPolicy{
				Enabled: to.Ptr(true),
				Days:    &days,
			},
			ContainerDeleteRetentionPolicy: &armstorage.DeleteRetentionPolicy{
				Enabled: to.Ptr(true),
				Days:    &days,
			},
		},
	}

	if _, err := c.blobServices.SetServiceProperties(ctx, c.resourceGroup, c.accountName, props, nil); err != nil {
		return WrapError(err, fmt.Sprintf("enable soft delete on %q", c.accountName))
	}

	return nil
}

// GetKeys returns the storage account access keys, in ARM-defined order
// (typically key1, key2). Returns an error if the call fails or the
// response contains no keys.
func (c *StorageAccountClient) GetKeys(ctx context.Context) ([]string, error) {
	resp, err := c.accounts.ListKeys(ctx, c.resourceGroup, c.accountName, nil)
	if err != nil {
		return nil, WrapError(err, fmt.Sprintf("list keys for %q", c.accountName))
	}

	if len(resp.Keys) == 0 {
		return nil, tgerrors.Errorf("no access keys returned for storage account %q", c.accountName)
	}

	keys := make([]string, 0, len(resp.Keys))

	for _, k := range resp.Keys {
		if k != nil && k.Value != nil {
			keys = append(keys, *k.Value)
		}
	}

	if len(keys) == 0 {
		return nil, tgerrors.Errorf("storage account %q returned keys but all values were empty", c.accountName)
	}

	return keys, nil
}

// withDefaults fills empty fields with the package defaults and ensures
// the "created-by" tag is present. The Tags map is replaced with a copy
// before being mutated so the caller's input map is not modified.
func (in *StorageAccountConfig) withDefaults() {
	if in.AccountKind == "" {
		in.AccountKind = defaultAccountKind
	}

	if in.AccountTier == "" {
		in.AccountTier = defaultAccountTier
	}

	if in.ReplicationType == "" {
		in.ReplicationType = defaultReplicationType
	}

	if in.AccessTier == "" {
		in.AccessTier = defaultAccessTier
	}

	tags := make(map[string]string, len(in.Tags)+1)
	for k, v := range in.Tags {
		tags[k] = v
	}

	if _, ok := tags["created-by"]; !ok {
		tags["created-by"] = "terragrunt"
	}

	in.Tags = tags
}

// accessTierValue maps a string to the SDK's AccessTier enum.
func accessTierValue(s string) *armstorage.AccessTier {
	switch s {
	case "Cool":
		return to.Ptr(armstorage.AccessTierCool)
	case "Premium":
		return to.Ptr(armstorage.AccessTierPremium)
	default:
		return to.Ptr(armstorage.AccessTierHot)
	}
}

func stringMapPtr(in map[string]string) map[string]*string {
	if len(in) == 0 {
		return nil
	}

	out := make(map[string]*string, len(in))
	for k, v := range in {
		out[k] = to.Ptr(v)
	}

	return out
}

// isStatusCode reports whether err is an azcore.ResponseError with the
// supplied HTTP status.
func isStatusCode(err error, status int) bool {
	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) {
		return false
	}

	return respErr.StatusCode == status
}
