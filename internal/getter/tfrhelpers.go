package getter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	svchost "github.com/hashicorp/terraform-svchost"
)

const (
	serviceDiscoveryPath = "/.well-known/terraform.json"
	authTokenEnvName     = "TG_TF_REGISTRY_TOKEN"
)

// RegistryServicePath is the modules service path returned by service discovery.
type RegistryServicePath struct {
	ModulesPath string `json:"modules.v1"`
}

// MalformedRegistryURLErr is returned when the tfr:// URL is malformed.
type MalformedRegistryURLErr struct {
	reason string
}

func (e MalformedRegistryURLErr) Error() string {
	return "tfr getter URL is malformed: " + e.reason
}

// ServiceDiscoveryErr is returned when the service-discovery protocol fails.
type ServiceDiscoveryErr struct {
	reason string
}

func (e ServiceDiscoveryErr) Error() string {
	return "Error identifying module registry API location: " + e.reason
}

// ModuleDownloadErr is returned when downloading the module fails.
type ModuleDownloadErr struct {
	sourceURL string
	details   string
}

func (e ModuleDownloadErr) Error() string {
	return fmt.Sprintf("Error downloading module from %s: %s", e.sourceURL, e.details)
}

// RegistryAPIErr is returned for non-2xx responses from the registry.
type RegistryAPIErr struct {
	url        string
	statusCode int
}

func (e RegistryAPIErr) Error() string {
	return fmt.Sprintf("Failed to fetch url %s: status code %d", e.url, e.statusCode)
}

// GetModuleRegistryURLBasePath performs the OpenTofu/Terraform
// service-discovery protocol against `domain` and returns the base path
// under which modules are hosted.
//
// See https://www.terraform.io/docs/internals/remote-service-discovery.html.
func GetModuleRegistryURLBasePath(ctx context.Context, l log.Logger, httpClient vhttp.Client, domain string) (string, error) {
	sdURL := url.URL{
		Scheme: "https",
		Host:   domain,
		Path:   serviceDiscoveryPath,
	}

	bodyData, _, err := httpGETAndGetResponse(ctx, l, httpClient, &sdURL)
	if err != nil {
		return "", err
	}

	var respJSON RegistryServicePath
	if err := json.Unmarshal(bodyData, &respJSON); err != nil {
		return "", errors.New(ServiceDiscoveryErr{reason: fmt.Sprintf("Error parsing response body %s: %s", string(bodyData), err)})
	}

	if respJSON.ModulesPath == "" {
		return "", errors.New(ServiceDiscoveryErr{reason: "modules.v1 missing in discovery response"})
	}

	return respJSON.ModulesPath, nil
}

// GetTerraformGetHeader makes a GET against `url` and returns the value of
// the X-Terraform-Get header (or the body's `location` field as a fallback).
func GetTerraformGetHeader(ctx context.Context, l log.Logger, httpClient vhttp.Client, url *url.URL) (string, error) {
	body, header, err := httpGETAndGetResponse(ctx, l, httpClient, url)
	if err != nil {
		return "", errors.New(ModuleDownloadErr{sourceURL: url.String(), details: "error receiving HTTP data"})
	}

	terraformGet := header.Get("X-Terraform-Get")
	if terraformGet != "" {
		return terraformGet, nil
	}

	var responseJSON map[string]string
	if err := json.Unmarshal(body, &responseJSON); err != nil {
		return "", errors.New(ModuleDownloadErr{
			sourceURL: url.String(),
			details:   fmt.Sprintf("Error parsing response body %s: %s", string(body), err),
		})
	}

	terraformGet = responseJSON["location"]
	if terraformGet != "" {
		return terraformGet, nil
	}

	return "", errors.New(ModuleDownloadErr{
		sourceURL: url.String(),
		details:   "no source URL was returned in header X-Terraform-Get and in location response from download URL",
	})
}

// GetDownloadURLFromHeader resolves a relative X-Terraform-Get value
// against the module URL.
func GetDownloadURLFromHeader(moduleURL *url.URL, terraformGet string) (string, error) {
	if strings.HasPrefix(terraformGet, "/") || strings.HasPrefix(terraformGet, "./") || strings.HasPrefix(terraformGet, "../") {
		relativePathURL, err := url.Parse(terraformGet)
		if err != nil {
			return "", fmt.Errorf("parsing X-Terraform-Get value %q: %w", terraformGet, err)
		}

		terraformGet = moduleURL.ResolveReference(relativePathURL).String()
	}

	return terraformGet, nil
}

// BuildRequestURL constructs the registry download URL for the given module.
func BuildRequestURL(registryDomain, moduleRegistryBasePath, modulePath, version string) (*url.URL, error) {
	moduleRegistryBasePath = strings.TrimSuffix(moduleRegistryBasePath, "/")
	modulePath = strings.TrimSuffix(modulePath, "/")
	modulePath = strings.TrimPrefix(modulePath, "/")

	moduleFullPath := fmt.Sprintf("%s/%s/%s/download", moduleRegistryBasePath, modulePath, version)

	moduleURL, err := url.Parse(moduleFullPath)
	if err != nil {
		return nil, err
	}

	if moduleURL.Scheme != "" {
		return moduleURL, nil
	}

	return &url.URL{Scheme: "https", Host: registryDomain, Path: moduleFullPath}, nil
}

// applyHostToken adds an Authorization header to req based on the user's
// OpenTofu/Terraform CLI config or the TG_TF_REGISTRY_TOKEN env var.
func applyHostToken(req *http.Request) (*http.Request, error) {
	cliCfg, err := cliconfig.LoadUserConfig()
	if err != nil {
		return nil, err
	}

	if creds := cliCfg.CredentialsSource().ForHost(svchost.Hostname(req.URL.Hostname())); creds != nil {
		creds.PrepareRequest(req)
		return req, nil
	}

	if authToken := os.Getenv(authTokenEnvName); authToken != "" {
		req.Header.Add("Authorization", "Bearer "+authToken)
	}

	return req, nil
}

// httpGETAndGetResponse performs a GET against getURL and returns its body and headers.
func httpGETAndGetResponse(ctx context.Context, l log.Logger, httpClient vhttp.Client, getURL *url.URL) ([]byte, *http.Header, error) {
	if httpClient == nil {
		httpClient = vhttp.NewOSClient()
	}

	if getURL == nil {
		return nil, nil, errors.New("httpGETAndGetResponse received nil getURL")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", getURL.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("building registry HTTP request for %s: %w", getURL, err)
	}

	req, err = applyHostToken(req)
	if err != nil {
		return nil, nil, fmt.Errorf("applying registry auth token for %s: %w", getURL, err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("executing registry HTTP request to %s: %w", getURL, err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			l.Warnf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, errors.New(RegistryAPIErr{url: getURL.String(), statusCode: resp.StatusCode})
	}

	bodyData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading registry response body from %s: %w", getURL, err)
	}

	return bodyData, &resp.Header, nil
}
