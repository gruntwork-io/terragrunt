// Storage account management.
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
	"maps"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"

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
		return nil, ErrAzureConfigRequired
	}

	if cfg.SubscriptionID == "" {
		return nil, ErrSubscriptionIDRequired
	}

	if cfg.Credential == nil {
		return nil, &UnsupportedAuthForOpError{Method: cfg.Method, Operation: "storage account operations"}
	}

	if cfg.AccountName == "" {
		return nil, ErrStorageAccountRequired
	}

	if cfg.ResourceGroup == "" {
		return nil, ErrResourceGroupNameRequired
	}

	armOpts := &arm.ClientOptions{ClientOptions: cfg.ClientOptions}

	accounts, err := armstorage.NewAccountsClient(cfg.SubscriptionID, cfg.Credential, armOpts)
	if err != nil {
		return nil, fmt.Errorf("creating armstorage accounts client: %w", err)
	}

	blobServices, err := armstorage.NewBlobServicesClient(cfg.SubscriptionID, cfg.Credential, armOpts)
	if err != nil {
		return nil, fmt.Errorf("creating armstorage blob services client: %w", err)
	}

	return &StorageAccountClient{
		accounts:       accounts,
		blobServices:   blobServices,
		subscriptionID: cfg.SubscriptionID,
		resourceGroup:  cfg.ResourceGroup,
		accountName:    cfg.AccountName,
	}, nil
}

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

	return false, fmt.Errorf("get storage account %q: %w", c.accountName, err)
}

// Create creates the storage account described by in. cfg.Name and
// cfg.ResourceGroupName, when non-empty, must equal the values bound to
// the client; mismatches return an error so callers cannot accidentally
// target a different account.
//
// The call blocks until ARM reports the create operation as complete.
// Blob versioning and soft delete are configured separately via
// EnableVersioning / EnableSoftDelete after the account exists.
func (c *StorageAccountClient) Create(ctx context.Context, l log.Logger, in *StorageAccountConfig) error {
	if in == nil {
		return ErrStorageAccountConfigRequired
	}

	if in.Name != "" && in.Name != c.accountName {
		return fmt.Errorf("storage account name %q does not match client account name %q", in.Name, c.accountName)
	}

	if in.ResourceGroupName != "" && in.ResourceGroupName != c.resourceGroup {
		return fmt.Errorf("storage account resource group %q does not match client resource group %q", in.ResourceGroupName, c.resourceGroup)
	}

	if in.Location == "" {
		return fmt.Errorf("location is required to create storage account %q", c.accountName)
	}

	// Operate on a copy so the caller's StorageAccountConfig is not mutated by
	// withDefaults (which fills empty fields and copies Tags).
	cfg := *in
	cfg.withDefaults()

	skuName := armstorage.SKUName(cfg.AccountTier + "_" + cfg.ReplicationType)
	kind := armstorage.Kind(cfg.AccountKind)

	accessTier, err := accessTierValue(cfg.AccessTier)
	if err != nil {
		return err
	}

	params := armstorage.AccountCreateParameters{
		Kind:     &kind,
		Location: to.Ptr(cfg.Location),
		SKU:      &armstorage.SKU{Name: &skuName},
		Tags:     stringMapPtr(cfg.Tags),
		Properties: &armstorage.AccountPropertiesCreateParameters{
			AccessTier:            accessTier,
			AllowBlobPublicAccess: to.Ptr(cfg.AllowBlobPublicAccess),
		},
	}

	l.Debugf("azurehelper: creating storage account %q in %q (%s, %s)", c.accountName, c.resourceGroup, kind, skuName)

	poller, err := c.accounts.BeginCreate(ctx, c.resourceGroup, c.accountName, params, nil)
	if err != nil {
		return fmt.Errorf("begin create storage account %q: %w", c.accountName, err)
	}

	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return fmt.Errorf("create storage account %q: %w", c.accountName, err)
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

		return fmt.Errorf("delete storage account %q: %w", c.accountName, err)
	}

	return nil
}

// updateBlobServiceProperties applies mutate to the account's CURRENT blob
// service properties and writes them back. SetServiceProperties is a
// full-replace PUT, so a blind write of a partial property set would silently
// reset every unspecified sub-policy (e.g. enabling versioning would clear an
// existing soft-delete policy). Reading first and mutating in place preserves
// settings that this call is not changing.
func (c *StorageAccountClient) updateBlobServiceProperties(
	ctx context.Context,
	mutate func(*armstorage.BlobServicePropertiesProperties),
) error {
	resp, err := c.blobServices.GetServiceProperties(ctx, c.resourceGroup, c.accountName, nil)
	if err != nil {
		return fmt.Errorf("get blob service properties for %q: %w", c.accountName, err)
	}

	inner := resp.BlobServiceProperties.BlobServiceProperties
	if inner == nil {
		inner = &armstorage.BlobServicePropertiesProperties{}
	}

	mutate(inner)

	// Send only the writable inner properties; the GET response also carries
	// read-only ID/Name/SKU/Type fields that do not belong in a PUT body.
	update := armstorage.BlobServiceProperties{BlobServiceProperties: inner}

	if _, err := c.blobServices.SetServiceProperties(ctx, c.resourceGroup, c.accountName, update, nil); err != nil {
		return fmt.Errorf("set blob service properties for %q: %w", c.accountName, err)
	}

	return nil
}

// EnableVersioning turns on blob versioning for the account's default
// blob service. Calling on an account that already has versioning
// enabled is a no-op from the caller's perspective. Existing soft-delete
// policies are preserved (read-modify-write).
func (c *StorageAccountClient) EnableVersioning(ctx context.Context, l log.Logger) error {
	l.Debugf("azurehelper: enabling blob versioning on %q", c.accountName)

	return c.updateBlobServiceProperties(ctx, func(p *armstorage.BlobServicePropertiesProperties) {
		p.IsVersioningEnabled = to.Ptr(true)
	})
}

// IsVersioningEnabled returns true if blob versioning is on.
func (c *StorageAccountClient) IsVersioningEnabled(ctx context.Context) (bool, error) {
	resp, err := c.blobServices.GetServiceProperties(ctx, c.resourceGroup, c.accountName, nil)
	if err != nil {
		return false, fmt.Errorf("get blob service properties for %q: %w", c.accountName, err)
	}

	if resp.BlobServiceProperties.BlobServiceProperties == nil {
		return false, nil
	}

	v := resp.BlobServiceProperties.BlobServiceProperties.IsVersioningEnabled

	return v != nil && *v, nil
}

// IsSoftDeleteEnabled returns true only if BOTH soft-delete policies that
// EnableSoftDelete configures are enabled: the blob DeleteRetentionPolicy and
// the ContainerDeleteRetentionPolicy. Returning true when only one is enabled
// would let NeedsBootstrap skip a partially-configured account.
func (c *StorageAccountClient) IsSoftDeleteEnabled(ctx context.Context) (bool, error) {
	resp, err := c.blobServices.GetServiceProperties(ctx, c.resourceGroup, c.accountName, nil)
	if err != nil {
		return false, fmt.Errorf("get blob service properties for %q: %w", c.accountName, err)
	}

	props := resp.BlobServiceProperties.BlobServiceProperties
	if props == nil {
		return false, nil
	}

	return retentionPolicyEnabled(props.DeleteRetentionPolicy) &&
		retentionPolicyEnabled(props.ContainerDeleteRetentionPolicy), nil
}

// retentionPolicyEnabled reports whether a delete-retention policy is non-nil
// and enabled.
func retentionPolicyEnabled(p *armstorage.DeleteRetentionPolicy) bool {
	return p != nil && p.Enabled != nil && *p.Enabled
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

	// Read-modify-write so we do not clobber a previously-set versioning flag.
	return c.updateBlobServiceProperties(ctx, func(p *armstorage.BlobServicePropertiesProperties) {
		p.DeleteRetentionPolicy = &armstorage.DeleteRetentionPolicy{
			Enabled: to.Ptr(true),
			Days:    &days,
		}
		p.ContainerDeleteRetentionPolicy = &armstorage.DeleteRetentionPolicy{
			Enabled: to.Ptr(true),
			Days:    &days,
		}
	})
}

// GetKeys returns the storage account access keys, in ARM-defined order
// (typically key1, key2). Returns an error if the call fails or the
// response contains no keys.
func (c *StorageAccountClient) GetKeys(ctx context.Context) ([]string, error) {
	resp, err := c.accounts.ListKeys(ctx, c.resourceGroup, c.accountName, nil)
	if err != nil {
		return nil, fmt.Errorf("list keys for %q: %w", c.accountName, err)
	}

	if len(resp.Keys) == 0 {
		return nil, fmt.Errorf("%w %q", ErrNoAccessKeysReturned, c.accountName)
	}

	keys := make([]string, 0, len(resp.Keys))

	for _, k := range resp.Keys {
		if k != nil && k.Value != nil && *k.Value != "" {
			keys = append(keys, *k.Value)
		}
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("%w (account %q)", ErrAllAccessKeysEmpty, c.accountName)
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

	tags := maps.Clone(in.Tags)
	if tags == nil {
		tags = make(map[string]string, 1)
	}

	if _, ok := tags["created-by"]; !ok {
		tags["created-by"] = "terragrunt"
	}

	in.Tags = tags
}

// accessTierValue maps a string to the SDK's AccessTier enum. An empty
// input yields the package default (Hot); any other unrecognised value
// returns an error so callers cannot silently land on an unintended tier.
func accessTierValue(s string) (*armstorage.AccessTier, error) {
	switch s {
	case "", "Hot":
		return to.Ptr(armstorage.AccessTierHot), nil
	case "Cool":
		return to.Ptr(armstorage.AccessTierCool), nil
	case "Cold":
		return to.Ptr(armstorage.AccessTierCold), nil
	case "Premium":
		return to.Ptr(armstorage.AccessTierPremium), nil
	default:
		return nil, &UnknownAccessTierError{Tier: s}
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
	respErr, ok := errors.AsType[*azcore.ResponseError](err)
	if !ok {
		return false
	}

	return respErr.StatusCode == status
}
