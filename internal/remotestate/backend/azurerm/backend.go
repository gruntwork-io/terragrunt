// Package azurerm implements the Azure Storage (azurerm) backend for
// interacting with remote state. It bootstraps the resource group, storage
// account, and blob container backing a unit's Terraform/OpenTofu state, and
// supports delete and migrate lifecycle operations via internal/azurehelper.
//
// The backend is experimental: every lifecycle operation is gated behind the
// `azure-backend` experiment and returns ErrAzureBackendExperimentRequired
// when it is not enabled.
package azurerm

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	BackendName = "azurerm"

	// defaultSoftDeleteRetentionDays is applied when enable_soft_delete is set
	// but soft_delete_retention_days is left unset (0).
	defaultSoftDeleteRetentionDays = 7
)

var _ backend.Backend = new(Backend)

// Backend implements the azurerm remote-state backend.
type Backend struct {
	*backend.CommonBackend
}

// NewBackend returns a new azurerm backend.
func NewBackend() *Backend {
	return &Backend{
		CommonBackend: backend.NewCommonBackend(BackendName),
	}
}

// experimentEnabled reports whether the azure-backend experiment is on. Every
// lifecycle entry point is called with options built by the remote-state layer,
// so a nil opts is a caller bug and panics with ErrBackendOptionsRequired.
//
// Callers treat a disabled experiment as "do nothing" rather than as an error.
// Before this backend existed, an azurerm config inherited CommonBackend's
// no-op behavior, so a globally applied --backend-bootstrap simply continued
// into native backend init. Gating must stop the experimental implementation
// from running without breaking that previously working path.
func experimentEnabled(opts *backend.Options) bool {
	if opts == nil {
		panic(ErrBackendOptionsRequired)
	}

	return opts.Experiments.Evaluate(experiment.AzureBackend)
}

// resolveConfig parses, validates, and resolves the azure session config for
// the given raw backend config, returning the parsed config and the resolved
// azurehelper.AzureConfig (credentials + cloud).
func resolveConfig(
	l log.Logger,
	v *venv.Venv,
	backendConfig backend.Config,
) (*ExtendedRemoteStateConfigAzurerm, *azurehelper.AzureConfig, error) {
	extCfg, err := Config(backendConfig).ExtendedAzurermConfig()
	if err != nil {
		return nil, nil, err
	}

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(extCfg.GetAzureSessionConfig()).
		Build(l, v)
	if err != nil {
		return nil, nil, err
	}

	return extCfg, cfg, nil
}

// armCapable reports whether the resolved auth method can reach the ARM control
// plane (resource group / storage account management). SAS-token and access-key
// auth are data-plane only, so those callers must pre-create the account.
func armCapable(cfg *azurehelper.AzureConfig) bool {
	return cfg.Method != azurehelper.AuthMethodSasToken && cfg.Method != azurehelper.AuthMethodAccessKey
}

// armWorkRequested reports whether the config asks for any ARM control-plane
// work; a user-managed account with no policy convergence requires none.
func armWorkRequested(extCfg *ExtendedRemoteStateConfigAzurerm) bool {
	return !extCfg.SkipStorageAccountCreation || !extCfg.SkipVersioning || extCfg.EnableSoftDelete
}

// warnArmWorkSkipped logs that account creation or versioning/soft-delete
// convergence was requested but cannot run under data-plane-only auth.
func warnArmWorkSkipped(l log.Logger, name string, method azurehelper.AuthMethod) {
	l.Warnf(
		"Cannot manage the storage account for %s backend with %s authentication; skipping account creation and versioning/soft-delete convergence.",
		name,
		method,
	)
}

// cloudName describes a cloud for an error message. The configured value is
// preferred, falling back to the resolved AAD authority host so a cloud that
// came from ARM_ENVIRONMENT is still named instead of rendering empty.
func cloudName(configured, authorityHost string) string {
	if configured != "" {
		return configured
	}

	return authorityHost
}

// newBlobClient builds the data-plane client used by Azure lifecycle operations.
// It prefers the native backend's shared-key authorization when possible, so an
// identity that may list account keys but lacks a blob data-plane role still
// works without requiring use_azuread_auth.
//
// The key lookup is best effort: when it fails (for example the identity may
// read blobs but not list keys) the token credential is used instead, so this
// only ever adds a way to authorize, never removes one.
func newBlobClient(ctx context.Context, l log.Logger, cfg *azurehelper.AzureConfig) (*azurehelper.BlobClient, error) {
	if !armCapable(cfg) || cfg.UseAzureADAuth || cfg.ResourceGroup == "" || cfg.SubscriptionID == "" {
		return azurehelper.NewBlobClient(cfg)
	}

	keyed, err := sharedKeyConfig(ctx, cfg)
	if err != nil {
		l.Debugf("%s: storage account key lookup failed, using token authorization for blob access: %v", BackendName, err)

		return azurehelper.NewBlobClient(cfg)
	}

	l.Debugf("%s: using shared-key blob authorization (set use_azuread_auth to authorize with Microsoft Entra instead)", BackendName)

	return azurehelper.NewBlobClient(keyed)
}

// NewStateBlobClient builds the data-plane client used for a direct state read.
// Unless use_azuread_auth is enabled, it strictly follows the native azurerm
// backend by resolving a storage-account key through ARM; lookup errors are not
// replaced with bearer authorization. It panics with
// [azurehelper.ErrAzureConfigRequired] when cfg is nil.
func NewStateBlobClient(
	ctx context.Context,
	l log.Logger,
	cfg *azurehelper.AzureConfig,
) (*azurehelper.BlobClient, error) {
	if cfg == nil {
		panic(azurehelper.ErrAzureConfigRequired)
	}

	if !armCapable(cfg) || cfg.UseAzureADAuth {
		return azurehelper.NewBlobClient(cfg)
	}

	keyed, err := sharedKeyConfig(ctx, cfg)
	if err != nil {
		// The ARM key lookup answers 404 for a wrong resource group, account, or subscription.
		return nil, fmt.Errorf("%w: %w", ErrStateClientSetup, err)
	}

	l.Debugf("%s: using shared-key authorization for direct state access", BackendName)

	return azurehelper.NewBlobClient(keyed)
}

// OpenStateBlob opens the configured state blob using the native azurerm
// backend's authorization policy. The caller owns the returned reader and must
// close it.
func OpenStateBlob(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	backendConfig backend.Config,
) (io.ReadCloser, error) {
	extCfg, cfg, err := resolveConfig(l, v, backendConfig)
	if err != nil {
		return nil, err
	}

	state := &extCfg.RemoteStateConfigAzurerm

	blobClient, err := NewStateBlobClient(ctx, l, cfg)
	if err != nil {
		return nil, err
	}

	body, err := blobClient.Container(state.ContainerName).GetBlob(ctx, state.Key)
	if err != nil {
		return nil, fmt.Errorf(
			"opening azurerm state blob %s/%s in storage account %s: %w",
			state.ContainerName,
			state.Key,
			state.StorageAccountName,
			err,
		)
	}

	return body, nil
}

// sharedKeyConfig returns a copy of cfg switched to access-key authentication
// using the first non-empty account key.
func sharedKeyConfig(ctx context.Context, cfg *azurehelper.AzureConfig) (*azurehelper.AzureConfig, error) {
	saClient, err := azurehelper.NewStorageAccountClient(cfg)
	if err != nil {
		return nil, err
	}

	keys, err := saClient.GetKeys(ctx)
	if err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return nil, azurehelper.ErrNoAccessKeysReturned
	}

	keyed := *cfg
	keyed.Method = azurehelper.AuthMethodAccessKey
	keyed.AccessKey = keys[0]

	return &keyed, nil
}

// NeedsBootstrap returns true if the storage account or container backing the
// state does not yet exist, or (when reachable) blob versioning or soft-delete
// configuration has drifted from what the config requests.
func (b *Backend) NeedsBootstrap(ctx context.Context, l log.Logger, v *venv.Venv, backendConfig backend.Config, opts *backend.Options) (bool, error) {
	if !experimentEnabled(opts) {
		return false, nil
	}

	extCfg, cfg, err := resolveConfig(l, v, backendConfig)
	if err != nil {
		return false, err
	}

	rs := &extCfg.RemoteStateConfigAzurerm

	if armWorkRequested(extCfg) && !armCapable(cfg) {
		warnArmWorkSkipped(l, b.Name(), cfg.Method)
	}

	if armCapable(cfg) && armWorkRequested(extCfg) {
		needs, err := accountNeedsBootstrap(ctx, cfg, extCfg)
		if err != nil {
			return false, err
		}

		if needs {
			return true, nil
		}
	}

	if !extCfg.SkipContainerCreation {
		blobClient, err := newBlobClient(ctx, l, cfg)
		if err != nil {
			return false, err
		}

		exists, err := blobClient.Container(rs.ContainerName).Exists(ctx)
		if err != nil {
			return false, err
		}

		if !exists {
			return true, nil
		}
	}

	return false, nil
}

// accountNeedsBootstrap reports whether the storage account is missing (and
// creatable) or has versioning / soft-delete drift to converge.
func accountNeedsBootstrap(
	ctx context.Context,
	cfg *azurehelper.AzureConfig,
	extCfg *ExtendedRemoteStateConfigAzurerm,
) (bool, error) {
	saClient, err := azurehelper.NewStorageAccountClient(cfg)
	if err != nil {
		return false, err
	}

	exists, err := saClient.Exists(ctx)
	if err != nil {
		return false, err
	}

	// A missing account needs bootstrap only when we are allowed to create
	// it; under skip_storage_account_creation the user manages the account.
	if !exists {
		return !extCfg.SkipStorageAccountCreation, nil
	}

	// An existing account is checked for versioning / soft-delete drift even
	// under skip_storage_account_creation, since those policies are converged
	// on pre-created accounts too.
	return accountPolicyDrift(ctx, saClient, extCfg)
}

// Bootstrap creates (if necessary) the resource group, storage account, and
// blob container backing the state, and ensures blob versioning / soft delete.
func (b *Backend) Bootstrap(ctx context.Context, l log.Logger, v *venv.Venv, backendConfig backend.Config, opts *backend.Options) error {
	if !experimentEnabled(opts) {
		return nil
	}

	extCfg, cfg, err := resolveConfig(l, v, backendConfig)
	if err != nil {
		return err
	}

	rs := &extCfg.RemoteStateConfigAzurerm

	// Only one goroutine bootstraps a given account/container at a time.
	mu := b.GetBucketMutex(rs.CacheKey())

	mu.Lock()
	defer mu.Unlock()

	if b.IsConfigInited(extCfg) {
		l.Debugf("%s container %s has already been confirmed to be initialized, skipping initialization checks", b.Name(), rs.CacheKey())

		return nil
	}

	if armWorkRequested(extCfg) && !armCapable(cfg) {
		warnArmWorkSkipped(l, b.Name(), cfg.Method)
	}

	if armCapable(cfg) {
		if err := b.bootstrapAccount(ctx, l, extCfg, cfg, opts); err != nil {
			return err
		}
	}

	if err := ensureContainer(ctx, l, cfg, extCfg, opts); err != nil {
		return err
	}

	b.MarkConfigInited(extCfg)

	return nil
}

// ensureContainer creates the state container when it is missing and creation
// is neither skipped nor forbidden.
func ensureContainer(ctx context.Context, l log.Logger, cfg *azurehelper.AzureConfig, extCfg *ExtendedRemoteStateConfigAzurerm, opts *backend.Options) error {
	if extCfg.SkipContainerCreation {
		return nil
	}

	rs := &extCfg.RemoteStateConfigAzurerm

	blobClient, err := newBlobClient(ctx, l, cfg)
	if err != nil {
		return err
	}

	exists, err := blobClient.Container(rs.ContainerName).Exists(ctx)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	// The blob container is the true analog of a GCS/S3 bucket and the
	// only creation step reachable by data-plane-only (SAS/access-key)
	// auth, so the fail-if-creation-required gate must live here too.
	if opts.FailIfBucketCreationRequired {
		return backend.BucketCreationNotAllowed(rs.ContainerName)
	}

	return blobClient.Container(rs.ContainerName).Create(ctx)
}

// bootstrapAccount ensures the resource group and storage account exist and are
// configured with versioning / soft delete. Only called for ARM-capable auth.
func (b *Backend) bootstrapAccount(
	ctx context.Context,
	l log.Logger,
	extCfg *ExtendedRemoteStateConfigAzurerm,
	cfg *azurehelper.AzureConfig,
	opts *backend.Options,
) error {
	// A user-managed account with no policy work needs nothing from ARM.
	if !armWorkRequested(extCfg) {
		return nil
	}

	// Blob versioning and soft delete are account-scoped, but the caller's
	// mutex is keyed per container (account/container), so units sharing an
	// account through different containers would race this read-modify-write.
	// Serialize account-plane convergence per storage account. The lock order
	// is always container-key then account-key, so this cannot deadlock.
	accountMu := b.GetBucketMutex(extCfg.RemoteStateConfigAzurerm.StorageAccountName)

	accountMu.Lock()
	defer accountMu.Unlock()

	saClient, err := azurehelper.NewStorageAccountClient(cfg)
	if err != nil {
		return err
	}

	exists, err := saClient.Exists(ctx)
	if err != nil {
		return err
	}

	if !exists {
		// The account is the user's responsibility under
		// skip_storage_account_creation; there is nothing to converge until it
		// exists, so return without touching versioning / soft delete.
		if extCfg.SkipStorageAccountCreation {
			return nil
		}

		if err := createAccount(ctx, l, saClient, extCfg, cfg, opts); err != nil {
			return err
		}
	}

	// Converge versioning / soft delete on both new and pre-existing accounts
	// (including under skip_storage_account_creation). EnableVersioning and
	// EnableSoftDelete are read-modify-writes, so they do not clobber each other.
	if !extCfg.SkipVersioning {
		if err := saClient.EnableVersioning(ctx, l); err != nil {
			return err
		}
	}

	if extCfg.EnableSoftDelete {
		if err := saClient.EnableSoftDelete(ctx, l, int(effectiveSoftDeleteDays(extCfg))); err != nil {
			return err
		}
	}

	return nil
}

// createAccount provisions the resource group (when allowed) and the storage
// account, honoring the fail-if-creation-required gate.
func createAccount(
	ctx context.Context,
	l log.Logger,
	saClient *azurehelper.StorageAccountClient,
	extCfg *ExtendedRemoteStateConfigAzurerm,
	cfg *azurehelper.AzureConfig,
	opts *backend.Options,
) error {
	// Refuse to provision anything (resource group or account) when the
	// caller forbids creation.
	if opts.FailIfBucketCreationRequired {
		return backend.BucketCreationNotAllowed(extCfg.RemoteStateConfigAzurerm.StorageAccountName)
	}

	// The resource group must exist before the account; only create it when
	// we are actually creating the account (an existing account already has
	// its resource group). cfg.ResourceGroup carries the env-resolved value
	// the storage account client is bound to, so gate and create on it.
	if !extCfg.SkipResourceGroupCreation && cfg.ResourceGroup != "" {
		rgClient, err := azurehelper.NewResourceGroupClient(cfg)
		if err != nil {
			return err
		}

		if err := rgClient.EnsureResourceGroup(ctx, l, cfg.ResourceGroup, extCfg.Location); err != nil {
			return err
		}
	}

	return saClient.Create(ctx, l, extCfg.StorageAccountConfig())
}

// accountPolicyDrift reports whether the existing account's blob versioning or
// soft-delete configuration differs from what the config requests.
func accountPolicyDrift(
	ctx context.Context,
	saClient *azurehelper.StorageAccountClient,
	extCfg *ExtendedRemoteStateConfigAzurerm,
) (bool, error) {
	if !extCfg.SkipVersioning {
		enabled, err := saClient.IsVersioningEnabled(ctx)
		if err != nil {
			return false, err
		}

		if !enabled {
			return true, nil
		}
	}

	if extCfg.EnableSoftDelete {
		blobDays, containerDays, err := saClient.SoftDeleteRetention(ctx)
		if err != nil {
			return false, err
		}

		// Drift when soft delete is off (0 days) or either policy's retention
		// differs from what bootstrap would apply, so a changed
		// soft_delete_retention_days is reconciled instead of silently skipped.
		desired := effectiveSoftDeleteDays(extCfg)
		if blobDays != desired || containerDays != desired {
			return true, nil
		}
	}

	return false, nil
}

// effectiveSoftDeleteDays returns the retention that bootstrap actually applies
// for the requested count: unset (0) and out-of-range values collapse to
// defaultSoftDeleteRetentionDays, mirroring StorageAccountClient.EnableSoftDelete.
func effectiveSoftDeleteDays(extCfg *ExtendedRemoteStateConfigAzurerm) int32 {
	days := extCfg.SoftDeleteRetentionDays
	if days < 1 || days > 365 {
		return int32(defaultSoftDeleteRetentionDays)
	}

	return int32(days)
}

// IsVersionControlEnabled returns true if blob versioning is enabled on the
// storage account. Data-plane-only auth (SAS / access key) cannot query this
// via ARM and returns false.
func (b *Backend) IsVersionControlEnabled(ctx context.Context, l log.Logger, v *venv.Venv, backendConfig backend.Config, opts *backend.Options) (bool, error) {
	if !experimentEnabled(opts) {
		return false, nil
	}

	_, cfg, err := resolveConfig(l, v, backendConfig)
	if err != nil {
		return false, err
	}

	if !armCapable(cfg) {
		l.Warnf("Cannot check blob versioning for %s backend with %s authentication; skipping.", b.Name(), cfg.Method)

		return false, nil
	}

	// Versioning is an ARM management-plane property, unreachable without a
	// resource group; degrade the same way as data-plane-only auth.
	if cfg.ResourceGroup == "" {
		l.Warnf("Cannot check blob versioning for %s backend without resource_group_name; skipping.", b.Name())

		return false, nil
	}

	saClient, err := azurehelper.NewStorageAccountClient(cfg)
	if err != nil {
		return false, err
	}

	return saClient.IsVersioningEnabled(ctx)
}

// Migrate copies the state blob from the source backend config to the
// destination backend config within the same storage account.
func (b *Backend) Migrate(ctx context.Context, l log.Logger, srcV, dstV *venv.Venv, srcBackendConfig, dstBackendConfig backend.Config, opts *backend.Options) error {
	if !experimentEnabled(opts) {
		return ErrAzureBackendExperimentRequired
	}

	srcExtCfg, cfg, err := resolveConfig(l, srcV, srcBackendConfig)
	if err != nil {
		return err
	}

	// Resolve from dstV because its ARM_ENVIRONMENT or AZURE_ENVIRONMENT may select a different cloud than srcV.
	dstExtCfg, dstCfg, err := resolveConfig(l, dstV, dstBackendConfig)
	if err != nil {
		return err
	}

	src := &srcExtCfg.RemoteStateConfigAzurerm
	dst := &dstExtCfg.RemoteStateConfigAzurerm

	// A storage account name identifies a different account in each Azure
	// cloud, and the blob client below is built from the source config, so a
	// cross-cloud destination would be written into the source account and the
	// source key then deleted.

	if dstCfg.CloudConfig.ActiveDirectoryAuthorityHost != cfg.CloudConfig.ActiveDirectoryAuthorityHost {
		return &CrossCloudMigrationError{
			StorageAccount: src.StorageAccountName,
			SrcEnvironment: cloudName(src.Environment, cfg.CloudConfig.ActiveDirectoryAuthorityHost),
			DstEnvironment: cloudName(dst.Environment, dstCfg.CloudConfig.ActiveDirectoryAuthorityHost),
		}
	}

	// Only the source backend config is resolved into credentials here, so the
	// single client below can reach the source account. ContainerClient.CopyBlob
	// itself writes through the destination's own account, but this backend has
	// no destination credentials to build that client from, so refuse a
	// cross-account migration rather than silently writing into the source
	// account. This same-backend Migrate has no automatic pull/push fallback;
	// the user must migrate cross-account state manually (init/pull/push).
	if !strings.EqualFold(src.StorageAccountName, dst.StorageAccountName) {
		return &CrossAccountMigrationError{
			SrcStorageAccount: src.StorageAccountName,
			DstStorageAccount: dst.StorageAccountName,
		}
	}

	blobClient, err := newBlobClient(ctx, l, cfg)
	if err != nil {
		return err
	}

	// Move (copy + delete source), mirroring the S3 and GCS backends: refuse to
	// overwrite an existing destination and leave no stale state at the old key.
	return blobClient.Container(src.ContainerName).
		MoveBlobIfNecessary(ctx, l, src.Key, blobClient.Container(dst.ContainerName), dst.Key)
}

// Delete deletes the Terraform state blob (config "key") from its container.
func (b *Backend) Delete(ctx context.Context, l log.Logger, v *venv.Venv, backendConfig backend.Config, opts *backend.Options) error {
	if !experimentEnabled(opts) {
		return ErrAzureBackendExperimentRequired
	}

	extCfg, cfg, err := resolveConfig(l, v, backendConfig)
	if err != nil {
		return err
	}

	rs := &extCfg.RemoteStateConfigAzurerm

	blobClient, err := newBlobClient(ctx, l, cfg)
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf(
		"The Terraform state blob %q in container %q (storage account %q) will be deleted. Do you want to continue?",
		rs.Key,
		rs.ContainerName,
		rs.StorageAccountName,
	)

	yes, err := shell.PromptUserForYesNo(ctx, l, v, prompt, opts.NonInteractive)
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	return blobClient.Container(rs.ContainerName).EnsureBlobDeleted(ctx, rs.Key)
}

// DeleteBucket deletes the entire blob container backing the state.
func (b *Backend) DeleteBucket(ctx context.Context, l log.Logger, v *venv.Venv, backendConfig backend.Config, opts *backend.Options) error {
	if !experimentEnabled(opts) {
		return ErrAzureBackendExperimentRequired
	}

	extCfg, cfg, err := resolveConfig(l, v, backendConfig)
	if err != nil {
		return err
	}

	rs := &extCfg.RemoteStateConfigAzurerm

	blobClient, err := newBlobClient(ctx, l, cfg)
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf(
		"The blob container %q in storage account %q will be completely deleted. Do you want to continue?",
		rs.ContainerName,
		rs.StorageAccountName,
	)

	yes, err := shell.PromptUserForYesNo(ctx, l, v, prompt, opts.NonInteractive)
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	return blobClient.Container(rs.ContainerName).EnsureDeleted(ctx)
}

// GetTFInitArgs returns the subset of config forwarded to `tofu init -backend-config`.
func (b *Backend) GetTFInitArgs(config backend.Config) map[string]any {
	return Config(config).GetTFInitArgs()
}
