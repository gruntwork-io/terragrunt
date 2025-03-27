// Package gcs represents GCS backend for interacting with remote state.
package gcs

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

const BackendName = "gcs"

var _ backend.Backend = new(Backend)

type Backend struct {
	*backend.CommonBackend
}

func NewBackend() *Backend {
	return &Backend{
		CommonBackend: backend.NewCommonBackend(BackendName),
	}
}

// NeedsInit returns true if the GCS bucket specified in the given config does not exist or if the bucket
// exists but versioning is not enabled.
//
// Returns true if:
//
// 1. Any of the existing backend settings are different than the current config
// 2. The configured GCS bucket does not exist
func (backend *Backend) NeedsInit(ctx context.Context, backendConfig backend.Config, backendExistingConfig backend.Config, opts *options.TerragruntOptions) (bool, error) {
	extGCSCfg, err := Config(backendConfig).ExtendedGCSConfig()
	if err != nil {
		return false, err
	}

	if Config(backendConfig).IsEqual(Config(backendExistingConfig), opts.Logger) {
		return true, nil
	}

	var (
		gcsCfg     = &extGCSCfg.RemoteStateConfigGCS
		bucketName = gcsCfg.Bucket
	)

	client, err := NewClient(ctx, extGCSCfg, opts.Logger)
	if err != nil {
		return false, err
	}

	defer func() {
		if err := client.Close(); err != nil {
			opts.Logger.Warnf("Error closing GCS client: %v", err)
		}
	}()

	if !client.DoesGCSBucketExist(ctx, bucketName) {
		return true, nil
	}

	return false, nil
}

// Init the remote state GCS bucket specified in the given config. This function will validate the config
// parameters, create the GCS bucket if it doesn't already exist, and check that versioning is enabled.
func (backend *Backend) Init(ctx context.Context, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	extGCSCfg, err := Config(backendConfig).ExtendedGCSConfig()
	if err != nil {
		return err
	}

	client, err := NewClient(ctx, extGCSCfg, opts.Logger)
	if err != nil {
		return err
	}

	defer func() {
		if err := client.Close(); err != nil {
			opts.Logger.Warnf("Error closing GCS client: %v", err)
		}
	}()

	var (
		gcsCfg     = &extGCSCfg.RemoteStateConfigGCS
		bucketName = gcsCfg.Bucket
	)

	// ensure that only one goroutine can initialize bucket
	mu := backend.GetBucketMutex(bucketName)
	mu.Lock()
	defer mu.Unlock()

	if backend.IsConfigInited(gcsCfg) {
		opts.Logger.Debugf("%s bucket %s has already been confirmed to be initialized, skipping initialization checks", backend.Name(), bucketName)

		return nil
	}

	// If bucket is specified and skip_bucket_creation is false then check if Bucket needs to be created
	if !extGCSCfg.SkipBucketCreation && bucketName != "" {
		if err := client.CreateGCSBucketIfNecessary(ctx, bucketName, opts); err != nil {
			return err
		}
	}
	// If bucket is specified and skip_bucket_versioning is false then warn user if versioning is disabled on bucket
	if !extGCSCfg.SkipBucketVersioning && bucketName != "" {
		// TODO: Remove lint suppression
		if _, err := client.CheckIfGCSVersioningEnabled(ctx, bucketName); err != nil { //nolint:contextcheck
			return err
		}
	}

	backend.MarkConfigInited(gcsCfg)

	return nil
}

// Delete deletes the remote state specified in the given config.
func (backend *Backend) Delete(ctx context.Context, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	extGCSCfg, err := Config(backendConfig).ExtendedGCSConfig()
	if err != nil {
		return err
	}

	client, err := NewClient(ctx, extGCSCfg, opts.Logger)
	if err != nil {
		return err
	}

	if !opts.ForceBackendDelete {
		versioned, err := client.CheckIfGCSVersioningEnabled(ctx, extGCSCfg.RemoteStateConfigGCS.Bucket)
		if err != nil {
			return err
		}

		if !versioned {
			return errors.New("bucket is not versioned, refusing to delete backend state. If you are sure you want to delete the backend state anyways, use the --force flag")
		}
	}

	var (
		bucketName = extGCSCfg.RemoteStateConfigGCS.Bucket
		prefix     = extGCSCfg.RemoteStateConfigGCS.Prefix
	)

	return client.DeleteGCSObjectIfNecessary(ctx, bucketName, prefix)
}

// DeleteBucket deletes the entire bucket specified in the given config.
func (backend *Backend) DeleteBucket(ctx context.Context, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	extGCSCfg, err := Config(backendConfig).ExtendedGCSConfig()
	if err != nil {
		return err
	}

	client, err := NewClient(ctx, extGCSCfg, opts.Logger)
	if err != nil {
		return err
	}

	var bucketName = extGCSCfg.RemoteStateConfigGCS.Bucket

	prompt := fmt.Sprintf("GCS bucket %s will be completely deleted. Do you want to continue?", bucketName)
	if yes, err := shell.PromptUserForYesNo(ctx, prompt, opts); err != nil {
		return err
	} else if yes {
		return client.DeleteGCSBucketIfNecessary(ctx, bucketName)
	}

	return nil
}

// GetTFInitArgs returns the subset of the given config that should be passed to terraform init
// when initializing the remote state.
func (backend *Backend) GetTFInitArgs(config backend.Config) map[string]any {
	return Config(config).FilterOutTerragruntKeys()
}
