package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/terraform/cache/helpers"
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
	cacheProviderHTTPStatusCode int
}

func NewProviderDirectHandler(providerService *services.ProviderService, cacheProviderHTTPStatusCode int, method *cliconfig.ProviderInstallationDirect, credsSource *cliconfig.CredentialsSource) *ProviderDirectHandler {
	return &ProviderDirectHandler{
		CommonProviderHandler:       NewCommonProviderHandler(providerService, method.Include, method.Exclude),
		ReverseProxy:                &ReverseProxy{CredsSource: credsSource, logger: providerService.Logger()},
		cacheProviderHTTPStatusCode: cacheProviderHTTPStatusCode,
	}
}

func (handler *ProviderDirectHandler) String() string {
	return "direct handler "
}

// GetVersions implements ProviderHandler.GetVersions
// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-all-versions-for-a-single-provider
//
//nolint:lll
func (handler *ProviderDirectHandler) GetVersions(ctx echo.Context, provider *models.Provider) error {
	apiURLs, err := handler.DiscoveryURL(ctx.Request().Context(), provider.RegistryName)
	if err != nil {
		return err
	}

	reqURL := &url.URL{
		Scheme: "https",
		Host:   provider.RegistryName,
		Path:   path.Join(apiURLs.ProvidersV1, provider.Namespace, provider.Name, "versions"),
	}

	return handler.ReverseProxy.NewRequest(ctx, reqURL)
}

// GetPlatform implements ProviderHandler.GetPlatform
func (handler *ProviderDirectHandler) GetPlatform(ctx echo.Context, provider *models.Provider, downloaderController router.Controller, cacheRequestID string) error {
	apiURLs, err := handler.DiscoveryURL(ctx.Request().Context(), provider.RegistryName)
	if err != nil {
		return err
	}

	platformURL := &url.URL{
		Scheme: "https",
		Host:   provider.RegistryName,
		Path:   path.Join(apiURLs.ProvidersV1, provider.Namespace, provider.Name, provider.Version, "download", provider.OS, provider.Arch),
	}

	return handler.ReverseProxy.
		WithModifyResponse(func(resp *http.Response) error {
			// start caching and return 423 status
			if cacheRequestID != "" {
				var body = new(models.ResponseBody)

				if err := helpers.DecodeJSONBody(resp, body); err != nil {
					return err
				}

				provider.ResponseBody = body.ResolveRelativeReferences(resp.Request.URL)

				handler.providerService.CacheProvider(ctx.Request().Context(), cacheRequestID, provider)
				return ctx.NoContent(handler.cacheProviderHTTPStatusCode)
			}

			// act as a proxy
			return proxyGetVersionsRequest(resp, downloaderController)
		}).
		NewRequest(ctx, platformURL)
}

// Download implements ProviderHandler.Download
func (handler *ProviderDirectHandler) Download(ctx echo.Context, provider *models.Provider) error {
	if cache := handler.providerService.GetProviderCache(provider); cache != nil {
		if path := cache.ArchivePath(); path != "" {
			handler.providerService.Logger().Debugf("Download cached provider %s", cache.Provider)
			return ctx.File(path)
		}
	}

	// check if the URL contains http scheme, it may just be a filename and we need to build the URL
	if !strings.Contains(provider.DownloadURL, "://") {
		apiURLs, err := handler.DiscoveryURL(ctx.Request().Context(), provider.RegistryName)
		if err != nil {
			return err
		}

		downloadURL := &url.URL{
			Scheme: "https",
			Host:   provider.RegistryName,
			Path:   filepath.Join(apiURLs.ProvidersV1, provider.RegistryName, provider.Namespace, provider.Name, provider.DownloadURL),
		}

		return handler.ReverseProxy.NewRequest(ctx, downloadURL)
	}

	downloadURL, err := url.Parse(provider.DownloadURL)
	if err != nil {
		return err
	}

	return handler.ReverseProxy.NewRequest(ctx, downloadURL)
}

// proxyGetVersionsRequest proxies the request to the remote registry and modifies the response to redirect the download URLs to the local server.
func proxyGetVersionsRequest(resp *http.Response, downloaderController router.Controller) error {
	var data map[string]json.RawMessage

	return helpers.ModifyJSONBody(resp, &data, func() error {
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
}
