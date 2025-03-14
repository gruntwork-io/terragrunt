// Package gcs represents GCS backend for interacting with remote state.
package gcs

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
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
func (backend *Backend) NeedsInit(ctx context.Context, backednConfig backend.Config, backendExistingConfig backend.Config, opts *options.TerragruntOptions) (bool, error) {
	if Config(backednConfig).IsEqual(Config(backendExistingConfig), opts.Logger) {
		return true, nil
	}

	extGCSCfg, err := Config(backednConfig).ParseExtendedGCSConfig()
	if err != nil {
		return false, err
	}

	var (
		gcsCfg     = &extGCSCfg.RemoteStateConfigGCS
		bucketName = gcsCfg.Bucket
	)

	gcsClient, err := NewClient(ctx, extGCSCfg, opts.Logger)
	if err != nil {
		return false, err
	}

	defer func() {
		if err := gcsClient.Close(); err != nil {
			opts.Logger.Warnf("Error closing GCS client: %v", err)
		}
	}()

	if !gcsClient.DoesGCSBucketExist(ctx, bucketName) {
		return true, nil
	}

	return false, nil
}

// Init the remote state GCS bucket specified in the given config. This function will validate the config
// parameters, create the GCS bucket if it doesn't already exist, and check that versioning is enabled.
func (backend *Backend) Init(ctx context.Context, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	extGCSCfg, err := Config(backendConfig).ParseExtendedGCSConfig()
	if err != nil {
		return err
	}

	if err := extGCSCfg.Validate(ctx); err != nil {
		return err
	}

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

	gcsClient, err := NewClient(ctx, extGCSCfg, opts.Logger)
	if err != nil {
		return err
	}

	defer func() {
		if err := gcsClient.Close(); err != nil {
			opts.Logger.Warnf("Error closing GCS client: %v", err)
		}
	}()

	// If bucket is specified and skip_bucket_creation is false then check if Bucket needs to be created
	if !extGCSCfg.SkipBucketCreation && bucketName != "" {
		if err := gcsClient.createGCSBucketIfNecessary(ctx, bucketName, opts); err != nil {
			return err
		}
	}
	// If bucket is specified and skip_bucket_versioning is false then warn user if versioning is disabled on bucket
	if !extGCSCfg.SkipBucketVersioning && bucketName != "" {
		// TODO: Remove lint suppression
		if err := gcsClient.checkIfGCSVersioningEnabled(bucketName); err != nil { //nolint:contextcheck
			return err
		}
	}

	backend.MarkConfigInited(gcsCfg)

	return nil
}

// GetTFInitArgs returns the subset of the given config that should be passed to terraform init
// when initializing the remote state.
func (backend *Backend) GetTFInitArgs(config backend.Config) map[string]any {
	return Config(config).GetTFInitArgs()
}
