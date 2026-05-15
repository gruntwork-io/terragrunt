// Package handlers provides the interfaces and common implementations for handling provider requests.
package handlers

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

var availablePlatforms []*models.Platform = []*models.Platform{
	{OS: "solaris", Arch: "amd64"},
	{OS: "openbsd", Arch: "386"},
	{OS: "openbsd", Arch: "arm"},
	{OS: "openbsd", Arch: "amd64"},
	{OS: "freebsd", Arch: "386"},
	{OS: "freebsd", Arch: "arm"},
	{OS: "freebsd", Arch: "amd64"},
	{OS: "linux", Arch: "386"},
	{OS: "linux", Arch: "arm"},
	{OS: "linux", Arch: "arm64"},
	{OS: "linux", Arch: "amd64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
	{OS: "windows", Arch: "386"},
	{OS: "windows", Arch: "amd64"},
}

// ProviderHandlers is a slice of ProviderHandler.
type ProviderHandlers []ProviderHandler

// NewProviderHandlers constructs the slice of [ProviderHandler]s described by
// cliCfg's provider_installation block. httpClient is threaded into every
// handler that issues outbound HTTP (direct, network mirror, proxy, and the
// service-discovery cache shared by all of them). env is the venv-mediated
// shell environment used to resolve TF_TOKEN_<host> credentials.
func NewProviderHandlers(cliCfg *cliconfig.Config, l log.Logger, httpClient vhttp.Client, env map[string]string, registryNames []string) (ProviderHandlers, error) {
	var (
		providerHandlers = make([]ProviderHandler, 0, len(cliCfg.ProviderInstallation.Methods))
		excludeAddrs     = make([]string, 0, len(cliCfg.ProviderInstallation.Methods))
		directIsDefined  bool
	)

	for _, registryName := range registryNames {
		excludeAddrs = append(excludeAddrs, registryName+"/*/*")
	}

	for _, method := range cliCfg.ProviderInstallation.Methods {
		switch method := method.(type) {
		case *cliconfig.ProviderInstallationFilesystemMirror:
			providerHandlers = append(providerHandlers, NewFilesystemMirrorProviderHandler(l, httpClient, method))
		case *cliconfig.ProviderInstallationNetworkMirror:
			networkMirrorHandler, err := NewNetworkMirrorProviderHandler(l, httpClient, method, cliCfg.CredentialsSource(env))
			if err != nil {
				return nil, err
			}

			providerHandlers = append(providerHandlers, networkMirrorHandler)
		case *cliconfig.ProviderInstallationDirect:
			providerHandlers = append(providerHandlers, NewDirectProviderHandler(l, httpClient, method, cliCfg.CredentialsSource(env)))
			directIsDefined = true
		}

		method.AppendExclude(excludeAddrs)
	}

	if !directIsDefined {
		// In a case if none of direct provider installation methods `cliCfg.ProviderInstallation.Methods` are specified.
		providerHandlers = append(providerHandlers, NewDirectProviderHandler(l, httpClient, new(cliconfig.ProviderInstallationDirect), cliCfg.CredentialsSource(env)))
	}

	return providerHandlers, nil
}

// SetDiscoveryURLCache pre-populates the discovery URL cache for all handlers
// that can handle the given registryName. This is used for custom host blocks
// where service URLs are already known and .well-known discovery is not available.
func (handlers ProviderHandlers) SetDiscoveryURLCache(registryName string, urls *RegistryURLs) {
	for _, handler := range handlers {
		if direct, ok := handler.(*DirectProviderHandler); ok {
			direct.CommonProviderHandler.SetDiscoveryURLCache(registryName, urls)
		}
	}
}

// DiscoveryURL looks for the first handler that can handle the given `registryName`,
// which is determined by the include and exclude settings in the `.terraformrc` CLI config file.
// If the handler is found, tries to discover its API endpoints otherwise return the default registry URLs.
func (handlers ProviderHandlers) DiscoveryURL(ctx context.Context, registryName string) (*RegistryURLs, error) {
	provider := models.ParseProvider(registryName)

	for _, handler := range handlers {
		if handler.CanHandleProvider(provider) {
			return handler.DiscoveryURL(ctx, registryName)
		}
	}

	return DefaultRegistryURLs, nil
}

type ProviderHandler interface {
	// CanHandleProvider returns true if the given provider can be handled by this handler.
	CanHandleProvider(provider *models.Provider) bool

	// GetVersions serves a request that returns all versions for a single provider.
	GetVersions(ctx context.Context, provider *models.Provider) (models.Versions, error)

	// GetPlatform serves a request that returns a provider for a specific platform.
	GetPlatform(ctx context.Context, provider *models.Provider) (*models.ResponseBody, error)

	// DiscoveryURL discovers modules and providers API endpoints for the specified `registryName`.
	// https://developer.hashicorp.com/terraform/internals/remote-service-discovery#discovery-process
	DiscoveryURL(ctx context.Context, registryName string) (*RegistryURLs, error)
}
