package services

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/registry/models"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/terraform/command/cliconfig"
	"golang.org/x/sync/errgroup"
)

var (
	unzipFileMode                      = os.FileMode(0000)
	waitNextAttepmtToLockProviderCache = time.Second * 5
	maxAttemptsToLockProviderCache     = 60 // equals 5 mins
)

// Borrow the "unpack a zip cache into a target directory" logic from
// go-getter
var unzip = getter.ZipDecompressor{}

// Borrow the "unpack a zip cache into a target directory" logic from go-getter
type ProviderCaches []*ProviderCache

func (providers ProviderCaches) Find(target *models.Provider) *ProviderCache {
	for _, provider := range providers {
		if provider.Match(target) {
			return provider
		}
	}

	return nil
}

type ProviderCache struct {
	*ProviderService
	*models.Provider

	ready bool
}

func (cache *ProviderCache) providerDir() string {
	return filepath.Join(cache.baseCacheDir, cache.Provider.Path())
}

func (cache *ProviderCache) filelockName() string {
	return filepath.Join(cache.baseCacheDir, cache.Provider.Path(), cache.Platform()) + ".lock"
}

func (cache *ProviderCache) platformDir() string {
	return filepath.Join(cache.baseCacheDir, cache.Provider.Path(), cache.Platform())
}

func (cache *ProviderCache) platformPluginDir() string {
	return filepath.Join(cache.terraformPluginDir, cache.Provider.Path(), cache.Platform())
}

func (cache *ProviderCache) Archive() string {
	if cache.DownloadURL == nil {
		return ""
	}
	return filepath.Join(cache.baseCacheDir, cache.Provider.Path(), filepath.Base(cache.DownloadURL.Path))
}

// warmUp checks if the binary file already exists in the cache directory, if not, downloads the archive and unzip it.
func (cache *ProviderCache) warmUp(ctx context.Context) error {
	var unpackedFound bool

	if err := os.MkdirAll(cache.providerDir(), os.ModePerm); err != nil {
		return errors.WithStackTrace(err)
	}

	filelock, err := util.AcquireFileLock(ctx, cache.filelockName(), maxAttemptsToLockProviderCache, waitNextAttepmtToLockProviderCache)
	if err != nil {
		return err
	}
	log.Tracef("Provider %q cache is locked", cache.Provider)
	defer func() {
		_ = filelock.Unlock()
		log.Tracef("Provider %q cache is released", cache.Provider)
	}()

	if platformDir := cache.platformDir(); !util.FileExists(platformDir) {
		if platformPluginDir := cache.platformPluginDir(); util.FileExists(platformPluginDir) {
			if err := os.Symlink(platformPluginDir, platformDir); err != nil {
				return errors.WithStackTrace(err)
			}
			unpackedFound = true
		} else {
			if err := os.MkdirAll(platformDir, os.ModePerm); err != nil {
				return errors.WithStackTrace(err)
			}
		}
	} else {
		unpackedFound = true
	}

	if (unpackedFound && !cache.cacheProviderArchive) || cache.DownloadURL == nil {
		return nil
	}

	if filename := cache.Archive(); !util.FileExists(filename) {
		log.Debugf("Fetching provider %q", cache.Provider)
		if err := util.FetchFile(ctx, cache.DownloadURL.String(), filename); err != nil {
			return err
		}
	}

	if !unpackedFound {
		if err := unzip.Decompress(cache.platformDir(), cache.Archive(), true, unzipFileMode); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	return nil
}

type ProviderService struct {
	baseCacheDir string

	providerCaches  ProviderCaches
	providerCacheCh chan *ProviderCache

	cacheMu      sync.RWMutex
	cacheReadyMu sync.RWMutex

	// If needCacheArchive is true, ensures that not only the unarchived binary is cached, but also its archive. We need acrhives in order to reduce the bandwidth, because `terraform lock provider` always loads providers from a remote registry to create a lock file rather than using a cached one. This is only used when opts.ProviderCompleteLock is true.
	cacheProviderArchive bool
	terraformPluginDir   string
}

func NewProviderService(baseCacheDir string, cacheProviderArchive bool) *ProviderService {
	return &ProviderService{
		baseCacheDir:         baseCacheDir,
		providerCacheCh:      make(chan *ProviderCache),
		cacheProviderArchive: cacheProviderArchive,
	}
}

// WaitForCacheReady blocks the call until all providers are cached.
func (service *ProviderService) WaitForCacheReady() {
	service.cacheReadyMu.Lock()
	defer service.cacheReadyMu.Unlock()
}

// CacheProvider starts caching the given provider using non-blocking approch.
func (service *ProviderService) CacheProvider(provider *models.Provider) {
	service.cacheMu.Lock()
	defer service.cacheMu.Unlock()

	if cache := service.providerCaches.Find(provider); cache != nil {
		return
	}

	cache := &ProviderCache{
		ProviderService: service,
		Provider:        provider,
	}

	service.providerCacheCh <- cache
	service.providerCaches = append(service.providerCaches, cache)
}

// GetProviderCache returns the requested provider archive cache, if it exists.
func (service *ProviderService) GetProviderCache(provider *models.Provider) *ProviderCache {
	service.cacheMu.RLock()
	defer service.cacheMu.RUnlock()

	if cache := service.providerCaches.Find(provider); cache != nil && cache.ready && util.FileExists(cache.Archive()) {
		return cache
	}
	return nil
}

// RunCacheWorker is responsible to handle a new caching request and removing temporary files upon completion.
func (service *ProviderService) RunCacheWorker(ctx context.Context) error {
	if service.baseCacheDir == "" {
		return errors.Errorf("provider cache directory not specified")
	}
	log.Debugf("Provider cache dir %q", service.baseCacheDir)

	configDir, err := cliconfig.ConfigDir()
	if err != nil {
		return errors.WithStackTrace(err)
	}
	service.terraformPluginDir = filepath.Join(configDir, "plugins")

	errGroup, ctx := errgroup.WithContext(ctx)
	for {
		select {
		case cache := <-service.providerCacheCh:
			errGroup.Go(func() error {
				service.cacheReadyMu.RLock()
				defer service.cacheReadyMu.RUnlock()

				if err := cache.warmUp(ctx); err != nil {
					return err
				}
				cache.ready = true

				log.Infof("Provider %q is cached", cache.Provider)
				return nil
			})
		case <-ctx.Done():
			merr := &multierror.Error{}

			if err := errGroup.Wait(); err != nil {
				merr = multierror.Append(merr, err)
			}

			for _, cache := range service.providerCaches {
				for _, filename := range []string{cache.Archive()} {
					if util.FileExists(filename) {
						if err := os.Remove(filename); err != nil {
							merr = multierror.Append(merr, errors.WithStackTrace(err))
						}
					}
				}
			}

			return merr.ErrorOrNil()
		}
	}
}
