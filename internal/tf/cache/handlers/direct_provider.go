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
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

	reqURL := resolveProviderURL(apiURLs.ProvidersV1, provider.RegistryName,
		provider.Namespace, provider.Name, "versions")

	versions := struct {
		Versions models.Versions `json:"versions"`
	}{}

	if err := handler.client.Do(ctx, http.MethodGet, reqURL, &versions); err != nil {
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

	reqURLStr := resolveProviderURL(apiURLs.ProvidersV1, provider.RegistryName,
		provider.Namespace, provider.Name, provider.Version, "download", provider.OS, provider.Arch)

	platformURL, err := url.Parse(reqURLStr)
	if err != nil {
		return nil, err
	}

	var resp = new(models.ResponseBody)

	if err := handler.client.Do(ctx, http.MethodGet, reqURLStr, resp); err != nil {
		return nil, err
	}

	resp = resp.ResolveRelativeReferences(platformURL)

	return resp, nil
}

// resolveProviderURL builds a provider API URL. If providersV1 is an absolute URL
// (starts with http:// or https://), it is used as the base. Otherwise, it is
// treated as a relative path on the registry host.
func resolveProviderURL(providersV1, registryName string, pathParts ...string) string {
	subPath := path.Join(pathParts...)

	if strings.HasPrefix(providersV1, "http://") || strings.HasPrefix(providersV1, "https://") {
		// Absolute URL from host block — append path parts directly
		base := strings.TrimRight(providersV1, "/")
		return base + "/" + subPath
	}

	// Relative path — build URL with registry host
	reqURL := &url.URL{
		Scheme: "https",
		Host:   registryName,
		Path:   path.Join(providersV1, subPath),
	}

	return reqURL.String()
}
