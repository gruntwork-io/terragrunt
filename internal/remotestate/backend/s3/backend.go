package s3

import (
	"context"

	"github.com/gruntwork-io/terragrunt/awshelper"
	"github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
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

	extS3Cfg, err := cfg.ParseExtendedS3Config()
	if err != nil {
		return false, err
	}

	s3Cfg := extS3Cfg.RemoteStateConfigS3

	client, err := NewClient(extS3Cfg, opts)
	if err != nil {
		return false, err
	}

	// Validate current AWS session before checking S3
	if !extS3Cfg.SkipCredentialsValidation {
		if err = awshelper.ValidateAwsSession(client.session); err != nil {
			return false, err
		}
	}

	if !client.DoesS3BucketExist(ctx, s3Cfg.Bucket) {
		return true, nil
	}

	if s3Cfg.GetLockTableName() != "" {
		dynamodbClient := dynamodb.New(client.session)

		tableExists, err := dynamodb.LockTableExistsAndIsActive(s3Cfg.GetLockTableName(), dynamodbClient)
		if err != nil {
			return false, err
		}

		if !tableExists {
			return true, nil
		}
	}

	return false, nil
}

// Init the remote state S3 bucket specified in the given config. This function will validate the config
// parameters, create the S3 bucket if it doesn't already exist, and check that versioning is enabled.
func (backend *Backend) Init(ctx context.Context, backedConfig backend.Config, opts *options.TerragruntOptions) error {
	extS3Cfg, err := Config(backedConfig).Normalize(opts.Logger).ParseExtendedS3Config()
	if err != nil {
		return errors.New(err)
	}

	if err := extS3Cfg.Validate(); err != nil {
		return errors.New(err)
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

	if err := client.createS3BucketIfNecessary(ctx, bucketName, opts); err != nil {
		return err
	}

	if !extS3Cfg.DisableBucketUpdate {
		if err := client.updateS3BucketIfNecessary(ctx, bucketName, opts); err != nil {
			return err
		}
	}

	if !extS3Cfg.SkipBucketVersioning {
		if _, err := client.checkIfVersioningEnabled(ctx, bucketName); err != nil {
			return err
		}
	}

	if err := extS3Cfg.CreateLockTableIfNecessary(extS3Cfg.DynamotableTags, opts); err != nil {
		return err
	}

	if err := extS3Cfg.UpdateLockTableSetSSEncryptionOnIfNecessary(opts); err != nil {
		return err
	}

	backend.MarkConfigInited(s3Cfg)

	return nil
}

// DeleteBucket deletes the remote state S3 bucket specified in the given config.
func (backend *Backend) DeleteBucket(ctx context.Context, backednConfig backend.Config, opts *options.TerragruntOptions) error {
	extS3Cfg, err := Config(backednConfig).Normalize(opts.Logger).ParseExtendedS3Config()
	if err != nil {
		return err
	}

	if err := extS3Cfg.Validate(); err != nil {
		return err
	}

	client, err := NewClient(extS3Cfg, opts)
	if err != nil {
		return err
	}

	if err := client.DeleteS3BucketIfNecessary(ctx, extS3Cfg.RemoteStateConfigS3.Bucket); err != nil {
		return err
	}

	return nil
}

func (backend *Backend) GetTerraformInitArgs(config backend.Config) map[string]any {
	return Config(config).GetTerraformInitArgs()
}
