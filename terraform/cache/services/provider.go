package services

import (
	"context"
	"fmt"
	"os"
	"path"
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
	return filepath.Join(cache.baseCacheDir, cache.Provider.Path(), cache.Platform())
}

func (cache *ProviderCache) pluginProviderDir() string {
	return filepath.Join(cache.terraformPluginDir, cache.Provider.Path(), cache.Platform())
}

func (cache *ProviderCache) lockFilename() string {
	return filepath.Join(cache.baseArchiveDir, cache.Provider.Filename()+".lock")
}

func (cache *ProviderCache) downloadURL() string {
	if cache.DownloadURL == nil {
		return ""
	}
	return cache.DownloadURL.String()
}

func (cache *ProviderCache) ArchiveFilename() string {
	return filepath.Join(cache.baseArchiveDir, cache.Provider.Filename()+path.Ext(cache.downloadURL()))
}

// warmUp checks if the binary file already exists in the cache directory, if not, downloads the archive and unzip it.
func (cache *ProviderCache) warmUp(ctx context.Context) error {
	var (
		pluginProviderDir = cache.pluginProviderDir()
		providerDir       = cache.providerDir()
		downloadURL       = cache.downloadURL()
		archiveFilename   = cache.ArchiveFilename()
		needCacheArchive  = cache.needCacheArchive

		unpackedCached bool
	)

	if err := os.MkdirAll(filepath.Dir(providerDir), os.ModePerm); err != nil {
		return errors.WithStackTrace(err)
	}

	if !util.FileExists(providerDir) {
		if util.FileExists(pluginProviderDir) {
			log.Debugf("Create symlink file %s to %s", providerDir, pluginProviderDir)
			if err := os.Symlink(pluginProviderDir, providerDir); err != nil {
				return errors.WithStackTrace(err)
			}
			unpackedCached = true
		} else {
			needCacheArchive = true
		}
	} else {
		unpackedCached = true
	}

	if needCacheArchive && downloadURL != "" && !util.FileExists(archiveFilename) {
		if err := util.DoWithRetry(ctx, fmt.Sprintf("Fetching provider %q", cache.Provider), maxRetriesFetchFile, retryDelayFetchFile, logrus.DebugLevel, func() error {
			return util.FetchFile(ctx, downloadURL, archiveFilename)
		}); err != nil {
			os.Remove(archiveFilename) //nolint:errcheck
			return err
		}
		cache.archiveCached = true
	}

	if !unpackedCached {
		log.Debugf("Unpack provider archive %s", archiveFilename)

		if err := unzip.Decompress(providerDir, archiveFilename, true, unzipFileMode); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	return nil
}

func (cache *ProviderCache) removeArchive() error {
	var archiveFilename = cache.ArchiveFilename()

	if !cache.needCacheArchive && cache.archiveCached && util.FileExists(archiveFilename) {
		log.Debugf("Remove provider cached archive %s", archiveFilename)
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

	if err := os.MkdirAll(filepath.Dir(lockfileName), os.ModePerm); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if err := util.DoWithRetry(ctx, fmt.Sprintf("Acquiring lock file %s", lockfileName), maxRetriesLockFile, retryDelayLockFile, logrus.DebugLevel, func() error {
		return lockfile.TryLock()
	}); err != nil {
		return nil, errors.Errorf("unable to acquire lock file %s (already locked?) try to remove the file manually: %w", lockfileName, err)
	}

	return lockfile, nil
}

type ProviderService struct {
	// The path to store unpacked providers. The file structure is the same as terraform plugin cache dir.
	baseCacheDir string

	// The path to store archive providers that are retrieved from the source registry and cached to reduce traffic.
	baseArchiveDir string

	providerCaches        ProviderCaches
	providerCacheWarmUpCh chan *ProviderCache

	cacheMu      sync.RWMutex
	cacheReadyMu sync.RWMutex

	// If needCacheArchive is true, ensures that not only the unarchived binary is cached, but also its archive. We need acrhives in order to reduce the bandwidth, because `terraform lock provider` always loads providers from a remote registry to create a lock file rather than using a cached one. This is only used when opts.ProviderCompleteLock is true.
	needCacheArchive   bool
	terraformPluginDir string
}

func NewProviderService(baseCacheDir, baseArchiveDir string, needCacheArchive bool) *ProviderService {
	return &ProviderService{
		baseCacheDir:          baseCacheDir,
		baseArchiveDir:        baseArchiveDir,
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

	if service.baseArchiveDir == "" {
		return errors.Errorf("provider archive directory not specified")
	}
	log.Debugf("Provider archive dir %q", service.baseArchiveDir)

	if service.baseCacheDir == service.baseArchiveDir {
		// We can only store uncompressed provider files in `baseCacheDir` because tofu considers any files there as providers.
		// https://github.com/opentofu/opentofu/blob/bdab86962fdd0a2106a744d7f8f1d3d3e7bc893e/internal/getproviders/filesystem_search.go#L27
		return errors.Errorf("the same directory is used for both unarchived and archived provider files")
	}

	if err := os.MkdirAll(service.baseCacheDir, os.ModePerm); err != nil {
		return errors.WithStackTrace(err)
	}

	if err := os.MkdirAll(service.baseArchiveDir, os.ModePerm); err != nil {
		return errors.WithStackTrace(err)
	}

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

				lockfile, err := cache.acquireLockFile(ctx)
				if err != nil {
					return err
				}
				defer lockfile.Unlock() //nolint:errcheck

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
				if err := cache.removeArchive(); err != nil {
					merr = multierror.Append(merr, errors.WithStackTrace(err))
				}
			}

			return merr.ErrorOrNil()
		}
	}
}
