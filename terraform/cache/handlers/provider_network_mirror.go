package handlers

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/gruntwork-io/terragrunt/terraform/cliconfig"
	"github.com/labstack/echo/v4"
)

var availablePlatforms []models.Platform = []models.Platform{
	{OS: "solaris", Arch: "amd64"},
	{OS: "openbsd", Arch: "386"},
	{OS: "openbsd", Arch: "arm"},
	{OS: "openbsd", Arch: "amd64"},
	{OS: "freebsd", Arch: "386"},
	{OS: "freebsd", Arch: "arm"},
	{OS: "freebsd", Arch: "amd64"},
	{OS: "linux", Arch: "386"},
	{OS: "linux", Arch: "arm"},
	{OS: "linux", Arch: "arm64"},
	{OS: "linux", Arch: "amd64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
	{OS: "windows", Arch: "386"},
	{OS: "windows", Arch: "amd64"},
}

type ProviderNetworkMirrorHandler struct {
	*http.Client
	providerService             *services.ProviderService
	cacheProviderHTTPStatusCode int

	networkMirrorURL string
	// includeProvider and excludeProvider are sets of provider matching patterns that together define which providers are eligible to be potentially installed from the corresponding Source.
	includeProvider, excludeProvider models.Providers
}

func NewProviderNetworkMirrorHandler(providerService *services.ProviderService, cacheProviderHTTPStatusCode int, networkMirror *cliconfig.ProviderInstallationNetworkMirror) ProviderHandler {
	var includeProvider, excludeProvider models.Providers

	if addrs := networkMirror.Include; addrs != nil {
		includeProvider = models.ParseProvidersFromAddresses(*addrs...)
	}

	if addrs := networkMirror.Exclude; addrs != nil {
		excludeProvider = models.ParseProvidersFromAddresses(*addrs...)
	}

	return &ProviderNetworkMirrorHandler{
		Client:                      &http.Client{},
		providerService:             providerService,
		cacheProviderHTTPStatusCode: cacheProviderHTTPStatusCode,
		networkMirrorURL:            networkMirror.URL,
		includeProvider:             includeProvider,
		excludeProvider:             excludeProvider,
	}
}

// CanHandleProvider implements ProviderHandler.CanHandleProvider
func (handler *ProviderNetworkMirrorHandler) CanHandleProvider(provider *models.Provider) bool {
	switch {
	case handler.excludeProvider.Find(provider) != nil:
		return false
	case len(handler.includeProvider) > 0:
		return handler.includeProvider.Find(provider) != nil
	default:
		return true
	}
}

// GetVersions implements ProviderHandler.GetVersions
func (handler *ProviderNetworkMirrorHandler) GetVersions(ctx echo.Context, provider *models.Provider) error {
	var respData struct {
		Versions map[string]struct{} `json:"versions"`
	}

	reqPath := path.Join(provider.RegistryName, provider.Namespace, provider.Name, "index.json")
	if err := handler.request(ctx, http.MethodGet, reqPath, &respData); err != nil {
		return err
	}

	versions := struct {
		ID       string           `json:"id"`
		Versions []models.Version `json:"versions"`
	}{
		ID: path.Join(provider.RegistryName, provider.Namespace, provider.Name),
	}

	for version := range respData.Versions {
		versions.Versions = append(versions.Versions, models.Version{
			Version:   version,
			Platforms: availablePlatforms,
		})
	}

	return ctx.JSON(http.StatusOK, versions)
}

// GetPlatfrom implements ProviderHandler.GetPlatfrom
func (handler *ProviderNetworkMirrorHandler) GetPlatfrom(ctx echo.Context, provider *models.Provider, downloaderPrefix, cacheRequestID string) error {
	if cacheRequestID == "" {
		// it is impossible to return all platform properties from a network mirror, return 404 status
		return ctx.NoContent(http.StatusNotFound)
	}

	var respData struct {
		Archives map[string]struct {
			URL    string   `json:"url"`
			Hashes []string `json:"hashes"`
		} `json:"archives"`
	}

	reqPath := path.Join(provider.RegistryName, provider.Namespace, provider.Name, provider.Version+".json")
	if err := handler.request(ctx, http.MethodGet, reqPath, &respData); err != nil {
		return err
	}

	if archive, ok := respData.Archives[provider.Platform()]; ok {
		provider.ResponseBody = &models.ResponseBody{DownloadURL: archive.URL}
	}

	// start caching and return 423 status
	handler.providerService.CacheProvider(ctx.Request().Context(), cacheRequestID, provider)
	return ctx.NoContent(handler.cacheProviderHTTPStatusCode)
}

// Download implements ProviderHandler.Download
func (handler *ProviderNetworkMirrorHandler) Download(ctx echo.Context, provider *models.Provider) error {
	return ctx.NoContent(http.StatusNotImplemented)
}

func (handler *ProviderNetworkMirrorHandler) request(ctx echo.Context, networkMirror, reqPath string, value any) error {
	reqURL := fmt.Sprintf("%s/%s", strings.TrimRight(handler.networkMirrorURL, "/"), reqPath)

	req, err := http.NewRequestWithContext(ctx.Request().Context(), networkMirror, reqURL, nil)
	if err != nil {
		return err
	}

	resp, err := handler.Do(req)
	if err != nil {
		return err
	}

	return DecodeJSONBody(resp, value)
}
