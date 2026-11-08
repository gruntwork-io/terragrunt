package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// ProviderDownloadURLName, ProviderSHASumsURLName and ProviderSHASumsSignatureURLName name the three URLs of a
	// provider's assets: zipped binary, hashes and signature.
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

	platformURL, err := ResolveProviderURL(apiURLs.ProvidersV1, provider.RegistryName,
		provider.Namespace, provider.Name, provider.Version, "download", provider.OS, provider.Arch)
	if err != nil {
		return err
	}

	return handler.ReverseProxy.
		WithModifyResponse(func(resp *http.Response) error {
			return modifyDownloadURLsInJSONBody(resp, platformURL, downloaderController)
		}).
		NewRequest(w, r, platformURL)
}

// Download streams the provider archive from the absolute download URL of provider.
func (handler *ProxyProviderHandler) Download(
	w router.ResponseWriter,
	r *http.Request,
	provider *models.Provider,
) error {
	downloadURL, err := url.Parse(provider.DownloadURL)
	if err != nil {
		return err
	}

	return handler.NewRequest(w, r, downloadURL)
}

// modifyDownloadURLsInJSONBody resolves the download URLs in the response against platformURL, as
// [models.ResponseBody.ResolveRelativeReferences] does for a cached provider, and redirects them to the local server.
func modifyDownloadURLsInJSONBody(
	resp *http.Response,
	platformURL *url.URL,
	downloaderController router.Controller,
) error {
	var data map[string]json.RawMessage

	return helpers.ModifyJSONBody(resp, &data, func() error {
		for _, name := range providerURLNames {
			linkBytes, ok := data[string(name)]
			if !ok {
				continue
			}

			var link string
			if err := json.Unmarshal(linkBytes, &link); err != nil {
				return err
			}

			if link == "" {
				continue
			}

			linkURL, err := url.Parse(models.ResolveRelativeReference(platformURL, link))
			if err != nil {
				return err
			}

			rewritten, err := json.Marshal(downloadLink(downloaderController.URL(), linkURL))
			if err != nil {
				return err
			}

			data[string(name)] = rewritten
		}

		return nil
	})
}

// downloadLink returns the URL under downloader that fetches remote:
// {downloader}/{remote_scheme}/{remote_host}{remote_path}?{remote_query}.
//
// The escaped remote path is escaped again: every '%', and every '/' that follows another '/'. The cache server's
// mux redirects a path with repeated slashes to a cleaned one, which would change a signed URL, so the path must
// reach the downloader with none. A single [url.PathUnescape] gives back the escaped remote path.
func downloadLink(downloader, remote *url.URL) string {
	remotePath := strings.ReplaceAll(strings.ReplaceAll(remote.EscapedPath(), "%", "%25"), "//", "/%2F")

	link := downloader.JoinPath(remote.Scheme, remote.Host).String() + remotePath
	if remote.RawQuery != "" {
		link += "?" + remote.RawQuery
	}

	return link
}
