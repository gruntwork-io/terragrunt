package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/registry/models"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/sync/errgroup"
)

var (
	unzipFileMode                      = os.FileMode(0000)
	waitNextAttepmtToLockProviderCache = time.Second * 5
	maxAttemptsToLockProviderCache     = 60 // equals 5 mins (waitNextAttepmtToLockProviderCache * 60)
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

	Filename     string
	filelockName string
	providerDir  string
	ready        bool
}

// fetch downloads the provider archive from the remote/original registry.
func (cache *ProviderCache) fetch(ctx context.Context) error {
	log.Debugf("Fetching provider %q", cache.Provider)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	out, err := os.Create(cache.Filename)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	go func() {
		// Closing os.Stdin will cause io.Copy to return with error "cache already closed" next time it reads from it.
		// This will stop download process when pressing Ctrl-C.
		<-ctx.Done()
		_ = out.Close()
	}()

	resp, err := http.Get(cache.DownloadURL.String())
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

// warmUp checks if the binary file already exists in the cache directory, if not, downloads the archive and unzip it.
func (cache *ProviderCache) warmUp(ctx context.Context) error {
	var unpackedFound bool

	if !util.FileExists(cache.providerDir) {
		if err := os.MkdirAll(cache.providerDir, os.ModePerm); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	log.Debugf("Try to lock provider cache %q", cache.Provider)
	filelock, err := util.AcquireFileLock(ctx, cache.filelockName, maxAttemptsToLockProviderCache, waitNextAttepmtToLockProviderCache)
	if err != nil {
		return err
	}
	log.Debugf("Provider %q cache is locked", cache.Provider)
	defer func() {
		_ = filelock.Unlock()
		log.Debugf("Provider %q cache is released", cache.Provider)
	}()

	entries, err := os.ReadDir(cache.providerDir)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "terraform-provider") {
			unpackedFound = true
			break
		}
	}

	if unpackedFound && !cache.cacheProviderArchive {
		return nil
	}

	if !util.FileExists(cache.Filename) {
		if err := cache.fetch(ctx); err != nil {
			return err
		}
	}

	if !unpackedFound {
		if err := unzip.Decompress(cache.providerDir, cache.Filename, true, unzipFileMode); err != nil {
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

	if cache := service.providerCaches.Find(provider); cache != nil || provider.DownloadURL == nil {
		return
	}

	filename := filepath.Base(provider.DownloadURL.Path)
	providerVersionDir := filepath.Join(service.baseCacheDir, provider.RegistryName, provider.Namespace, provider.Name, provider.Version)

	cache := &ProviderCache{
		ProviderService: service,
		Provider:        provider,
		Filename:        filepath.Join(providerVersionDir, filename),
		filelockName:    filepath.Join(providerVersionDir, fmt.Sprintf("%s_%s.lock", provider.OS, provider.Arch)),
		providerDir:     filepath.Join(providerVersionDir, fmt.Sprintf("%s_%s", provider.OS, provider.Arch)),
	}

	service.providerCacheCh <- cache
	service.providerCaches = append(service.providerCaches, cache)
}

// GetProviderCache returns the requested provider archive cache, if it exists.
func (service *ProviderService) GetProviderCache(provider *models.Provider) *ProviderCache {
	service.cacheMu.RLock()
	defer service.cacheMu.RUnlock()

	if cache := service.providerCaches.Find(provider); cache != nil && cache.ready && util.FileExists(cache.Filename) {
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
				if util.FileExists(cache.Filename) {
					if err := os.Remove(cache.Filename); err != nil {
						merr = multierror.Append(merr, errors.WithStackTrace(err))
					}
				}
			}

			return merr.ErrorOrNil()
		}
	}
}
