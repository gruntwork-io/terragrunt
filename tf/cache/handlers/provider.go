// Package handlers provides the interfaces and common implementations for handling provider requests.
package handlers

import (
	"context"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/tf/cliconfig"
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

func NewProviderHandlers(cliCfg *cliconfig.Config, logger log.Logger, registryNames []string) (ProviderHandlers, error) {
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
			providerHandlers = append(providerHandlers, NewFilesystemMirrorProviderHandler(logger, method))
		case *cliconfig.ProviderInstallationNetworkMirror:
			networkMirrorHandler, err := NewNetworkMirrorProviderHandler(logger, method, cliCfg.CredentialsSource())
			if err != nil {
				return nil, err
			}

			providerHandlers = append(providerHandlers, networkMirrorHandler)
		case *cliconfig.ProviderInstallationDirect:
			providerHandlers = append(providerHandlers, NewDirectProviderHandler(logger, method, cliCfg.CredentialsSource()))
			directIsDefined = true
		}

		method.AppendExclude(excludeAddrs)
	}

	if !directIsDefined {
		// In a case if none of direct provider installation methods `cliCfg.ProviderInstallation.Methods` are specified.
		providerHandlers = append(providerHandlers, NewDirectProviderHandler(logger, new(cliconfig.ProviderInstallationDirect), cliCfg.CredentialsSource()))
	}

	return providerHandlers, nil
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
