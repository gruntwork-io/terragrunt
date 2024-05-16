package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/gruntwork-io/terragrunt/terraform/cliconfig"
	"github.com/labstack/echo/v4"
)

const (
	// Provider's assets consist of three files/URLs: zipped binary, hashes and signature
	ProviderDownloadURLName         providerURLName = "download_url"
	ProviderSHASumsURLName          providerURLName = "shasums_url"
	ProviderSHASumsSignatureURLName providerURLName = "shasums_signature_url"
)

var (
	// providerURLNames contains urls that must be modified to forward terraform requests through this server.
	providerURLNames = []providerURLName{
		ProviderDownloadURLName,
		ProviderSHASumsURLName,
		ProviderSHASumsSignatureURLName,
	}
)

type providerURLName string

type ProviderDirectHandler struct {
	*CommonProviderHandler

	*ReverseProxy
	providerService             *services.ProviderService
	cacheProviderHTTPStatusCode int
}

func NewProviderDirectHandler(providerService *services.ProviderService, cacheProviderHTTPStatusCode int, method *cliconfig.ProviderInstallationDirect) ProviderHandler {
	return &ProviderDirectHandler{
		CommonProviderHandler:       NewCommonProviderHandler(method.Include, method.Exclude),
		ReverseProxy:                &ReverseProxy{},
		providerService:             providerService,
		cacheProviderHTTPStatusCode: cacheProviderHTTPStatusCode,
	}
}

func (handler *ProviderDirectHandler) String() string {
	return "direct handler "
}

// GetVersions implements ProviderHandler.GetVersions
func (handler *ProviderDirectHandler) GetVersions(ctx echo.Context, provider *models.Provider) error {
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-all-versions-for-a-single-provider
	reqURL := &url.URL{
		Scheme: "https",
		Host:   provider.RegistryName,
		Path:   path.Join("/v1/providers", provider.Namespace, provider.Name, "versions"),
	}

	return handler.ReverseProxy.NewRequest(ctx, reqURL)
}

// GetPlatfrom implements ProviderHandler.GetPlatfrom
func (handler *ProviderDirectHandler) GetPlatfrom(ctx echo.Context, provider *models.Provider, downloaderController router.Controller, cacheRequestID string) error {
	return handler.ReverseProxy.
		WithModifyResponse(func(resp *http.Response) error {
			var data map[string]json.RawMessage

			// start caching and return 423 status
			if cacheRequestID != "" {
				var body = new(models.ResponseBody)

				if err := DecodeJSONBody(resp, body); err != nil {
					return err
				}
				provider.ResponseBody = body

				handler.providerService.CacheProvider(ctx.Request().Context(), cacheRequestID, provider)
				return ctx.NoContent(handler.cacheProviderHTTPStatusCode)
			}

			// act as a proxy
			return ModifyJSONBody(resp, &data, func() error {
				for _, name := range providerURLNames {
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
					linkURL.Path = path.Join(downloaderController.URL().Path, linkURL.Host, linkURL.Path)
					linkURL.Scheme = downloaderController.URL().Scheme
					linkURL.Host = downloaderController.URL().Host

					link = strconv.Quote(linkURL.String())
					data[string(name)] = []byte(link)
				}

				return nil
			})
		}).
		NewRequest(ctx, handler.platformURL(provider))

}

// Download implements ProviderHandler.Download
func (handler *ProviderDirectHandler) Download(ctx echo.Context, provider *models.Provider) error {
	if cache := handler.providerService.GetProviderCache(provider); cache != nil {
		if path := cache.ArchivePath(); path != "" {
			log.Debugf("Download cached provider %s", cache.Provider)
			return ctx.File(path)
		}
	}

	downloadURL, err := url.Parse(provider.DownloadURL)
	if err != nil {
		return err
	}
	return handler.ReverseProxy.NewRequest(ctx, downloadURL)

}

// platformURL returns the URL used to query the all platforms for a single version.
// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-a-platform
func (handler *ProviderDirectHandler) platformURL(provider *models.Provider) *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   provider.RegistryName,
		Path:   path.Join("/v1/providers", provider.Namespace, provider.Name, provider.Version, "download", provider.OS, provider.Arch),
	}
}
