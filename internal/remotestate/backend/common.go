package backend

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/puzpuzpuz/xsync/v3"
)

var _ Backend = new(CommonBackend)

type CommonBackend struct {
	bucketLocks   *xsync.MapOf[string, *sync.Mutex]
	initedConfigs *xsync.MapOf[string, bool]
	name          string
}

func NewCommonBackend(name string) *CommonBackend {
	return &CommonBackend{
		name:          name,
		bucketLocks:   xsync.NewMapOf[string, *sync.Mutex](),
		initedConfigs: xsync.NewMapOf[string, bool](),
	}
}

// Name implements `backends.Backend` interface.
func (backend *CommonBackend) Name() string {
	return backend.name
}

func (backend *CommonBackend) IsVersionControlEnabled(ctx context.Context, l log.Logger, config Config, opts *options.TerragruntOptions) (bool, error) {
	l.Warnf("Checking version control for %s backend not implemented.", backend.Name())

	return false, nil
}

// NeedsBootstrap implements `backends.NeedsBootstrap` interface.
func (backend *CommonBackend) NeedsBootstrap(ctx context.Context, l log.Logger, config Config, opts *options.TerragruntOptions) (bool, error) {
	return false, nil
}

// Bootstrap implements `backends.Bootstrap` interface.
func (backend *CommonBackend) Bootstrap(ctx context.Context, l log.Logger, config Config, opts *options.TerragruntOptions) error {
	l.Warnf("Bootstrap for %s backend not implemented.", backend.Name())

	return nil
}

// Migrate implements `backends.Migrate` interface.
func (backend *CommonBackend) Migrate(ctx context.Context, l log.Logger, srcConfig, dstConfig Config, opts *options.TerragruntOptions) error {
	l.Warnf("Migrate for %s backend not implemented.", backend.Name())

	return nil
}

// Delete implements `backends.Delete` interface.
func (backend *CommonBackend) Delete(ctx context.Context, l log.Logger, config Config, opts *options.TerragruntOptions) error {
	l.Warnf("Delete for %s backend not implemented.", backend.Name())

	return nil
}

// DeleteBucket implements `backends.DeleteBucket` interface.
func (backend *CommonBackend) DeleteBucket(ctx context.Context, l log.Logger, config Config, opts *options.TerragruntOptions) error {
	l.Warnf("Deleting entire bucket for %s backend not implemented.", backend.Name())

	return nil
}

// GetTFInitArgs implements `backends.GetTFInitArgs` interface.
func (backend *CommonBackend) GetTFInitArgs(config Config) map[string]any {
	return config
}

func (backend *CommonBackend) GetBucketMutex(bucketName string) *sync.Mutex {
	mu, _ := backend.bucketLocks.LoadOrCompute(bucketName, func() *sync.Mutex {
		return new(sync.Mutex)
	})

	return mu
}

func (backend *CommonBackend) IsConfigInited(config interface{ CacheKey() string }) bool {
	status, ok := backend.initedConfigs.Load(config.CacheKey())

	return ok && status
}

func (backend *CommonBackend) MarkConfigInited(config interface{ CacheKey() string }) {
	backend.initedConfigs.Store(config.CacheKey(), true)
}
