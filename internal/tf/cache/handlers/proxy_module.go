package handlers

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/labstack/echo/v4"
)

// RegistryURLDiscoverer resolves the registry's well-known service endpoints for a given registry name.
type RegistryURLDiscoverer interface {
	DiscoveryURL(ctx context.Context, registryName string) (*RegistryURLs, error)
}

// ProxyModuleHandler proxies module-registry API requests to the upstream registry,
// stripping the inbound cache-server bearer token and re-injecting the user's
// configured credentials for the upstream host.
type ProxyModuleHandler struct {
	*helpers.ReverseProxy

	discoverer RegistryURLDiscoverer
	logger     log.Logger
}

// NewProxyModuleHandler returns a handler that forwards module-registry requests upstream.
func NewProxyModuleHandler(logger log.Logger, credsSource *cliconfig.CredentialsSource, discoverer RegistryURLDiscoverer) *ProxyModuleHandler {
	return &ProxyModuleHandler{
		ReverseProxy: &helpers.ReverseProxy{CredsSource: credsSource, Logger: logger},
		discoverer:   discoverer,
		logger:       logger,
	}
}

// Proxy forwards a module-registry request to the upstream registry.
func (h *ProxyModuleHandler) Proxy(ctx echo.Context, registryName, restPath string) error {
	apiURLs, err := h.discoverer.DiscoveryURL(ctx.Request().Context(), registryName)
	if err != nil {
		return err
	}

	upstream, err := buildModulesUpstreamURL(registryName, apiURLs.ModulesV1, restPath)
	if err != nil {
		return err
	}

	if q := ctx.Request().URL.RawQuery; q != "" {
		upstream.RawQuery = q
	}

	// Drop the inbound cache-server bearer so it can't leak upstream when the
	// user has no credentials configured for the target host. The ReverseProxy
	// then injects the user's upstream credentials based on the target host.
	ctx.Request().Header.Del(echo.HeaderAuthorization)

	return h.ReverseProxy.NewRequest(ctx, upstream)
}

// buildModulesUpstreamURL constructs the upstream URL for a module-registry request.
// If modulesV1 is an absolute URL (contains "://"), it is used as the base.
// Otherwise the URL is built as https://<registryName><modulesV1>.
func buildModulesUpstreamURL(registryName, modulesV1, restPath string) (*url.URL, error) {
	if strings.Contains(modulesV1, "://") {
		base, err := url.Parse(modulesV1)
		if err != nil {
			return nil, fmt.Errorf("parsing modules.v1 URL %q: %w", modulesV1, err)
		}

		base.Path = joinPath(base.Path, restPath)

		return base, nil
	}

	return &url.URL{
		Scheme: "https",
		Host:   registryName,
		Path:   joinPath(modulesV1, restPath),
	}, nil
}

func joinPath(base, rest string) string {
	trailingSlash := strings.HasSuffix(rest, "/")

	joined := path.Join(base, rest)
	if trailingSlash && !strings.HasSuffix(joined, "/") {
		joined += "/"
	}

	return joined
}
