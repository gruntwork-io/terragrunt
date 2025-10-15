// Package s3 represents AWS S3 backend for interacting with remote state.
package s3

import (
	"context"
	"fmt"
	"path"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

// NeedsBootstrap returns true if the remote state S3 bucket specified in the given config needs to be bootstrapped.
//
// Returns true if:
//
// 1. Any of the existing backend settings are different than the current config
// 2. The configured S3 bucket or DynamoDB table does not exist
func (backend *Backend) NeedsBootstrap(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) (bool, error) {
	cfg := Config(backendConfig).Normalize(l)

	extS3Cfg, err := cfg.ExtendedS3Config(l)
	if err != nil {
		return false, err
	}

	client, err := NewClient(ctx, l, extS3Cfg, opts)
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

// Bootstrap the remote state S3 bucket specified in the given config. This function will validate the config
// parameters, create the S3 bucket if it doesn't already exist, and check that versioning is enabled.
func (backend *Backend) Bootstrap(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	extS3Cfg, err := Config(backendConfig).ExtendedS3Config(l)
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
		l.Debugf("%s bucket %s has already been confirmed to be initialized, skipping initialization checks", backend.Name(), bucketName)

		return nil
	}

	client, err := NewClient(ctx, l, extS3Cfg, opts)
	if err != nil {
		return err
	}

	if err := client.CreateS3BucketIfNecessary(ctx, l, bucketName, opts); err != nil {
		return err
	}

	if !extS3Cfg.DisableBucketUpdate {
		if err := client.UpdateS3BucketIfNecessary(ctx, l, bucketName, opts); err != nil {
			return err
		}
	}

	if !extS3Cfg.SkipBucketVersioning {
		if _, err := client.CheckIfVersioningEnabled(ctx, l, bucketName); err != nil {
			return err
		}
	}

	if tableName := extS3Cfg.RemoteStateConfigS3.GetLockTableName(); tableName != "" {
		if err := client.CreateLockTableIfNecessary(ctx, l, tableName, extS3Cfg.DynamotableTags); err != nil {
			return err
		}

		if extS3Cfg.EnableLockTableSSEncryption {
			if err := client.UpdateLockTableSetSSEncryptionOnIfNecessary(ctx, l, tableName); err != nil {
				return err
			}
		}
	}

	backend.MarkConfigInited(s3Cfg)

	return nil
}

// IsVersionControlEnabled returns true if version control for s3 bucket is enabled.
func (backend *Backend) IsVersionControlEnabled(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) (bool, error) {
	extS3Cfg, err := Config(backendConfig).ExtendedS3Config(l)
	if err != nil {
		return false, err
	}

	var bucketName = extS3Cfg.RemoteStateConfigS3.Bucket

	client, err := NewClient(ctx, l, extS3Cfg, opts)
	if err != nil {
		return false, err
	}

	return client.CheckIfVersioningEnabled(ctx, l, bucketName)
}

// Migrate copies the s3 bucket object located at src config to dst config and deletes the src object.
// Creates a new DynamoDB table item for dst config and deletes the table item from the src config.
func (backend *Backend) Migrate(ctx context.Context, l log.Logger, srcBackendConfig, dstBackendConfig backend.Config, opts *options.TerragruntOptions) error {
	srcExtS3Cfg, err := Config(srcBackendConfig).ExtendedS3Config(l)
	if err != nil {
		return err
	}

	dstExtS3Cfg, err := Config(dstBackendConfig).ExtendedS3Config(l)
	if err != nil {
		return err
	}

	var (
		srcBucketName = srcExtS3Cfg.RemoteStateConfigS3.Bucket
		srcBucketKey  = srcExtS3Cfg.RemoteStateConfigS3.Key
		srcTableName  = srcExtS3Cfg.RemoteStateConfigS3.GetLockTableName()
		srcTableKey   = path.Join(srcBucketName, srcBucketKey+stateIDSuffix)

		dstBucketName = dstExtS3Cfg.RemoteStateConfigS3.Bucket
		dstBucketKey  = dstExtS3Cfg.RemoteStateConfigS3.Key
		dstTableName  = dstExtS3Cfg.RemoteStateConfigS3.GetLockTableName()
		dstTableKey   = path.Join(dstBucketName, dstBucketKey+stateIDSuffix)
	)

	client, err := NewClient(ctx, l, srcExtS3Cfg, opts)
	if err != nil {
		return err
	}

	if err = client.MoveS3ObjectIfNecessary(ctx, l, srcBucketName, srcBucketKey, dstBucketName, dstBucketKey); err != nil {
		return err
	}

	if dstTableName != "" {
		if err := client.CreateTableItemIfNecessary(ctx, l, dstTableName, dstTableKey); err != nil {
			return err
		}
	}

	if srcTableName != "" {
		return client.DeleteTableItemIfNecessary(ctx, l, srcTableName, srcTableKey)
	}

	return nil
}

// Delete deletes the remote state specified in the given config.
func (backend *Backend) Delete(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	extS3Cfg, err := Config(backendConfig).ExtendedS3Config(l)
	if err != nil {
		return err
	}

	var (
		bucketName = extS3Cfg.RemoteStateConfigS3.Bucket
		bucketKey  = extS3Cfg.RemoteStateConfigS3.Key
		tableName  = extS3Cfg.RemoteStateConfigS3.GetLockTableName()
	)

	client, err := NewClient(ctx, l, extS3Cfg, opts)
	if err != nil {
		return err
	}

	if tableName != "" {
		tableKey := path.Join(bucketName, bucketKey+stateIDSuffix)

		prompt := fmt.Sprintf("DynamoDB table %s key %s will be deleted. Do you want to continue?", tableName, tableKey)
		if yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts); err != nil {
			return err
		} else if yes {
			if err := client.DeleteTableItemIfNecessary(ctx, l, tableName, tableKey); err != nil {
				return err
			}
		}
	}

	prompt := fmt.Sprintf("S3 bucket %s key %s will be deleted. Do you want to continue?", bucketName, bucketKey)
	if yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts); err != nil {
		return err
	} else if yes {
		return client.DeleteS3ObjectIfNecessary(ctx, l, bucketName, bucketKey)
	}

	return nil
}

// DeleteBucket deletes the entire bucket specified in the given config.
func (backend *Backend) DeleteBucket(ctx context.Context, l log.Logger, backendConfig backend.Config, opts *options.TerragruntOptions) error {
	extS3Cfg, err := Config(backendConfig).ExtendedS3Config(l)
	if err != nil {
		return err
	}

	client, err := NewClient(ctx, l, extS3Cfg, opts)
	if err != nil {
		return err
	}

	var (
		bucketName = extS3Cfg.RemoteStateConfigS3.Bucket
		tableName  = extS3Cfg.RemoteStateConfigS3.GetLockTableName()
	)

	if tableName != "" {
		prompt := fmt.Sprintf("DynamoDB table %s will be completely deleted. Do you want to continue?", tableName)
		if yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts); err != nil {
			return err
		} else if yes {
			if err := client.DeleteTableIfNecessary(ctx, l, tableName); err != nil {
				return err
			}
		}
	}

	prompt := fmt.Sprintf("S3 bucket %s will be completely deleted. Do you want to continue?", bucketName)
	if yes, err := shell.PromptUserForYesNo(ctx, l, prompt, opts); err != nil {
		return err
	} else if yes {
		return client.DeleteS3BucketIfNecessary(ctx, l, bucketName)
	}

	return nil
}

func (backend *Backend) GetTFInitArgs(config backend.Config) map[string]any {
	return Config(config).GetTFInitArgs()
}
