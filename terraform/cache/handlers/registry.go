package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/gruntwork-io/terragrunt/terraform/getproviders"
	"github.com/labstack/echo/v4"
)

const (
	// Provider's assets consist of three files/URLs: zipped binary, hashes and signature
	ProviderDownloadURLName         ProviderURLName = "download_url"
	ProviderSHASumsURLName          ProviderURLName = "shasums_url"
	ProviderSHASumsSignatureURLName ProviderURLName = "shasums_signature_url"
)

var (
	// ProviderURLNames contains urls that must be modified to forward terraform requests through this server.
	ProviderURLNames = []ProviderURLName{
		ProviderDownloadURLName,
		ProviderSHASumsURLName,
		ProviderSHASumsSignatureURLName,
	}
)

type ProviderURLName string

type Registry struct {
	*ReverseProxy
	downloadURLPath             string
	providerService             *services.ProviderService
	cacheProviderHTTPStatusCode int
}

func NewRegistry(providerService *services.ProviderService, cacheProviderHTTPStatusCode int) *Registry {
	return &Registry{
		ReverseProxy:                &ReverseProxy{},
		providerService:             providerService,
		cacheProviderHTTPStatusCode: cacheProviderHTTPStatusCode,
	}
}

func (client *Registry) SetDownloadURLPath(urlPath string) {
	client.downloadURLPath = urlPath
}

// GetVersions returns all versions for a single provider
func (client *Registry) GetVersions(ctx echo.Context, provider *models.Provider) error {
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-all-versions-for-a-single-provider
	reqURL := &url.URL{
		Scheme: "https",
		Host:   provider.RegistryName,
		Path:   path.Join("/v1/providers", provider.Namespace, provider.Name, "versions"),
	}

	return client.ReverseProxy.NewRequest(ctx, reqURL)
}

// GetPlatfrom returns a provider for a specific platform
func (client *Registry) GetPlatfrom(ctx echo.Context, provider *models.Provider, cacheRequestID string) error {
	return client.ReverseProxy.
		WithModifyResponse(func(resp *http.Response) error {
			var data map[string]json.RawMessage

			// start caching and return 423 status
			if cacheRequestID != "" {
				var responseBody = new(getproviders.Package)

				if err := DecodeJSONBody(resp, responseBody); err != nil {
					return err
				}
				provider.Package = responseBody

				client.providerService.CacheProvider(ctx.Request().Context(), cacheRequestID, provider)
				return ctx.NoContent(client.cacheProviderHTTPStatusCode)
			}

			// act as a proxy
			return ModifyJSONBody(resp, &data, func() error {
				for _, name := range ProviderURLNames {
					linkBytes, ok := data[string(name)]
					if !ok || linkBytes == nil {
						continue
					}
					link := string(linkBytes)

					link, err := strconv.Unquote(link)
					if err != nil {
						return err
					}

					linkURL, err := url.Parse(link)
					if err != nil {
						return err
					}

					// Modify link to htpp://{localhost_host}/downloads/provider/{remote_host}/{remote_path}
					linkURL.Scheme = ctx.Request().URL.Scheme
					linkURL.Host = ctx.Request().URL.Host
					linkURL.Path = client.downloadURLPath

					link = strconv.Quote(linkURL.String())
					data[string(name)] = []byte(link)
				}

				return nil
			})
		}).
		NewRequest(ctx, provider.PlatformURL())

}

// GetPlatfrom returns a provider for a specific platform
func (client *Registry) Download(ctx echo.Context, downloadURL *url.URL) error {
	provider := models.NewProviderFromDownloadURL(downloadURL.String())

	if cache := client.providerService.GetProviderCache(provider); cache != nil {
		if filename := cache.Filename(); filename != "" {
			log.Debugf("Using cached provider %s", cache.Provider)
			return ctx.File(filename)
		}
	}

	return client.ReverseProxy.NewRequest(ctx, downloadURL)

}
