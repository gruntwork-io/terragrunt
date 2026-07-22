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

// checkExperiment returns ErrAzureBackendExperimentRequired unless the
// azure-backend experiment is enabled.
func checkExperiment(opts *backend.Options) error {
	if opts == nil || !opts.Experiments.Evaluate(experiment.AzureBackend) {
		return ErrAzureBackendExperimentRequired
	}

	return nil
}

// resolveConfig parses, validates, and resolves the azure session config for
// the given raw backend config, returning the parsed config and the resolved
// azurehelper.AzureConfig (credentials + cloud).
func resolveConfig(
	l log.Logger,
	v venv.Venv,
	backendConfig backend.Config,
) (*ExtendedRemoteStateConfigAzurerm, *azurehelper.AzureConfig, error) {
	extCfg, err := Config(backendConfig).ExtendedAzurermConfig()
	if err != nil {
		return nil, nil, err
	}

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(extCfg.GetAzureSessionConfig()).
		WithVenv(v).
		Build(l)
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
	l.Warnf("Cannot manage the storage account for %s backend with %s authentication; skipping account creation and versioning/soft-delete convergence.", name, method)
}

// NeedsBootstrap returns true if the storage account or container backing the
// state does not yet exist, or (when reachable) blob versioning or soft-delete
// configuration has drifted from what the config requests.
func (b *Backend) NeedsBootstrap(ctx context.Context, l log.Logger, v venv.Venv, backendConfig backend.Config, opts *backend.Options) (bool, error) {
	if err := checkExperiment(opts); err != nil {
		return false, err
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
		blobClient, err := azurehelper.NewBlobClient(cfg)
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
func accountNeedsBootstrap(ctx context.Context, cfg *azurehelper.AzureConfig, extCfg *ExtendedRemoteStateConfigAzurerm) (bool, error) {
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
func (b *Backend) Bootstrap(ctx context.Context, l log.Logger, v venv.Venv, backendConfig backend.Config, opts *backend.Options) error {
	if err := checkExperiment(opts); err != nil {
		return err
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

	if b.IsConfigInited(rs) {
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

	if err := ensureContainer(ctx, cfg, extCfg, opts); err != nil {
		return err
	}

	b.MarkConfigInited(rs)

	return nil
}

// ensureContainer creates the state container when it is missing and creation
// is neither skipped nor forbidden.
func ensureContainer(ctx context.Context, cfg *azurehelper.AzureConfig, extCfg *ExtendedRemoteStateConfigAzurerm, opts *backend.Options) error {
	if extCfg.SkipContainerCreation {
		return nil
	}

	rs := &extCfg.RemoteStateConfigAzurerm

	blobClient, err := azurehelper.NewBlobClient(cfg)
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
func (b *Backend) IsVersionControlEnabled(ctx context.Context, l log.Logger, v venv.Venv, backendConfig backend.Config, opts *backend.Options) (bool, error) {
	if err := checkExperiment(opts); err != nil {
		return false, err
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
func (b *Backend) Migrate(ctx context.Context, l log.Logger, v venv.Venv, srcBackendConfig, dstBackendConfig backend.Config, opts *backend.Options) error {
	if err := checkExperiment(opts); err != nil {
		return err
	}

	srcExtCfg, cfg, err := resolveConfig(l, v, srcBackendConfig)
	if err != nil {
		return err
	}

	dstExtCfg, err := Config(dstBackendConfig).ExtendedAzurermConfig()
	if err != nil {
		return err
	}

	src := &srcExtCfg.RemoteStateConfigAzurerm
	dst := &dstExtCfg.RemoteStateConfigAzurerm

	// The blob client is bound to a single storage account (the source), so it
	// cannot copy across accounts. Refuse a cross-account migration loudly
	// rather than silently writing into the source account. This same-backend
	// Migrate has no automatic pull/push fallback; the user must migrate
	// cross-account state manually (init/pull/push).
	if !strings.EqualFold(src.StorageAccountName, dst.StorageAccountName) {
		return fmt.Errorf(
			"cross-account state migration from storage account %q to %q is not supported by the azurerm backend "+
				"(its blob client is bound to a single storage account); "+
				"migrate via separate init/pull/push or keep both units on the same storage account",
			src.StorageAccountName, dst.StorageAccountName,
		)
	}

	blobClient, err := azurehelper.NewBlobClient(cfg)
	if err != nil {
		return err
	}

	// Move (copy + delete source), mirroring the S3 and GCS backends: refuse to
	// overwrite an existing destination and leave no stale state at the old key.
	return blobClient.Container(src.ContainerName).MoveBlobIfNecessary(ctx, src.Key, blobClient.Container(dst.ContainerName), dst.Key)
}

// Delete deletes the Terraform state blob (config "key") from its container.
func (b *Backend) Delete(ctx context.Context, l log.Logger, v venv.Venv, backendConfig backend.Config, opts *backend.Options) error {
	if err := checkExperiment(opts); err != nil {
		return err
	}

	extCfg, cfg, err := resolveConfig(l, v, backendConfig)
	if err != nil {
		return err
	}

	rs := &extCfg.RemoteStateConfigAzurerm

	blobClient, err := azurehelper.NewBlobClient(cfg)
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf("The Terraform state blob %q in container %q (storage account %q) will be deleted. Do you want to continue?",
		rs.Key, rs.ContainerName, rs.StorageAccountName)

	yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts.NonInteractive, v.Writers.ErrWriter)
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	return blobClient.Container(rs.ContainerName).EnsureBlobDeleted(ctx, rs.Key)
}

// DeleteBucket deletes the entire blob container backing the state.
func (b *Backend) DeleteBucket(ctx context.Context, l log.Logger, v venv.Venv, backendConfig backend.Config, opts *backend.Options) error {
	if err := checkExperiment(opts); err != nil {
		return err
	}

	extCfg, cfg, err := resolveConfig(l, v, backendConfig)
	if err != nil {
		return err
	}

	rs := &extCfg.RemoteStateConfigAzurerm

	blobClient, err := azurehelper.NewBlobClient(cfg)
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf("The blob container %q in storage account %q will be completely deleted. Do you want to continue?",
		rs.ContainerName, rs.StorageAccountName)

	yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts.NonInteractive, v.Writers.ErrWriter)
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
