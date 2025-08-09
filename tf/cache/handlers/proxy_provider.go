package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/tf/cliconfig"
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

type ProxyProviderHandler struct {
	*CommonProviderHandler
	*helpers.ReverseProxy
}

func NewProxyProviderHandler(logger log.Logger, credsSource *cliconfig.CredentialsSource) *ProxyProviderHandler {
	return &ProxyProviderHandler{
		CommonProviderHandler: NewCommonProviderHandler(logger, nil, nil),
		ReverseProxy:          &helpers.ReverseProxy{CredsSource: credsSource, Logger: logger},
	}
}

func (handler *ProxyProviderHandler) String() string {
	return "proxy"
}

// GetVersions implements ProviderHandler.GetVersions
// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-all-versions-for-a-single-provider
//
//nolint:lll
func (handler *ProxyProviderHandler) GetVersions(ctx echo.Context, provider *models.Provider) error {
	apiURLs, err := handler.DiscoveryURL(ctx.Request().Context(), provider.RegistryName)
	if err != nil {
		return err
	}

	reqURL := &url.URL{
		Scheme: "https",
		Host:   provider.RegistryName,
		Path:   path.Join(apiURLs.ProvidersV1, provider.Namespace, provider.Name, "versions"),
	}

	return handler.NewRequest(ctx, reqURL)
}

// GetPlatform implements ProviderHandler.GetPlatform
func (handler *ProxyProviderHandler) GetPlatform(ctx echo.Context, provider *models.Provider, downloaderController router.Controller) error {
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
			return modifyDownloadURLsInJSONBody(resp, downloaderController)
		}).
		NewRequest(ctx, platformURL)
}

// Download implements ProviderHandler.Download
func (handler *ProxyProviderHandler) Download(ctx echo.Context, provider *models.Provider) error {
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

		return handler.NewRequest(ctx, downloadURL)
	}

	downloadURL, err := url.Parse(provider.DownloadURL)
	if err != nil {
		return err
	}

	return handler.NewRequest(ctx, downloadURL)
}

// modifyDownloadURLsInJSONBody modifies the response to redirect the download URLs to the local server.
func modifyDownloadURLsInJSONBody(resp *http.Response, downloaderController router.Controller) error {
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

			// Modify link to http://{localhost_host}/downloads/provider/{remote_host}/{remote_path}
			linkURL.Path = path.Join(downloaderController.URL().Path, linkURL.Host, linkURL.Path)
			linkURL.Scheme = downloaderController.URL().Scheme
			linkURL.Host = downloaderController.URL().Host

			link = strconv.Quote(linkURL.String())
			data[string(name)] = []byte(link)
		}

		return nil
	})
}
