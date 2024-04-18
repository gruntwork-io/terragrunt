package services

import (
	"context"
	goerrors "errors"
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
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	unzipFileMode = os.FileMode(0000)

	retryDelayLockFile = time.Second * 5
	maxRetriesLockFile = 60

	retryDelayFetchFile = time.Second * 2
	maxRetriesFetchFile = 5
)

// Borrow the "unpack a zip cache into a target directory" logic from go-getter
var unzip = getter.ZipDecompressor{}

type ProviderCaches []*ProviderCache

func (caches ProviderCaches) Find(target *models.Provider) *ProviderCache {
	for _, cache := range caches {
		if cache.Match(target) {
			return cache
		}
	}

	return nil
}

func (caches ProviderCaches) FindByOwner(owner string) ProviderCaches {
	var foundCaches ProviderCaches

	for _, cache := range caches {
		if util.ListContainsElement(cache.owners, owner) {
			foundCaches = append(foundCaches, cache)
		}
	}

	return foundCaches
}

type ProviderCache struct {
	*models.Provider
	owners []string

	started       chan struct{}
	archiveCached bool
	ready         bool

	userProviderDir string
	providerDir     string
	lockFilename    string
	archiveFilename string
}

func (cache *ProviderCache) ArchiveFilename() string {
	if util.FileExists(cache.archiveFilename) {
		return cache.archiveFilename
	}
	return ""
}

func (cache *ProviderCache) addOwner(owner string) {
	cache.owners = append(cache.owners, owner)
}

// warmUp checks if the required provider already exists in the cache directory, if not:
// 1. Checks if the required provider exists in the user plugins directory, located at %APPDATA%\terraform.d\plugins on Windows and ~/.terraform.d/plugins on other systems. If so, creates a symlink to this folder. (Some providers are not available for darwin_arm64, in this case we can use https://github.com/kreuzwerker/m1-terraform-provider-helper which compiles and saves providers to the user plugins directory)
// 2. Downloads the provider from the original registry, unpacks and saves it into the cache directory.
func (cache *ProviderCache) warmUp(ctx context.Context) error {
	if util.FileExists(cache.providerDir) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(cache.providerDir), os.ModePerm); err != nil {
		return errors.WithStackTrace(err)
	}

	if util.FileExists(cache.userProviderDir) {
		log.Debugf("Create symlink file %s to %s", cache.providerDir, cache.userProviderDir)
		if err := os.Symlink(cache.userProviderDir, cache.providerDir); err != nil {
			return errors.WithStackTrace(err)
		}
		return nil
	}

	if cache.DownloadURL == nil {
		return errors.Errorf("provider download url not found")
	}

	if err := util.DoWithRetry(ctx, fmt.Sprintf("Fetching provider %q", cache.Provider), maxRetriesFetchFile, retryDelayFetchFile, logrus.DebugLevel, func() error {
		return util.FetchFile(ctx, cache.DownloadURL.String(), cache.archiveFilename)
	}); err != nil {
		return err
	}
	cache.archiveCached = true

	log.Debugf("Unpack provider archive %s", cache.archiveFilename)

	if err := unzip.Decompress(cache.providerDir, cache.archiveFilename, true, unzipFileMode); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

func (cache *ProviderCache) removeArchive() error {
	if cache.archiveCached && util.FileExists(cache.archiveFilename) {
		log.Debugf("Remove provider cached archive %s", cache.archiveFilename)
		if err := os.Remove(cache.archiveFilename); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	return nil
}

func (cache *ProviderCache) acquireLockFile(ctx context.Context) (*util.Lockfile, error) {
	lockfile := util.NewLockfile(cache.lockFilename)

	if err := os.MkdirAll(filepath.Dir(cache.lockFilename), os.ModePerm); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if err := util.DoWithRetry(ctx, fmt.Sprintf("Acquiring lock file %s", cache.lockFilename), maxRetriesLockFile, retryDelayLockFile, logrus.DebugLevel, func() error {
		return lockfile.TryLock()
	}); err != nil {
		return nil, errors.Errorf("unable to acquire lock file %s (already locked?) try to remove the file manually: %w", cache.lockFilename, err)
	}

	return lockfile, nil
}

type ProviderService struct {
	// The path to store unpacked providers. The file structure is the same as terraform plugin cache dir.
	baseCacheDir string

	// The path to store archive providers that are retrieved from the source registry and cached to reduce traffic.
	baseArchiveDir string

	// the user plugins directory, by default: %APPDATA%\terraform.d\plugins on Windows, ~/.terraform.d/plugins on other systems.
	baseUserCacheDir string

	providerCaches        ProviderCaches
	providerCacheWarmUpCh chan *ProviderCache

	cacheMu      sync.RWMutex
	cacheReadyMu sync.RWMutex
}

func NewProviderService(baseCacheDir, baseArchiveDir, baseUserCacheDir string) *ProviderService {
	return &ProviderService{
		baseCacheDir:          baseCacheDir,
		baseUserCacheDir:      baseUserCacheDir,
		baseArchiveDir:        baseArchiveDir,
		providerCacheWarmUpCh: make(chan *ProviderCache),
	}
}

// WaitForCacheReady blocks the call until all providers are cached.
func (service *ProviderService) WaitForCacheReady(owner string) ProviderCaches {
	service.cacheReadyMu.Lock()
	defer service.cacheReadyMu.Unlock()

	return service.providerCaches.FindByOwner(owner)
}

// CacheProvider starts caching the given provider using non-blocking approach.
func (service *ProviderService) CacheProvider(ctx context.Context, owner string, provider *models.Provider) *ProviderCache {
	service.cacheMu.Lock()
	defer service.cacheMu.Unlock()

	if cache := service.providerCaches.Find(provider); cache != nil {
		cache.addOwner(owner)
		return cache
	}

	packageName := fmt.Sprintf("%s-%s-%s-%s-%s", provider.RegistryName, provider.Namespace, provider.Name, provider.Version, provider.Platform())

	cache := &ProviderCache{
		Provider: provider,
		started:  make(chan struct{}, 1),

		userProviderDir: filepath.Join(service.baseUserCacheDir, provider.Path(), provider.Platform()),
		providerDir:     filepath.Join(service.baseCacheDir, provider.Path(), provider.Platform()),
		lockFilename:    filepath.Join(service.baseArchiveDir, fmt.Sprintf("%s.lock", packageName)),
	}

	if provider.DownloadURL != nil {
		cache.archiveFilename = filepath.Join(service.baseArchiveDir, fmt.Sprintf("%s%s", packageName, path.Ext(provider.DownloadURL.String())))
	}

	select {
	case service.providerCacheWarmUpCh <- cache:
		// We need to wait for caching to start and only then release the client (Terraform) request. Otherwise, the client may call `WaitForCacheReady()` faster than `service.ReadyMuReady` will be lock.
		<-cache.started
		service.providerCaches = append(service.providerCaches, cache)
	case <-ctx.Done():
		// quit
	}

	cache.addOwner(owner)
	return cache
}

// GetProviderCache returns the requested provider archive cache, if it exists.
func (service *ProviderService) GetProviderCache(provider *models.Provider) *ProviderCache {
	service.cacheMu.RLock()
	defer service.cacheMu.RUnlock()

	if cache := service.providerCaches.Find(provider); cache != nil && cache.ready {
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

	errGroup, ctx := errgroup.WithContext(ctx)
	for {
		select {
		case cache := <-service.providerCacheWarmUpCh:
			errGroup.Go(func() error {
				service.cacheReadyMu.RLock()
				defer service.cacheReadyMu.RUnlock()

				cache.started <- struct{}{}

				// We need to use a locking mechanism between Terragrunt processes to prevent simultaneous write access to the same provider.
				lockfile, err := cache.acquireLockFile(ctx)
				if err != nil {
					return err
				}
				defer lockfile.Unlock() //nolint:errcheck

				if err := cache.warmUp(ctx); err != nil {
					os.Remove(cache.providerDir)     //nolint:errcheck
					os.Remove(cache.archiveFilename) //nolint:errcheck
					return err
				}

				cache.ready = true
				log.Infof("Provider %q is cached", cache.Provider)

				return nil
			})
		case <-ctx.Done():
			merr := &multierror.Error{}

			if err := errGroup.Wait(); err != nil && !goerrors.Is(err, context.Canceled) {
				merr = multierror.Append(merr, err)
			}

			for _, cache := range service.providerCaches {
				close(cache.started)

				if err := cache.removeArchive(); err != nil && !goerrors.Is(err, context.Canceled) {
					merr = multierror.Append(merr, errors.WithStackTrace(err))
				}
			}

			return merr.ErrorOrNil()
		}
	}
}
