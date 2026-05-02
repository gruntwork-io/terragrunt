// Package azurerm -- client wrapper.
//
// Client composes the azurehelper data-plane (BlobClient) and control-plane
// (StorageAccountClient, ResourceGroupClient, RBACClient) wrappers into the
// surface needed by the backend lifecycle methods (NeedsBootstrap, Bootstrap,
// Migrate, Delete, DeleteBucket).
package azurerm

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Client wraps azurehelper clients bound to a single storage account /
// container pair. Control-plane clients (storage account, resource group,
// RBAC) are nil when the resolved auth method (SAS token, access key) does
// not support ARM operations; callers must guard accordingly.
type Client struct {
	*ExtendedRemoteStateConfigAzureRM

	azCfg          *azurehelper.AzureConfig
	blob           *azurehelper.BlobClient
	storageAccount *azurehelper.StorageAccountClient
	resourceGroup  *azurehelper.ResourceGroupClient
	rbac           *azurehelper.RBACClient
}

// NewClient constructs a Client. The data-plane (blob) client is always
// created; control-plane clients are constructed only when the resolved
// AzureConfig carries a token credential.
func NewClient(
	ctx context.Context,
	l log.Logger,
	cfg *ExtendedRemoteStateConfigAzureRM,
	opts *backend.Options,
) (*Client, error) {
	azCfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(cfg.GetAzureSessionConfig()).
		WithEnv(opts.Env).
		Build(ctx, l)
	if err != nil {
		return nil, err
	}

	blob, err := azurehelper.NewBlobClient(ctx, azCfg, "")
	if err != nil {
		return nil, err
	}

	out := &Client{
		ExtendedRemoteStateConfigAzureRM: cfg,
		azCfg:                            azCfg,
		blob:                             blob.BindContainer(cfg.RemoteStateConfigAzureRM.ContainerName),
	}

	// Control-plane clients only work with token credentials. SAS-token and
	// access-key configs are data-plane only.
	if azCfg.Credential != nil {
		sa, err := azurehelper.NewStorageAccountClient(azCfg)
		if err != nil {
			return nil, err
		}

		out.storageAccount = sa

		rg, err := azurehelper.NewResourceGroupClient(azCfg)
		if err != nil {
			return nil, err
		}

		out.resourceGroup = rg

		rbac, err := azurehelper.NewRBACClient(azCfg)
		if err != nil {
			return nil, err
		}

		out.rbac = rbac
	}

	return out, nil
}

// Close is provided for symmetry with other backend clients. azurehelper
// holds no resources that require explicit release; this is currently a
// no-op but exists so callers can `defer client.Close()` without thinking
// about the underlying SDK.
func (c *Client) Close() error { return nil }

// requireControlPlane returns an error if the resolved credentials cannot
// reach the ARM control plane (SAS-token / access-key). Used by methods
// that manage storage accounts, resource groups, or RBAC.
func (c *Client) requireControlPlane(op string) error {
	if c.storageAccount == nil {
		return errors.Errorf("%s requires a token credential; auth method %q is data-plane only", op, c.azCfg.Method)
	}

	return nil
}

// DoesStorageAccountExist reports whether the configured storage account
// exists in the configured resource group.
func (c *Client) DoesStorageAccountExist(ctx context.Context) (bool, error) {
	if err := c.requireControlPlane("checking storage account existence"); err != nil {
		return false, err
	}

	return c.storageAccount.Exists(ctx)
}

// CreateStorageAccountIfNecessary ensures the configured storage account
// exists. It first ensures the resource group exists (unless the caller
// has set skip_resource_group_creation), then creates the account if
// missing. The method is a no-op when skip_storage_account_creation is set.
//
// FailIfBucketCreationRequired short-circuits with backend.BucketCreationNotAllowed
// when creation would otherwise be needed; opts.NonInteractive is honoured
// when prompting the user.
func (c *Client) CreateStorageAccountIfNecessary(ctx context.Context, l log.Logger, opts *backend.Options) error {
	if c.SkipStorageAccountCreation {
		return nil
	}

	if err := c.requireControlPlane("creating storage account"); err != nil {
		return err
	}

	exists, err := c.storageAccount.Exists(ctx)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	if c.Location == "" {
		return errors.New(MissingRequiredAzureRMRemoteStateConfig("location"))
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

	if err := c.storageAccount.Create(ctx, l, createCfg); err != nil {
		return err
	}

	if !c.SkipVersioning {
		if err := c.storageAccount.EnableVersioning(ctx, l); err != nil {
			return err
		}
	}

	if c.EnableSoftDelete {
		if err := c.storageAccount.EnableSoftDelete(ctx, l, c.SoftDeleteRetentionDays); err != nil {
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
		return errors.New(MissingRequiredAzureRMRemoteStateConfig("resource_group_name"))
	}

	return c.resourceGroup.CreateIfNecessary(ctx, l, c.RemoteStateConfigAzureRM.ResourceGroupName, c.Location)
}

// IsVersioningEnabled returns true if blob versioning is enabled on the
// underlying storage account. Returns an error if the auth method cannot
// reach the ARM control plane.
func (c *Client) IsVersioningEnabled(ctx context.Context, l log.Logger) (bool, error) {
	if err := c.requireControlPlane("checking versioning"); err != nil {
		return false, err
	}

	enabled, err := c.storageAccount.IsVersioningEnabled(ctx)
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

// CreateContainerIfNecessary ensures the configured container exists. The
// method is a no-op when skip_container_creation is set.
func (c *Client) CreateContainerIfNecessary(ctx context.Context, l log.Logger, opts *backend.Options) error {
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

	return c.blob.CreateContainerIfNecessary(ctx, c.RemoteStateConfigAzureRM.ContainerName)
}

// MoveBlob copies srcKey within the bound container to dstContainer/dstKey
// and then deletes the source blob. CopyBlob is server-side and may be
// asynchronous for very large blobs; the source delete may briefly race in
// that case. State files are small in practice so this is acceptable for
// the migration use case.
func (c *Client) MoveBlob(ctx context.Context, srcContainer, srcKey, dstContainer, dstKey string) error {
	if srcContainer == dstContainer && srcKey == dstKey {
		return nil
	}

	if err := c.blob.CopyBlob(ctx, srcContainer, srcKey, dstContainer, dstKey); err != nil {
		return err
	}

	return c.blob.DeleteBlob(ctx, srcContainer, srcKey)
}

// DeleteBlobIfExists deletes a single blob. Missing blobs return nil.
func (c *Client) DeleteBlobIfExists(ctx context.Context, container, key string) error {
	return c.blob.DeleteBlob(ctx, container, key)
}

// DeleteContainer deletes the configured container. Missing containers
// return nil.
func (c *Client) DeleteContainer(ctx context.Context) error {
	return c.blob.DeleteContainer(ctx, c.RemoteStateConfigAzureRM.ContainerName)
}

// DeleteStorageAccount deletes the configured storage account. Returns an
// error if the auth method cannot reach the ARM control plane.
func (c *Client) DeleteStorageAccount(ctx context.Context, l log.Logger) error {
	if err := c.requireControlPlane("deleting storage account"); err != nil {
		return err
	}

	return c.storageAccount.Delete(ctx, l)
}

// AssignBlobDataOwnerIfNecessary assigns the Storage Blob Data Owner role
// on the configured storage account to the supplied principal. No-op when
// assign_storage_blob_data_owner is false or principalID is empty.
func (c *Client) AssignBlobDataOwnerIfNecessary(ctx context.Context, l log.Logger, principalID string) error {
	if !c.AssignBlobDataOwner || principalID == "" {
		return nil
	}

	if err := c.requireControlPlane("assigning Storage Blob Data Owner"); err != nil {
		return err
	}

	scope := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s",
		c.azCfg.SubscriptionID, c.RemoteStateConfigAzureRM.ResourceGroupName, c.RemoteStateConfigAzureRM.StorageAccountName,
	)

	return c.rbac.AssignRoleIfMissing(ctx, l, azurehelper.AssignRoleInput{
		Scope:            scope,
		PrincipalID:      principalID,
		RoleDefinitionID: azurehelper.RoleStorageBlobDataOwner,
	})
}
