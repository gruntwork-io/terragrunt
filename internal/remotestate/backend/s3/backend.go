// Package s3 represents AWS S3 backend for interacting with remote state.
package s3

import (
	"context"
	"fmt"
	"path"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

const BackendName = "s3"

var _ backend.Backend = new(Backend)

type Backend struct {
	*backend.CommonBackend
}

func NewBackend() *Backend {
	return &Backend{
		CommonBackend: backend.NewCommonBackend(BackendName),
	}
}

// NeedsInit returns true if the remote state S3 bucket specified in the given config needs to be initialized.
//
// Returns true if:
//
// 1. Any of the existing backend settings are different than the current config
// 2. The configured S3 bucket or DynamoDB table does not exist
func (backend *Backend) NeedsInit(ctx context.Context, backendConfig backend.Config, backendExistingConfig backend.Config, opts *options.TerragruntOptions) (bool, error) {
	cfg := Config(backendConfig).Normalize(opts.Logger)

	if !cfg.IsEqual(Config(backendExistingConfig), opts.Logger) {
		return true, nil
	}

	extS3Cfg, err := cfg.ExtendedS3Config(opts.Logger)
	if err != nil {
		return false, err
	}

	client, err := NewClient(extS3Cfg, opts)
	if err != nil {
		return false, err
	}

	var (
		bucketName = extS3Cfg.RemoteStateConfigS3.Bucket
		tableName  = extS3Cfg.RemoteStateConfigS3.GetLockTableName()
	)

	if exists, err := client.DoesS3BucketExist(ctx, bucketName); err != nil || !exists {
		return true, err
	}

	if tableName != "" {
		if exists, err := client.DoesLockTableExistAndIsActive(ctx, tableName); err != nil || !exists {
			return true, err
		}
	}

	return false, nil
}

// Init the remote state S3 bucket specified in the given config. This function will validate the config
// parameters, create the S3 bucket if it doesn't already exist, and check that versioning is enabled.
func (backend *Backend) Init(ctx context.Context, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	extS3Cfg, err := Config(backendConfig).ExtendedS3Config(opts.Logger)
	if err != nil {
		return err
	}

	var (
		s3Cfg      = &extS3Cfg.RemoteStateConfigS3
		bucketName = s3Cfg.Bucket
	)

	// ensure that only one goroutine can initialize bucket
	mu := backend.GetBucketMutex(bucketName)
	mu.Lock()
	defer mu.Unlock()

	if backend.IsConfigInited(s3Cfg) {
		opts.Logger.Debugf("%s bucket %s has already been confirmed to be initialized, skipping initialization checks", backend.Name(), bucketName)

		return nil
	}

	client, err := NewClient(extS3Cfg, opts)
	if err != nil {
		return err
	}

	if err := client.CreateS3BucketIfNecessary(ctx, bucketName, opts); err != nil {
		return err
	}

	if !extS3Cfg.DisableBucketUpdate {
		if err := client.UpdateS3BucketIfNecessary(ctx, bucketName, opts); err != nil {
			return err
		}
	}

	if !extS3Cfg.SkipBucketVersioning {
		if _, err := client.CheckIfVersioningEnabled(ctx, bucketName); err != nil {
			return err
		}
	}

	if tableName := extS3Cfg.RemoteStateConfigS3.GetLockTableName(); tableName != "" {
		if err := client.CreateLockTableIfNecessary(ctx, tableName, extS3Cfg.DynamotableTags); err != nil {
			return err
		}

		if err := client.UpdateLockTableSetSSEncryptionOnIfNecessary(ctx, tableName); err != nil {
			return err
		}
	}

	backend.MarkConfigInited(s3Cfg)

	return nil
}

// Delete deletes the remote state specified in the given config.
func (backend *Backend) Delete(ctx context.Context, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	extS3Cfg, err := Config(backendConfig).ExtendedS3Config(opts.Logger)
	if err != nil {
		return err
	}

	client, err := NewClient(extS3Cfg, opts)
	if err != nil {
		return err
	}

	if !opts.ForceBackendDelete {
		versioned, err := client.CheckIfVersioningEnabled(ctx, extS3Cfg.RemoteStateConfigS3.Bucket)
		if err != nil {
			return err
		}

		if !versioned {
			return errors.New("bucket is not versioned, refusing to delete backend state. If you are sure you want to delete the backend state anyways, use the --force flag")
		}
	}

	var (
		bucketName = extS3Cfg.RemoteStateConfigS3.Bucket
		bucketKey  = extS3Cfg.RemoteStateConfigS3.Key
		tableName  = extS3Cfg.RemoteStateConfigS3.GetLockTableName()
	)

	if tableName != "" {
		tableKey := path.Join(bucketName, bucketKey+stateIDSuffix)

		if err := client.DeleteTableItemIfNecessary(ctx, tableName, tableKey); err != nil {
			return err
		}
	}

	return client.DeleteS3ObjectIfNecessary(ctx, bucketName, bucketKey)
}

// DeleteBucket deletes the entire bucket specified in the given config.
func (backend *Backend) DeleteBucket(ctx context.Context, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	extS3Cfg, err := Config(backendConfig).ExtendedS3Config(opts.Logger)
	if err != nil {
		return err
	}

	client, err := NewClient(extS3Cfg, opts)
	if err != nil {
		return err
	}

	var (
		bucketName = extS3Cfg.RemoteStateConfigS3.Bucket
		tableName  = extS3Cfg.RemoteStateConfigS3.GetLockTableName()
	)

	if tableName != "" {
		prompt := fmt.Sprintf("DynamoDB table %s will be completely deleted. Do you want to continue?", tableName)
		if yes, err := shell.PromptUserForYesNo(ctx, prompt, opts); err != nil {
			return err
		} else if yes {
			if err := client.DeleteTableIfNecessary(ctx, tableName); err != nil {
				return err
			}
		}
	}

	prompt := fmt.Sprintf("S3 bucket %s will be completely deleted. Do you want to continue?", bucketName)
	if yes, err := shell.PromptUserForYesNo(ctx, prompt, opts); err != nil {
		return err
	} else if yes {
		return client.DeleteS3BucketIfNecessary(ctx, bucketName)
	}

	return nil
}

func (backend *Backend) GetTFInitArgs(config backend.Config) map[string]any {
	return Config(config).GetTFInitArgs()
}
