package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/cache/helpers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/terraform/cliconfig"
	"github.com/gruntwork-io/terragrunt/terraform/getproviders"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-getter/v2"
	svchost "github.com/hashicorp/terraform-svchost"
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

func (caches ProviderCaches) FindByRequestID(requestID string) ProviderCaches {
	var foundCaches ProviderCaches

	for _, cache := range caches {
		if util.ListContainsElement(cache.requestIDs, requestID) {
			foundCaches = append(foundCaches, cache)
		}
	}

	return foundCaches
}

func (caches ProviderCaches) removeArchive() error {
	for _, cache := range caches {
		if err := cache.removeArchive(); err != nil {
			return err
		}
	}

	return nil
}

type ProviderCache struct {
	*ProviderService
	*models.Provider
	requestIDs []string

	started            chan struct{}
	documentSHA256Sums []byte
	signature          []byte
	archiveCached      bool
	ready              bool
	err                error

	userProviderDir string
	packageDir      string
	lockfilePath    string
	archivePath     string
}

func (cache *ProviderCache) DocumentSHA256Sums(ctx context.Context) ([]byte, error) {
	if cache.documentSHA256Sums != nil || cache.SHA256SumsURL == "" {
		return cache.documentSHA256Sums, nil
	}

	var documentSHA256Sums = new(bytes.Buffer)

	req, err := cache.newRequest(ctx, cache.SHA256SumsURL)
	if err != nil {
		return nil, err
	}

	if err := helpers.Fetch(ctx, req, documentSHA256Sums); err != nil {
		return nil, fmt.Errorf("failed to retrieve authentication checksums for provider %q: %w", cache.Provider, err)
	}

	cache.documentSHA256Sums = documentSHA256Sums.Bytes()

	return cache.documentSHA256Sums, nil
}

func (cache *ProviderCache) Signature(ctx context.Context) ([]byte, error) {
	if cache.signature != nil || cache.SHA256SumsSignatureURL == "" {
		return cache.signature, nil
	}

	var signature = new(bytes.Buffer)

	req, err := cache.newRequest(ctx, cache.SHA256SumsSignatureURL)
	if err != nil {
		return nil, err
	}

	if err := helpers.Fetch(ctx, req, signature); err != nil {
		return nil, fmt.Errorf("failed to retrieve authentication signature for provider %q: %w", cache.Provider, err)
	}

	cache.signature = signature.Bytes()

	return cache.signature, nil
}

func (cache *ProviderCache) Version() string {
	return cache.Provider.Version
}

func (cache *ProviderCache) Address() string {
	return cache.Provider.Address()
}

func (cache *ProviderCache) PackageDir() string {
	return cache.packageDir
}

func (cache *ProviderCache) AuthenticatePackage(ctx context.Context) (*getproviders.PackageAuthenticationResult, error) {
	var (
		checksum           [sha256.Size]byte
		documentSHA256Sums []byte
		signature          []byte
		err                error
	)

	if documentSHA256Sums, err = cache.DocumentSHA256Sums(ctx); err != nil || documentSHA256Sums == nil {
		return nil, err
	}

	if signature, err = cache.Signature(ctx); err != nil || signature == nil {
		return nil, err
	}

	if _, err := hex.Decode(checksum[:], []byte(cache.SHA256Sum)); err != nil {
		return nil, errors.Errorf("registry response includes invalid SHA256 hash %q for provider %q: %w", cache.SHA256Sum, cache.Provider, err)
	}

	checks := []getproviders.PackageAuthentication{
		getproviders.NewMatchingChecksumAuthentication(documentSHA256Sums, cache.Filename, checksum),
		getproviders.NewArchiveChecksumAuthentication(checksum),
	}

	if len(cache.SigningKeys.Keys()) != 0 {
		checks = append(checks, getproviders.NewSignatureAuthentication(documentSHA256Sums, signature, cache.SigningKeys.Keys()))
	} else {
		// `registry.opentofu.org` does not have signatures for some providers.
		cache.logger.Warnf("Signature validation was skipped due to the registry not containing GPG keys for the provider %s", cache.Provider)
	}

	return getproviders.PackageAuthenticationAll(checks...).Authenticate(cache.archivePath)
}

func (cache *ProviderCache) ArchivePath() string {
	if util.FileExists(cache.archivePath) {
		return cache.archivePath
	}

	return ""
}

func (cache *ProviderCache) addRequestID(requestID string) {
	cache.requestIDs = append(cache.requestIDs, requestID)
}

// warmUp checks if the required provider already exists in the cache directory, if not:
// 1. Checks if the required provider exists in the user plugins directory, located at %APPDATA%\terraform.d\plugins on Windows and ~/.terraform.d/plugins on other systems. If so, creates a symlink to this folder. (Some providers are not available for darwin_arm64, in this case we can use https://github.com/kreuzwerker/m1-terraform-provider-helper which compiles and saves providers to the user plugins directory)
// 2. Downloads the provider from the original registry, unpacks and saves it into the cache directory.
func (cache *ProviderCache) warmUp(ctx context.Context) error {
	if util.FileExists(cache.packageDir) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(cache.packageDir), os.ModePerm); err != nil {
		return errors.New(err)
	}

	if util.FileExists(cache.userProviderDir) {
		cache.logger.Debugf("Create symlink file %s to %s", cache.packageDir, cache.userProviderDir)

		if err := os.Symlink(cache.userProviderDir, cache.packageDir); err != nil {
			return errors.New(err)
		}

		cache.logger.Infof("Cached %s from user plugins directory", cache.Provider)

		return nil
	}

	if cache.DownloadURL == "" {
		return errors.Errorf("not found provider download url")
	}

	if util.FileExists(cache.DownloadURL) {
		cache.archivePath = cache.DownloadURL
	} else {
		if err := util.DoWithRetry(ctx, fmt.Sprintf("Fetching provider %s", cache.Provider), maxRetriesFetchFile, retryDelayFetchFile, cache.logger, log.DebugLevel, func(ctx context.Context) error {
			req, err := cache.newRequest(ctx, cache.DownloadURL)
			if err != nil {
				return err
			}
			return helpers.FetchToFile(ctx, req, cache.archivePath)
		}); err != nil {
			return err
		}

		cache.archiveCached = true
	}

	cache.logger.Debugf("Unpack provider archive %s", cache.archivePath)

	if err := unzip.Decompress(cache.packageDir, cache.archivePath, true, unzipFileMode); err != nil {
		return errors.New(err)
	}

	auth, err := cache.AuthenticatePackage(ctx)
	if err != nil {
		return err
	}

	cache.logger.Infof("Cached %s (%s)", cache.Provider, auth)

	return nil
}

func (cache *ProviderCache) newRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.New(err)
	}

	if cache.credsSource == nil {
		return req, nil
	}

	hostname := svchost.Hostname(req.URL.Hostname())
	if creds := cache.credsSource.ForHost(hostname); creds != nil {
		creds.PrepareRequest(req)
	}

	return req, nil
}

func (cache *ProviderCache) removeArchive() error {
	if cache.archiveCached && util.FileExists(cache.archivePath) {
		cache.logger.Debugf("Remove provider cached archive %s", cache.archivePath)

		if err := os.Remove(cache.archivePath); err != nil {
			return errors.New(err)
		}
	}

	return nil
}

func (cache *ProviderCache) acquireLockFile(ctx context.Context) (*util.Lockfile, error) {
	lockfile := util.NewLockfile(cache.lockfilePath)

	if err := os.MkdirAll(filepath.Dir(cache.lockfilePath), os.ModePerm); err != nil {
		return nil, errors.New(err)
	}

	if err := util.DoWithRetry(ctx, "Acquiring lock file "+cache.lockfilePath, maxRetriesLockFile, retryDelayLockFile, cache.logger, log.DebugLevel, func(ctx context.Context) error {
		return lockfile.TryLock()
	}); err != nil {
		return nil, errors.Errorf("unable to acquire lock file %s (already locked?) try to remove the file manually: %w", cache.lockfilePath, err)
	}

	return lockfile, nil
}

type ProviderService struct {
	// The path to store unpacked providers. The file structure is the same as terraform plugin cache dir.
	cacheDir string

	// The path to a predictable temporary directory for provider archives and lock files.
	tempDir string

	// the user plugins directory, by default: %APPDATA%\terraform.d\plugins on Windows, ~/.terraform.d/plugins on other systems.
	userCacheDir string

	providerCaches        ProviderCaches
	providerCacheWarmUpCh chan *ProviderCache

	cacheMu      sync.RWMutex
	cacheReadyMu sync.RWMutex

	credsSource *cliconfig.CredentialsSource

	logger log.Logger
}

func NewProviderService(cacheDir, userCacheDir string, credsSource *cliconfig.CredentialsSource, logger log.Logger) *ProviderService {
	return &ProviderService{
		cacheDir:              cacheDir,
		userCacheDir:          userCacheDir,
		providerCacheWarmUpCh: make(chan *ProviderCache),
		credsSource:           credsSource,
		logger:                logger,
	}
}

func (service *ProviderService) Logger() log.Logger {
	return service.logger
}

// WaitForCacheReady returns cached providers that were requested by `terraform init` from the cache server, with an  URL containing the given `requestID` value.
// The function returns the value only when all cache requests have been processed.
func (service *ProviderService) WaitForCacheReady(requestID string) ([]getproviders.Provider, error) {
	service.cacheReadyMu.Lock()
	defer service.cacheReadyMu.Unlock()

	var (
		providers []getproviders.Provider
		errs      = &errors.MultiError{}
	)

	for _, provider := range service.providerCaches.FindByRequestID(requestID) {
		if provider.err != nil {
			errs = errs.Append(fmt.Errorf("unable to cache provider: %s, err: %w", provider, provider.err))
		}

		if provider.ready {
			providers = append(providers, provider)
		}
	}

	return providers, errs.ErrorOrNil()
}

// CacheProvider starts caching the given provider using non-blocking approach.
func (service *ProviderService) CacheProvider(ctx context.Context, requestID string, provider *models.Provider) *ProviderCache {
	service.cacheMu.Lock()
	defer service.cacheMu.Unlock()

	if cache := service.providerCaches.Find(provider); cache != nil {
		cache.addRequestID(requestID)
		return cache
	}

	packageName := fmt.Sprintf("%s-%s-%s-%s-%s", provider.RegistryName, provider.Namespace, provider.Name, provider.Version, provider.Platform())

	cache := &ProviderCache{
		ProviderService: service,
		Provider:        provider,
		started:         make(chan struct{}, 1),

		userProviderDir: filepath.Join(service.userCacheDir, provider.Address(), provider.Version, provider.Platform()),
		packageDir:      filepath.Join(service.cacheDir, provider.Address(), provider.Version, provider.Platform()),
		lockfilePath:    filepath.Join(service.tempDir, packageName+".lock"),
		archivePath:     filepath.Join(service.tempDir, packageName+path.Ext(provider.Filename)),
	}

	select {
	case service.providerCacheWarmUpCh <- cache:
		// We need to wait for caching to start and only then release the client (Terraform) requestID. Otherwise, the client may call `WaitForCacheReady()` faster than `service.ReadyMuReady` will be lock.
		<-cache.started
		service.providerCaches = append(service.providerCaches, cache)
	case <-ctx.Done():
	}

	cache.addRequestID(requestID)

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

// Run is responsible to handle a new caching requestID and removing temporary files upon completion.
func (service *ProviderService) Run(ctx context.Context) error {
	if service.cacheDir == "" {
		return errors.Errorf("provider cache directory not specified")
	}

	service.logger.Debugf("Provider cache dir %q", service.cacheDir)

	if err := os.MkdirAll(service.cacheDir, os.ModePerm); err != nil {
		return errors.New(err)
	}

	tempDir, err := util.GetTempDir()
	if err != nil {
		return err
	}

	service.tempDir = filepath.Join(tempDir, "providers")

	errs := &errors.MultiError{}
	errGroup, ctx := errgroup.WithContext(ctx)

	for {
		select {
		case cache := <-service.providerCacheWarmUpCh:
			errGroup.Go(func() error {
				if err := service.startProviderCaching(ctx, cache); err != nil {
					errs = errs.Append(err)
				}

				return nil
			})
		case <-ctx.Done():
			if err := errGroup.Wait(); err != nil {
				errs = errs.Append(err)
			}

			if err := service.providerCaches.removeArchive(); err != nil {
				errs = errs.Append(err)
			}

			return errs.ErrorOrNil()
		}
	}
}

func (service *ProviderService) startProviderCaching(ctx context.Context, cache *ProviderCache) error {
	service.cacheReadyMu.RLock()
	defer service.cacheReadyMu.RUnlock()

	cache.started <- struct{}{}

	// We need to use a locking mechanism between Terragrunt processes to prevent simultaneous write access to the same provider.
	lockfile, err := cache.acquireLockFile(ctx)
	if err != nil {
		return err
	}
	defer lockfile.Unlock() //nolint:errcheck

	if cache.err = cache.warmUp(ctx); cache.err != nil {
		os.Remove(cache.packageDir)  //nolint:errcheck
		os.Remove(cache.archivePath) //nolint:errcheck

		return cache.err
	}

	cache.ready = true

	return nil
}
