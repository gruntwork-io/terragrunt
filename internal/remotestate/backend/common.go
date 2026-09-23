package backend

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

var _ Backend = new(CommonBackend)

type CommonBackend struct {
	bucketLocks   map[string]*sync.Mutex
	initedConfigs map[string]struct{}
	name          string
	mu            sync.Mutex
}

func NewCommonBackend(name string) *CommonBackend {
	return &CommonBackend{
		name:          name,
		bucketLocks:   make(map[string]*sync.Mutex),
		initedConfigs: make(map[string]struct{}),
	}
}

// Name implements `backends.Backend` interface.
func (backend *CommonBackend) Name() string {
	return backend.name
}

func (backend *CommonBackend) IsVersionControlEnabled(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	config Config,
	opts *Options,
) (bool, error) {
	l.Warnf("Checking version control for %s backend not implemented.", backend.Name())

	return false, nil
}

// NeedsBootstrap implements `backends.NeedsBootstrap` interface.
func (backend *CommonBackend) NeedsBootstrap(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	config Config,
	opts *Options,
) (bool, error) {
	return false, nil
}

// Bootstrap implements `backends.Bootstrap` interface.
func (backend *CommonBackend) Bootstrap(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	config Config,
	opts *Options,
) error {
	l.Warnf("Bootstrap for %s backend not implemented.", backend.Name())

	return nil
}

// Migrate implements `backends.Migrate` interface.
func (backend *CommonBackend) Migrate(
	ctx context.Context,
	l log.Logger,
	srcV, dstV *venv.Venv,
	srcConfig, dstConfig Config,
	opts *Options,
) error {
	l.Warnf("Migrate for %s backend not implemented.", backend.Name())

	return nil
}

// Delete implements `backends.Delete` interface.
func (backend *CommonBackend) Delete(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	config Config,
	opts *Options,
) error {
	l.Warnf("Delete for %s backend not implemented.", backend.Name())

	return nil
}

// DeleteBucket implements `backends.DeleteBucket` interface.
func (backend *CommonBackend) DeleteBucket(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	config Config,
	opts *Options,
) error {
	l.Warnf("Deleting entire bucket for %s backend not implemented.", backend.Name())

	return nil
}

// GetTFInitArgs implements `backends.GetTFInitArgs` interface.
func (backend *CommonBackend) GetTFInitArgs(config Config) map[string]any {
	return config
}

func (backend *CommonBackend) GetBucketMutex(bucketName string) *sync.Mutex {
	backend.mu.Lock()
	defer backend.mu.Unlock()

	bucketMu, ok := backend.bucketLocks[bucketName]
	if !ok {
		bucketMu = new(sync.Mutex)
		backend.bucketLocks[bucketName] = bucketMu
	}

	return bucketMu
}

func (backend *CommonBackend) IsConfigInited(config interface{ CacheKey() string }) bool {
	key := config.CacheKey()

	backend.mu.Lock()
	defer backend.mu.Unlock()

	_, ok := backend.initedConfigs[key]

	return ok
}

func (backend *CommonBackend) MarkConfigInited(config interface{ CacheKey() string }) {
	key := config.CacheKey()

	backend.mu.Lock()
	defer backend.mu.Unlock()

	backend.initedConfigs[key] = struct{}{}
}
