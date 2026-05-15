package handlers

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

var _ ProviderHandler = new(DirectProviderHandler)

type DirectProviderHandler struct {
	*CommonProviderHandler

	client *helpers.Client
}

func NewDirectProviderHandler(l log.Logger, httpClient vhttp.Client, method *cliconfig.ProviderInstallationDirect, credsSource *cliconfig.CredentialsSource) *DirectProviderHandler {
	return &DirectProviderHandler{
		CommonProviderHandler: NewCommonProviderHandler(l, httpClient, method.Include, method.Exclude),
		client:                helpers.NewClient(httpClient, credsSource),
	}
}

func (handler *DirectProviderHandler) String() string {
	return "direct"
}

// GetVersions implements ProviderHandler.GetVersions
// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-all-versions-for-a-single-provider
//
//nolint:lll
func (handler *DirectProviderHandler) GetVersions(ctx context.Context, provider *models.Provider) (models.Versions, error) {
	apiURLs, err := handler.DiscoveryURL(ctx, provider.RegistryName)
	if err != nil {
		return nil, err
	}

	reqURL, err := ResolveProviderURL(apiURLs.ProvidersV1, provider.RegistryName,
		provider.Namespace, provider.Name, "versions")
	if err != nil {
		return nil, err
	}

	versions := struct {
		Versions models.Versions `json:"versions"`
	}{}

	if err := handler.client.Do(ctx, http.MethodGet, reqURL.String(), &versions); err != nil {
		return nil, err
	}

	return versions.Versions, nil
}

// GetPlatform implements ProviderHandler.GetPlatform
func (handler *DirectProviderHandler) GetPlatform(ctx context.Context, provider *models.Provider) (*models.ResponseBody, error) {
	apiURLs, err := handler.DiscoveryURL(ctx, provider.RegistryName)
	if err != nil {
		return nil, err
	}

	platformURL, err := ResolveProviderURL(apiURLs.ProvidersV1, provider.RegistryName,
		provider.Namespace, provider.Name, provider.Version, "download", provider.OS, provider.Arch)
	if err != nil {
		return nil, err
	}

	var resp = new(models.ResponseBody)

	if err := handler.client.Do(ctx, http.MethodGet, platformURL.String(), resp); err != nil {
		return nil, err
	}

	resp = resp.ResolveRelativeReferences(platformURL)

	return resp, nil
}

// ResolveProviderURL builds a provider API URL. If providersV1 is an absolute URL
// (starts with http:// or https://), it is used as the base. Otherwise, it is
// treated as a relative path on the registry host.
func ResolveProviderURL(providersV1, registryName string, pathParts ...string) (*url.URL, error) {
	subPath := path.Join(pathParts...)

	if strings.HasPrefix(providersV1, "http://") || strings.HasPrefix(providersV1, "https://") {
		// Absolute URL from host block — append path parts directly
		base := strings.TrimRight(providersV1, "/")
		raw := base

		if subPath != "" {
			raw = base + "/" + subPath
		}

		u, err := url.Parse(raw)
		if err != nil {
			return nil, err
		}

		return u, nil
	}

	// Relative path — build URL with registry host
	return &url.URL{
		Scheme: "https",
		Host:   registryName,
		Path:   path.Join(providersV1, subPath),
	}, nil
}
