package handlers

import (
	"context"
	"net/http"
	"net/url"
	"path"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/tf/cliconfig"
)

var _ ProviderHandler = new(DirectProviderHandler)

type DirectProviderHandler struct {
	*CommonProviderHandler

	client *helpers.Client
}

func NewDirectProviderHandler(logger log.Logger, method *cliconfig.ProviderInstallationDirect, credsSource *cliconfig.CredentialsSource) *DirectProviderHandler {
	return &DirectProviderHandler{
		CommonProviderHandler: NewCommonProviderHandler(logger, method.Include, method.Exclude),
		client:                helpers.NewClient(credsSource),
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

	reqURL := &url.URL{
		Scheme: "https",
		Host:   provider.RegistryName,
		Path:   path.Join(apiURLs.ProvidersV1, provider.Namespace, provider.Name, "versions"),
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

	platformURL := &url.URL{
		Scheme: "https",
		Host:   provider.RegistryName,
		Path:   path.Join(apiURLs.ProvidersV1, provider.Namespace, provider.Name, provider.Version, "download", provider.OS, provider.Arch),
	}

	var resp = new(models.ResponseBody)

	if err := handler.client.Do(ctx, http.MethodGet, platformURL.String(), resp); err != nil {
		return nil, err
	}

	resp = resp.ResolveRelativeReferences(platformURL)

	return resp, nil
}
