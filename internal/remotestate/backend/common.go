package backend

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/options"
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

// NeedsBootstrap implements `backends.NeedsBootstrap` interface.
func (backend *CommonBackend) NeedsBootstrap(ctx context.Context, config Config, opts *options.TerragruntOptions) (bool, error) {
	opts.Logger.Warnf("NeedsBootstrap for %s backend not implemented", backend.Name())

	return true, nil
}

// Bootstrap implements `backends.Bootstrap` interface.
func (backend *CommonBackend) Bootstrap(ctx context.Context, config Config, opts *options.TerragruntOptions) error {
	opts.Logger.Warnf("Bootstrap for %s backend not implemented", backend.Name())

	return nil
}

// Delete implements `backends.Delete` interface.
func (backend *CommonBackend) Delete(ctx context.Context, config Config, opts *options.TerragruntOptions) error {
	opts.Logger.Warnf("Delete for %s backend not implemented", backend.Name())

	return nil
}

// DeleteBucket implements `backends.DeleteBucket` interface.
func (backend *CommonBackend) DeleteBucket(ctx context.Context, config Config, opts *options.TerragruntOptions) error {
	opts.Logger.Warnf("Deleting entire bucket for %s backend not implemented", backend.Name())

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
