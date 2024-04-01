package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-getter/v2"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/terraform/command/cliconfig"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

var (
	unzipFileMode = os.FileMode(0000)

	retryDelayLockFile = time.Second * 5
	maxRetriesLockFile = 60

	retryDelayFetchFile = time.Second * 2
	maxRetriesFetchFile = 30
)

// Borrow the "unpack a zip cache into a target directory" logic from go-getter
var unzip = getter.ZipDecompressor{}

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

	archiveCached bool
	ready         bool
}

func (cache *ProviderCache) providerDir() string {
	return filepath.Join(cache.baseCacheDir, cache.Provider.Path())
}

func (cache *ProviderCache) lockFilename() string {
	return filepath.Join(cache.baseCacheDir, fmt.Sprintf("%s-%s-%s-%s-%s.lock", cache.Provider.RegistryName, cache.Provider.Namespace, cache.Provider.Name, cache.Provider.Version, cache.Platform()))
}

func (cache *ProviderCache) platformDir() string {
	return filepath.Join(cache.baseCacheDir, cache.Provider.Path(), cache.Platform())
}

func (cache *ProviderCache) pluginProviderPlatformDir() string {
	return filepath.Join(cache.terraformPluginDir, cache.Provider.Path(), cache.Platform())
}

func (cache *ProviderCache) ArchiveFilename() string {
	if cache.DownloadURL == nil {
		return ""
	}
	return filepath.Join(cache.baseCacheDir, cache.Provider.Path(), filepath.Base(cache.DownloadURL.Path))
}

// warmUp checks if the binary file already exists in the cache directory, if not, downloads the archive and unzip it.
func (cache *ProviderCache) warmUp(ctx context.Context) error {
	var (
		pluginProviderPlatformDir = cache.pluginProviderPlatformDir()
		platformDir               = cache.platformDir()
		providerDir               = cache.providerDir()
		archiveFilename           = cache.ArchiveFilename()
		needCacheArchive          = cache.needCacheArchive

		alreadyCached bool
	)

	if err := os.MkdirAll(providerDir, os.ModePerm); err != nil {
		return errors.WithStackTrace(err)
	}

	lockfile, err := cache.acquireLockFile(ctx)
	if err != nil {
		return err
	}
	defer lockfile.Unlock() //nolint:errcheck

	if !util.FileExists(platformDir) {
		if util.FileExists(pluginProviderPlatformDir) {
			log.Debugf("Create symlink file %s to %s", platformDir, pluginProviderPlatformDir)
			if err := os.Symlink(pluginProviderPlatformDir, platformDir); err != nil {
				return errors.WithStackTrace(err)
			}
			alreadyCached = true
		} else {
			needCacheArchive = true
		}
	} else {
		alreadyCached = true
	}

	if needCacheArchive && !util.FileExists(archiveFilename) {
		if cache.DownloadURL == nil {
			return errors.Errorf("unable to cache provider %q, the download URL is undefined", cache.Provider)
		}

		if err := util.DoWithRetry(ctx, fmt.Sprintf("Fetching provider with retry %q", cache.Provider), maxRetriesFetchFile, retryDelayFetchFile, logrus.DebugLevel, func() error {
			return util.FetchFile(ctx, cache.DownloadURL.String(), archiveFilename)
		}); err != nil {
			return err
		}
		cache.archiveCached = true
	}

	if !alreadyCached {
		log.Debugf("Decompress provider archive %s", archiveFilename)

		if err := unzip.Decompress(platformDir, archiveFilename, true, unzipFileMode); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	return nil
}

func (cache *ProviderCache) removeArchive() error {
	var archiveFilename = cache.ArchiveFilename()

	if !cache.needCacheArchive && cache.archiveCached && util.FileExists(archiveFilename) {
		log.Debugf("Remove provider cache archive %s", archiveFilename)
		if err := os.Remove(archiveFilename); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	return nil
}

func (cache *ProviderCache) acquireLockFile(ctx context.Context) (*util.Lockfile, error) {
	var (
		lockfileName = cache.lockFilename()
		lockfile     = util.NewLockfile(lockfileName)
	)

	if err := util.DoWithRetry(ctx, fmt.Sprintf("Acquiring lock file with retry %s", lockfileName), maxRetriesLockFile, retryDelayLockFile, logrus.DebugLevel, func() error {
		return lockfile.TryLock()
	}); err != nil {
		return nil, errors.Errorf("unable to acquire lock file %s (already locked?) try removing the file manually: %w", lockfileName, err)
	}

	return lockfile, nil
}

type ProviderService struct {
	baseCacheDir string

	providerCaches        ProviderCaches
	providerCacheWarmUpCh chan *ProviderCache

	cacheMu      sync.RWMutex
	cacheReadyMu sync.RWMutex

	// If needCacheArchive is true, ensures that not only the unarchived binary is cached, but also its archive. We need acrhives in order to reduce the bandwidth, because `terraform lock provider` always loads providers from a remote registry to create a lock file rather than using a cached one. This is only used when opts.ProviderCompleteLock is true.
	needCacheArchive   bool
	terraformPluginDir string
}

func NewProviderService(baseCacheDir string, needCacheArchive bool) *ProviderService {
	return &ProviderService{
		baseCacheDir:          baseCacheDir,
		providerCacheWarmUpCh: make(chan *ProviderCache),
		needCacheArchive:      needCacheArchive,
	}
}

// WaitForCacheReady blocks the call until all providers are cached.
func (service *ProviderService) WaitForCacheReady() {
	service.cacheReadyMu.Lock()
	defer service.cacheReadyMu.Unlock()
}

// CacheProvider starts caching the given provider using non-blocking approch.
func (service *ProviderService) CacheProvider(ctx context.Context, provider *models.Provider) {
	service.cacheMu.Lock()
	defer service.cacheMu.Unlock()

	if cache := service.providerCaches.Find(provider); cache != nil {
		return
	}

	cache := &ProviderCache{
		ProviderService: service,
		Provider:        provider,
	}
	service.providerCaches = append(service.providerCaches, cache)

	select {
	case <-ctx.Done():
	case service.providerCacheWarmUpCh <- cache:
	}
}

// GetProviderCache returns the requested provider archive cache, if it exists.
func (service *ProviderService) GetProviderCache(provider *models.Provider) *ProviderCache {
	service.cacheMu.RLock()
	defer service.cacheMu.RUnlock()

	if cache := service.providerCaches.Find(provider); cache != nil && cache.ready && util.FileExists(cache.ArchiveFilename()) {
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
		case cache := <-service.providerCacheWarmUpCh:
			errGroup.Go(func() error {
				service.cacheReadyMu.RLock()
				defer service.cacheReadyMu.RUnlock()

				if err := cache.warmUp(ctx); err != nil {
					os.RemoveAll(cache.platformDir()) //nolint:errcheck
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
				if err := cache.removeArchive(); err != nil {
					merr = multierror.Append(merr, errors.WithStackTrace(err))
				}
			}

			return merr.ErrorOrNil()
		}
	}
}
