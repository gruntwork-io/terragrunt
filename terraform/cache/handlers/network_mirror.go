package handlers

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/gruntwork-io/terragrunt/terraform/getproviders"
	"github.com/labstack/echo/v4"
)

type NetworkMirror struct {
	*http.Client
	providerService             *services.ProviderService
	cacheProviderHTTPStatusCode int
}

func NewNetworkMirror(providerService *services.ProviderService, cacheProviderHTTPStatusCode int) *NetworkMirror {
	return &NetworkMirror{
		Client:                      &http.Client{},
		providerService:             providerService,
		cacheProviderHTTPStatusCode: cacheProviderHTTPStatusCode,
	}
}

// GetVersions returns all versions for a single provider
func (client *NetworkMirror) GetVersions(ctx echo.Context, networkMirrorURL string, provider *models.Provider) error {
	reqURL := fmt.Sprintf("%s/%s", strings.TrimRight(networkMirrorURL, "/"), path.Join(provider.RegistryName, provider.Namespace, provider.Name, "index.json"))

	var respData struct {
		Versions map[string]struct{} `json:"versions"`
	}
	if err := client.do(ctx, http.MethodGet, reqURL, &respData); err != nil {
		return err
	}

	versions := struct {
		ID       string                 `json:"id"`
		Versions []getproviders.Version `json:"versions"`
	}{
		ID: path.Join(provider.RegistryName, provider.Namespace, provider.Name),
	}

	for version, _ := range respData.Versions {
		versions.Versions = append(versions.Versions, getproviders.Version{
			Version:   version,
			Platforms: getproviders.AvailablePlatforms,
		})
	}

	return ctx.JSON(http.StatusOK, versions)
}

// GetPlatfrom returns a provider for a specific platform
func (client *NetworkMirror) GetPlatfrom(ctx echo.Context, networkMirrorURL string, provider *models.Provider, cacheRequestID string) error {
	if cacheRequestID == "" {
		// it is impossible to return all platform properties from a network mirror, return 404 status
		return ctx.NoContent(http.StatusNotFound)
	}

	reqURL := fmt.Sprintf("%s/%s", strings.TrimRight(networkMirrorURL, "/"), path.Join(provider.RegistryName, provider.Namespace, provider.Name, provider.Version+".json"))

	var respData struct {
		Archives map[string]struct {
			URL    string   `json:"url"`
			Hashes []string `json:"hashes"`
		} `json:"archives"`
	}
	if err := client.do(ctx, http.MethodGet, reqURL, &respData); err != nil {
		return err
	}

	if archive, ok := respData.Archives[provider.Platform()]; ok {
		provider.Package = &getproviders.Package{DownloadURL: archive.URL}
	}

	// start caching and return 423 status
	client.providerService.CacheProvider(ctx.Request().Context(), cacheRequestID, provider)
	return ctx.NoContent(client.cacheProviderHTTPStatusCode)
}

func (client *NetworkMirror) do(ctx echo.Context, method, reqURL string, value any) error {
	req, err := http.NewRequestWithContext(ctx.Request().Context(), method, reqURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	return DecodeJSONBody(resp, value)
}
