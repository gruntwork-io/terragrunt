package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/registry/models"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/sync/errgroup"
)

var (
	defaultBaseCacheDir = filepath.Join(os.TempDir(), "terragrunt-provider-cache")
	unzipFileMode       = os.FileMode(0000)
)

// We borrow the "unpack a zip cache into a target directory" logic from
// go-getter, even though we're not otherwise using go-getter here.
// (We don't need the same flexibility as we have for modules, because
// providers _always_ come from provider registries, which have a very
// specific protocol and set of expectations.)
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
	*models.Provider
	Filename string

	needCacheArchive bool
	cacheDir         string
	ready            bool
}

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

func (cache *ProviderCache) warmUp(ctx context.Context) error {
	var unpackedFound bool

	if !util.FileExists(cache.cacheDir) {
		if err := os.MkdirAll(cache.cacheDir, os.ModePerm); err != nil {
			return errors.WithStackTrace(err)
		}
	} else {
		entries, err := os.ReadDir(cache.cacheDir)
		if err != nil {
			return errors.WithStackTrace(err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				unpackedFound = true
				break
			}
		}

	}

	if unpackedFound && !cache.needCacheArchive {
		return nil
	}

	if !util.FileExists(cache.Filename) {
		if err := cache.fetch(ctx); err != nil {
			return err
		}
	}

	if !unpackedFound {
		if err := unzip.Decompress(cache.cacheDir, cache.Filename, true, unzipFileMode); err != nil {
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
}

func NewProviderService() *ProviderService {
	return &ProviderService{
		baseCacheDir:    defaultBaseCacheDir,
		providerCacheCh: make(chan *ProviderCache),
	}
}

func (service *ProviderService) SetProviderCacheDir(baseCacheDir string) {
	service.baseCacheDir = baseCacheDir
}

func (service *ProviderService) WaitForCacheReady(ctx context.Context) {
	service.cacheReadyMu.Lock()
	defer service.cacheReadyMu.Unlock()
}

func (service *ProviderService) CacheProvider(provider *models.Provider, needCacheArchive bool) {
	service.cacheMu.Lock()
	defer service.cacheMu.Unlock()

	if cache := service.providerCaches.Find(provider); cache != nil || provider.DownloadURL == nil {
		return
	}

	cache := &ProviderCache{
		Provider:         provider,
		Filename:         filepath.Join(service.baseCacheDir, provider.RegistryName, provider.Namespace, provider.Name, filepath.Base(provider.DownloadURL.Path)),
		cacheDir:         filepath.Join(service.baseCacheDir, provider.RegistryName, provider.Namespace, provider.Name, provider.Version, fmt.Sprintf("%s_%s", provider.OS, provider.Arch)),
		needCacheArchive: needCacheArchive,
	}

	service.providerCacheCh <- cache
	service.providerCaches = append(service.providerCaches, cache)
}

func (service *ProviderService) GetProviderCache(provider *models.Provider) *ProviderCache {
	service.cacheMu.RLock()
	defer service.cacheMu.RUnlock()

	if cache := service.providerCaches.Find(provider); cache != nil && cache.ready && util.FileExists(cache.Filename) {
		return cache
	}
	return nil
}

func (service *ProviderService) RunCacheWorker(ctx context.Context) error {
	if service.baseCacheDir == "" {
		return errors.Errorf("provider cache directory not specified")
	}

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

				log.Infof("Provider %q cached", cache.Provider)
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
