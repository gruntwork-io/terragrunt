package handlers

import (
	"encoding/json"
	"fmt"
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
	ProviderDownloadURLName providerURLName = "download_url"
	// ProviderSHASumsURLName is the platform response field with the URL of the SHA256SUMS document.
	ProviderSHASumsURLName providerURLName = "shasums_url"
	// ProviderSHASumsSignatureURLName is the platform response field with the URL of the SHA256SUMS signature.
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

// ProxyProviderHandler forwards provider requests to the registry and rewrites the asset URLs in its answers to point back at the cache server.
type ProxyProviderHandler struct {
	*CommonProviderHandler
	*helpers.ReverseProxy
}

// NewProxyProviderHandler returns a handler that sends requests over the transport of c and authenticates to registries with credsSource.
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

// modifyDownloadURLsInJSONBody points the download URLs of the response at the local server.
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
				return fmt.Errorf("decoding %s in the provider platform response: %w", name, err)
			}

			if link == "" {
				continue
			}

			linkURL, err := models.ResolveRelativeReference(platformURL, link)
			if err != nil {
				return fmt.Errorf("resolving %s in the provider platform response: %w", name, err)
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

// downloadLink points remote at downloader as {scheme}/{host}{path}, escaping the path again and keeping user info.
func downloadLink(downloader, remote *url.URL) string {
	segments := strings.Split(remote.EscapedPath(), "/")
	for i, segment := range segments {
		if segment == "." || segment == ".." {
			segments[i] = strings.Repeat("%2E", len(segment))

			continue
		}

		segments[i] = strings.ReplaceAll(segment, "%", "%25")
	}

	remotePath := strings.ReplaceAll(strings.Join(segments, "/"), "//", "/%2F")

	local := *downloader
	local.User = remote.User

	link := local.JoinPath(remote.Scheme, remote.Host).String() + remotePath
	if remote.RawQuery != "" {
		link += "?" + remote.RawQuery
	}

	return link
}
