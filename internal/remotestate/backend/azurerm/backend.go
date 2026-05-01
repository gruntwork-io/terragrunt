// Package azurerm implements the Azure Storage (azurerm) remote-state
// backend. Bootstrap, migration and teardown lifecycle operations are
// gated behind the azure-backend experiment; when the experiment is
// disabled only Name() and GetTFInitArgs() are functional so that
// terragrunt init can still pass an azurerm backend block through to
// terraform unchanged.
package azurerm

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const BackendName = "azurerm"

var _ backend.Backend = new(Backend)

type Backend struct {
	*backend.CommonBackend
}

func NewBackend() *Backend {
	return &Backend{
		CommonBackend: backend.NewCommonBackend(BackendName),
	}
}

// GetTFInitArgs returns the config filtered to the keys the terraform
// azurerm backend understands (terragrunt-only keys removed).
func (b *Backend) GetTFInitArgs(config backend.Config) map[string]any {
	return Config(config).GetTFInitArgs()
}

// experimentEnabled reports whether the azure-backend experiment is on.
func experimentEnabled(opts *backend.Options) bool {
	return opts != nil && opts.Experiments.Evaluate(experiment.AzureBackend)
}

// NeedsBootstrap returns true if the configured storage account or
// container does not yet exist. Returns ExperimentNotEnabledError when
// the azure-backend experiment is disabled.
func (b *Backend) NeedsBootstrap(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *backend.Options) (bool, error) {
	if !experimentEnabled(opts) {
		return false, ExperimentNotEnabledError{}
	}

	extCfg, err := Config(backendConfig).ExtendedAzureRMConfig()
	if err != nil {
		return false, err
	}

	client, err := NewClient(ctx, l, extCfg, opts)
	if err != nil {
		return false, err
	}

	defer func() {
		if err := client.Close(); err != nil {
			l.Warnf("Error closing Azure client: %v", err)
		}
	}()

	if extCfg.SkipStorageAccountCreation && extCfg.SkipResourceGroupCreation && extCfg.SkipContainerCreation {
		return false, nil
	}

	if !extCfg.SkipStorageAccountCreation {
		exists, err := client.DoesStorageAccountExist(ctx)
		if err != nil {
			return false, err
		}

		if !exists {
			return true, nil
		}
	}

	if !extCfg.SkipContainerCreation {
		exists, err := client.DoesContainerExist(ctx)
		if err != nil {
			return false, err
		}

		if !exists {
			return true, nil
		}
	}

	return false, nil
}

// Bootstrap creates the resource group, storage account, and container
// (any of those that don't already exist and aren't skipped via
// skip_*_creation).
func (b *Backend) Bootstrap(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *backend.Options) error {
	if !experimentEnabled(opts) {
		return ExperimentNotEnabledError{}
	}

	extCfg, err := Config(backendConfig).ExtendedAzureRMConfig()
	if err != nil {
		return err
	}

	client, err := NewClient(ctx, l, extCfg, opts)
	if err != nil {
		return err
	}

	defer func() {
		if err := client.Close(); err != nil {
			l.Warnf("Error closing Azure client: %v", err)
		}
	}()

	mu := b.GetBucketMutex(extCfg.RemoteStateConfigAzureRM.StorageAccountName + "/" + extCfg.RemoteStateConfigAzureRM.ContainerName)

	mu.Lock()
	defer mu.Unlock()

	if b.IsConfigInited(&extCfg.RemoteStateConfigAzureRM) {
		l.Debugf("%s container %s/%s has already been confirmed to be initialized, skipping initialization checks",
			b.Name(), extCfg.RemoteStateConfigAzureRM.StorageAccountName, extCfg.RemoteStateConfigAzureRM.ContainerName)

		return nil
	}

	if err := client.CreateStorageAccountIfNecessary(ctx, l, opts); err != nil {
		return err
	}

	if !extCfg.SkipVersioning && !extCfg.SkipStorageAccountCreation {
		if _, err := client.IsVersioningEnabled(ctx, l); err != nil {
			return err
		}
	}

	if err := client.CreateContainerIfNecessary(ctx, l, opts); err != nil {
		return err
	}

	b.MarkConfigInited(&extCfg.RemoteStateConfigAzureRM)

	return nil
}

// IsVersionControlEnabled returns true if blob versioning is enabled on
// the configured storage account.
func (b *Backend) IsVersionControlEnabled(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *backend.Options) (bool, error) {
	if !experimentEnabled(opts) {
		return false, ExperimentNotEnabledError{}
	}

	extCfg, err := Config(backendConfig).ExtendedAzureRMConfig()
	if err != nil {
		return false, err
	}

	client, err := NewClient(ctx, l, extCfg, opts)
	if err != nil {
		return false, err
	}

	defer func() {
		if err := client.Close(); err != nil {
			l.Warnf("Error closing Azure client: %v", err)
		}
	}()

	return client.IsVersioningEnabled(ctx, l)
}

// Migrate moves the state blob from the source backend's container/key to
// the destination backend's container/key. Both backends must point at the
// same storage account (cross-account migration is not supported).
func (b *Backend) Migrate(ctx context.Context, l log.Logger, srcBackendConfig, dstBackendConfig backend.Config, opts *backend.Options) error {
	if !experimentEnabled(opts) {
		return ExperimentNotEnabledError{}
	}

	srcCfg, err := Config(srcBackendConfig).ExtendedAzureRMConfig()
	if err != nil {
		return err
	}

	dstCfg, err := Config(dstBackendConfig).ExtendedAzureRMConfig()
	if err != nil {
		return err
	}

	if srcCfg.RemoteStateConfigAzureRM.StorageAccountName != dstCfg.RemoteStateConfigAzureRM.StorageAccountName {
		return fmt.Errorf("azurerm migrate: cross-account migration is not supported (src=%s, dst=%s)",
			srcCfg.RemoteStateConfigAzureRM.StorageAccountName, dstCfg.RemoteStateConfigAzureRM.StorageAccountName)
	}

	client, err := NewClient(ctx, l, srcCfg, opts)
	if err != nil {
		return err
	}

	defer func() {
		if err := client.Close(); err != nil {
			l.Warnf("Error closing Azure client: %v", err)
		}
	}()

	return client.MoveBlob(ctx, srcCfg.RemoteStateConfigAzureRM.ContainerName, srcCfg.RemoteStateConfigAzureRM.Key, dstCfg.RemoteStateConfigAzureRM.ContainerName, dstCfg.RemoteStateConfigAzureRM.Key)
}

// Delete removes the state blob from the configured container after
// operator confirmation.
func (b *Backend) Delete(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *backend.Options) error {
	if !experimentEnabled(opts) {
		return ExperimentNotEnabledError{}
	}

	extCfg, err := Config(backendConfig).ExtendedAzureRMConfig()
	if err != nil {
		return err
	}

	client, err := NewClient(ctx, l, extCfg, opts)
	if err != nil {
		return err
	}

	defer func() {
		if err := client.Close(); err != nil {
			l.Warnf("Error closing Azure client: %v", err)
		}
	}()

	prompt := fmt.Sprintf("Azure blob %s/%s/%s will be deleted. Do you want to continue?",
		extCfg.RemoteStateConfigAzureRM.StorageAccountName, extCfg.RemoteStateConfigAzureRM.ContainerName, extCfg.RemoteStateConfigAzureRM.Key)

	yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts.NonInteractive, opts.Writers.ErrWriter)
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	return client.DeleteBlobIfExists(ctx, extCfg.RemoteStateConfigAzureRM.ContainerName, extCfg.RemoteStateConfigAzureRM.Key)
}

// DeleteBucket deletes the configured container (and the storage account
// itself when skip_storage_account_creation is false) after operator
// confirmation.
func (b *Backend) DeleteBucket(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *backend.Options) error {
	if !experimentEnabled(opts) {
		return ExperimentNotEnabledError{}
	}

	extCfg, err := Config(backendConfig).ExtendedAzureRMConfig()
	if err != nil {
		return err
	}

	client, err := NewClient(ctx, l, extCfg, opts)
	if err != nil {
		return err
	}

	defer func() {
		if err := client.Close(); err != nil {
			l.Warnf("Error closing Azure client: %v", err)
		}
	}()

	prompt := fmt.Sprintf("Azure container %s/%s will be completely deleted. Do you want to continue?",
		extCfg.RemoteStateConfigAzureRM.StorageAccountName, extCfg.RemoteStateConfigAzureRM.ContainerName)

	yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts.NonInteractive, opts.Writers.ErrWriter)
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	if err := client.DeleteContainer(ctx); err != nil {
		return err
	}

	if !extCfg.SkipStorageAccountCreation {
		if err := client.DeleteStorageAccount(ctx, l); err != nil {
			return err
		}
	}

	return nil
}
