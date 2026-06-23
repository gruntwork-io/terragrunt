// Package azurerm -- client wrapper.
//
// Client composes the azurehelper data-plane (BlobClient) and control-plane
// (StorageAccountClient, ResourceGroupClient, RBACClient) wrappers into the
// surface needed by the backend lifecycle methods (NeedsBootstrap, Bootstrap,
// Migrate, Delete, DeleteBucket).
package azurerm

import (
	"context"
	"errors"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Client wraps azurehelper clients bound to a single storage account /
// container pair. The data-plane BlobClient is always built up-front.
// Control-plane helpers (storage account, resource group, RBAC) are
// constructed lazily on first use: a config that opts out of every
// control-plane operation via skip_*_creation flags never causes the
// helpers to be built, so a data-plane-only credential (SAS / access
// key, or a token credential with no resource_group_name) is sufficient.
type Client struct {
	*ExtendedRemoteStateConfigAzureRM

	azCfg *azurehelper.AzureConfig
	blob  *azurehelper.BlobClient

	storageAccount    *azurehelper.StorageAccountClient
	storageAccountErr error

	resourceGroup    *azurehelper.ResourceGroupClient
	resourceGroupErr error

	rbac    *azurehelper.RBACClient
	rbacErr error

	storageAccountOK bool
	resourceGroupOK  bool
	rbacOK           bool
}

// NewClient constructs a Client. Only the data-plane (blob) client is
// built up front; control-plane clients are deferred to first use so a
// pure data-plane configuration (skip_* flags set) does not require ARM
// reachability.
func NewClient(
	ctx context.Context,
	l log.Logger,
	cfg *ExtendedRemoteStateConfigAzureRM,
	opts *backend.Options,
) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("azurerm: ExtendedRemoteStateConfigAzureRM is required")
	}

	if opts == nil {
		return nil, errors.New("azurerm: backend.Options is required")
	}

	azCfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(cfg.GetAzureSessionConfig()).
		WithEnv(opts.Env).
		Build(ctx, l)
	if err != nil {
		return nil, err
	}

	blob, err := azurehelper.NewBlobClient(azCfg)
	if err != nil {
		return nil, err
	}

	return &Client{
		ExtendedRemoteStateConfigAzureRM: cfg,
		azCfg:                            azCfg,
		blob:                             blob,
	}, nil
}

// Close is provided for symmetry with other backend clients. azurehelper
// holds no resources that require explicit release; the method is a no-op
// but exists so callers can `defer client.Close()` without thinking about
// the underlying SDK.
func (c *Client) Close() error { return nil }

// storageAccountClient lazily constructs the ARM storage-account helper
// and caches it (or the construction error) for subsequent calls.
func (c *Client) storageAccountClient(op string) (*azurehelper.StorageAccountClient, error) {
	if c.storageAccountOK {
		return c.storageAccount, c.storageAccountErr
	}

	if c.azCfg.Credential == nil {
		c.storageAccountErr = &ControlPlaneUnavailableError{Method: c.azCfg.Method, Operation: op}
	} else {
		c.storageAccount, c.storageAccountErr = azurehelper.NewStorageAccountClient(c.azCfg)
	}

	c.storageAccountOK = true

	return c.storageAccount, c.storageAccountErr
}

// resourceGroupClient lazily constructs the ARM resource-group helper.
func (c *Client) resourceGroupClient(op string) (*azurehelper.ResourceGroupClient, error) {
	if c.resourceGroupOK {
		return c.resourceGroup, c.resourceGroupErr
	}

	if c.azCfg.Credential == nil {
		c.resourceGroupErr = &ControlPlaneUnavailableError{Method: c.azCfg.Method, Operation: op}
	} else {
		c.resourceGroup, c.resourceGroupErr = azurehelper.NewResourceGroupClient(c.azCfg)
	}

	c.resourceGroupOK = true

	return c.resourceGroup, c.resourceGroupErr
}

// rbacClient lazily constructs the ARM RBAC helper.
func (c *Client) rbacClient(op string) (*azurehelper.RBACClient, error) {
	if c.rbacOK {
		return c.rbac, c.rbacErr
	}

	if c.azCfg.Credential == nil {
		c.rbacErr = &ControlPlaneUnavailableError{Method: c.azCfg.Method, Operation: op}
	} else {
		c.rbac, c.rbacErr = azurehelper.NewRBACClient(c.azCfg)
	}

	c.rbacOK = true

	return c.rbac, c.rbacErr
}

// DoesStorageAccountExist reports whether the configured storage account
// exists in the configured resource group.
func (c *Client) DoesStorageAccountExist(ctx context.Context) (bool, error) {
	sa, err := c.storageAccountClient("checking storage account existence")
	if err != nil {
		return false, err
	}

	return sa.Exists(ctx)
}

// EnsureStorageAccount ensures the configured storage account exists.
// No-op when skip_storage_account_creation is set or the account already
// exists. When creation is required: returns backend.BucketCreationNotAllowed
// if opts.FailIfBucketCreationRequired is set; otherwise prompts the operator
// (honouring opts.NonInteractive), then ensures the resource group exists
// (unless skip_resource_group_creation is set), and finally creates the
// account.
func (c *Client) EnsureStorageAccount(ctx context.Context, l log.Logger, opts *backend.Options) error {
	if c.SkipStorageAccountCreation {
		return nil
	}

	sa, err := c.storageAccountClient("creating storage account")
	if err != nil {
		return err
	}

	exists, err := sa.Exists(ctx)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	if c.Location == "" {
		return MissingRequiredAzureRMRemoteStateConfig("location")
	}

	if opts.FailIfBucketCreationRequired {
		return backend.BucketCreationNotAllowed(c.RemoteStateConfigAzureRM.StorageAccountName)
	}

	prompt := fmt.Sprintf(
		"Remote state Azure storage account %q does not exist or you don't have permissions to access it. Would you like Terragrunt to create it?",
		c.RemoteStateConfigAzureRM.StorageAccountName,
	)

	yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts.NonInteractive, opts.Writers.ErrWriter)
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	if err := c.ensureResourceGroup(ctx, l); err != nil {
		return err
	}

	createCfg := &azurehelper.StorageAccountConfig{
		Name:              c.RemoteStateConfigAzureRM.StorageAccountName,
		ResourceGroupName: c.RemoteStateConfigAzureRM.ResourceGroupName,
		Location:          c.Location,
		AccountKind:       c.AccountKind,
		AccountTier:       c.AccountTier,
		ReplicationType:   c.AccountReplicationType,
		AccessTier:        c.AccessTier,
		Tags:              c.Tags,
	}

	if err := sa.Create(ctx, l, createCfg); err != nil {
		return err
	}

	if !c.SkipVersioning {
		if err := sa.EnableVersioning(ctx, l); err != nil {
			return err
		}
	}

	if c.EnableSoftDelete {
		if err := sa.EnableSoftDelete(ctx, l, c.SoftDeleteRetentionDays); err != nil {
			return err
		}
	}

	return nil
}

// ensureResourceGroup creates the configured resource group if it does
// not already exist. No-op when skip_resource_group_creation is true.
func (c *Client) ensureResourceGroup(ctx context.Context, l log.Logger) error {
	if c.SkipResourceGroupCreation {
		return nil
	}

	if c.RemoteStateConfigAzureRM.ResourceGroupName == "" {
		return MissingRequiredAzureRMRemoteStateConfig("resource_group_name")
	}

	rg, err := c.resourceGroupClient("creating resource group")
	if err != nil {
		return err
	}

	return rg.EnsureResourceGroup(ctx, l, c.RemoteStateConfigAzureRM.ResourceGroupName, c.Location)
}

// IsVersioningEnabled returns true if blob versioning is enabled on the
// underlying storage account. Returns (false, nil) with a debug log when
// the auth method is data-plane only (SAS / access key): the framework's
// IsVersionControlEnabled caller distinguishes only between "off" and
// "doesn't exist", so degrading silently is preferable to crashing the
// migrate path on otherwise-valid configurations.
func (c *Client) IsVersioningEnabled(ctx context.Context, l log.Logger) (bool, error) {
	sa, err := c.storageAccountClient("checking versioning")
	if err != nil {
		var unavailable *ControlPlaneUnavailableError
		if errors.As(err, &unavailable) {
			l.Debugf("azurerm: cannot check versioning with auth method %q; assuming off", c.azCfg.Method)
			return false, nil
		}

		return false, err
	}

	enabled, err := sa.IsVersioningEnabled(ctx)
	if err != nil {
		return false, err
	}

	if !enabled {
		l.Warnf(
			"Versioning is not enabled for the remote state Azure storage account %s. We recommend enabling versioning so that you can roll back to previous versions of your state in case of error.",
			c.RemoteStateConfigAzureRM.StorageAccountName,
		)
	}

	return enabled, nil
}

// DoesContainerExist reports whether the configured container exists.
func (c *Client) DoesContainerExist(ctx context.Context) (bool, error) {
	return c.blob.ContainerExists(ctx, c.RemoteStateConfigAzureRM.ContainerName)
}

// EnsureContainer ensures the configured container exists. The
// method is a no-op when skip_container_creation is set.
func (c *Client) EnsureContainer(ctx context.Context, l log.Logger, opts *backend.Options) error {
	if c.SkipContainerCreation {
		return nil
	}

	exists, err := c.DoesContainerExist(ctx)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	if opts.FailIfBucketCreationRequired {
		return backend.BucketCreationNotAllowed(c.RemoteStateConfigAzureRM.ContainerName)
	}

	prompt := fmt.Sprintf(
		"Remote state Azure container %q in storage account %q does not exist. Would you like Terragrunt to create it?",
		c.RemoteStateConfigAzureRM.ContainerName, c.RemoteStateConfigAzureRM.StorageAccountName,
	)

	yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts.NonInteractive, opts.Writers.ErrWriter)
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	return c.blob.EnsureContainer(ctx, c.RemoteStateConfigAzureRM.ContainerName)
}

// MoveBlob copies srcContainer/srcKey to dstContainer/dstKey and then
// deletes the source blob. CopyBlob uses the synchronous Put Blob From URL
// API and returns only after the copy is committed, so the source can be
// deleted immediately.
//
// The delete step alone is idempotent (a missing source blob is treated
// as success). The whole operation is not idempotent: a second call after
// success fails inside CopyBlob with source-not-found.
func (c *Client) MoveBlob(ctx context.Context, srcContainer, srcKey, dstContainer, dstKey string) error {
	if srcContainer == dstContainer && srcKey == dstKey {
		return nil
	}

	if err := c.blob.CopyBlob(ctx, srcContainer, srcKey, dstContainer, dstKey); err != nil {
		return err
	}

	return c.blob.EnsureBlobDeleted(ctx, srcContainer, srcKey)
}

// EnsureBlobDeleted deletes a single blob; idempotent on BlobNotFound.
func (c *Client) EnsureBlobDeleted(ctx context.Context, container, key string) error {
	return c.blob.EnsureBlobDeleted(ctx, container, key)
}

// EnsureContainerDeleted deletes the configured container. Idempotent:
// returns nil if the container is already gone.
func (c *Client) EnsureContainerDeleted(ctx context.Context) error {
	return c.blob.EnsureContainerDeleted(ctx, c.RemoteStateConfigAzureRM.ContainerName)
}

// DeleteStorageAccount deletes the configured storage account. Returns an
// error if the auth method cannot reach the ARM control plane.
func (c *Client) DeleteStorageAccount(ctx context.Context, l log.Logger) error {
	sa, err := c.storageAccountClient("deleting storage account")
	if err != nil {
		return err
	}

	return sa.Delete(ctx, l)
}

// EnsureBlobDataOwner assigns the Storage Blob Data Owner role
// on the configured storage account to the supplied principal. No-op when
// assign_storage_blob_data_owner is false or principalID is empty.
func (c *Client) EnsureBlobDataOwner(ctx context.Context, l log.Logger, principalID string) error {
	if !c.AssignBlobDataOwner || principalID == "" {
		return nil
	}

	rbac, err := c.rbacClient("assigning Storage Blob Data Owner")
	if err != nil {
		return err
	}

	scope := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s",
		c.azCfg.SubscriptionID, c.RemoteStateConfigAzureRM.ResourceGroupName, c.RemoteStateConfigAzureRM.StorageAccountName,
	)

	return rbac.AssignRoleIfMissing(ctx, l, azurehelper.AssignRoleInput{
		Scope:            scope,
		PrincipalID:      principalID,
		RoleDefinitionID: azurehelper.RoleStorageBlobDataOwner,
	})
}
