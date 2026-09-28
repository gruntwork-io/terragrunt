package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

func NewProxyProviderHandler(
	l log.Logger,
	c vhttp.Client,
	credsSource *cliconfig.CredentialsSource,
) *ProxyProviderHandler {
	return &ProxyProviderHandler{
		CommonProviderHandler: NewCommonProviderHandler(l, c, nil, nil),
		ReverseProxy: &helpers.ReverseProxy{
			CredsSource: credsSource,
			Logger:      l,
			Transport:   c.Transport,
		},
	}
}

func (handler *ProxyProviderHandler) String() string {
	return "proxy"
}

// GetPlatform forwards a platform request to the provider's registry, rewriting
// the download URLs in the answer to point at downloaderController.
func (handler *ProxyProviderHandler) GetPlatform(
	w router.ResponseWriter,
	r *http.Request,
	provider *models.Provider,
	downloaderController router.Controller,
) error {
	apiURLs, err := handler.DiscoveryURL(r.Context(), provider.RegistryName)
	if err != nil {
		return err
	}

	platformURL := &url.URL{
		Scheme: schemeHTTPS,
		Host:   provider.RegistryName,
		Path: path.Join(
			apiURLs.ProvidersV1,
			provider.Namespace,
			provider.Name,
			provider.Version,
			"download",
			provider.OS,
			provider.Arch,
		),
	}

	return handler.ReverseProxy.
		WithModifyResponse(func(resp *http.Response) error {
			return modifyDownloadURLsInJSONBody(resp, downloaderController)
		}).
		NewRequest(w, r, platformURL)
}

// Download streams the provider archive from its registry. A bare file name
// in the provider's download URL is resolved against the registry's providers
// endpoint.
func (handler *ProxyProviderHandler) Download(w router.ResponseWriter, r *http.Request, provider *models.Provider) error {
	if !strings.Contains(provider.DownloadURL, "://") {
		apiURLs, err := handler.DiscoveryURL(r.Context(), provider.RegistryName)
		if err != nil {
			return err
		}

		downloadURL := &url.URL{
			Scheme: schemeHTTPS,
			Host:   provider.RegistryName,
			Path: filepath.Join(
				apiURLs.ProvidersV1,
				provider.RegistryName,
				provider.Namespace,
				provider.Name,
				provider.DownloadURL,
			),
		}

		return handler.NewRequest(w, r, downloadURL)
	}

	downloadURL, err := url.Parse(provider.DownloadURL)
	if err != nil {
		return err
	}

	return handler.NewRequest(w, r, downloadURL)
}

// modifyDownloadURLsInJSONBody modifies the response to redirect the download URLs to the local server.
func modifyDownloadURLsInJSONBody(
	resp *http.Response,
	downloaderController router.Controller,
) error {
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
