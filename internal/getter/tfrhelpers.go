package getter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-cleanhttp"
	goversion "github.com/hashicorp/go-version"
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

// ConstraintParseErr is returned when a version constraint cannot be parsed.
type ConstraintParseErr struct {
	err        error
	constraint string
}

func (e ConstraintParseErr) Error() string {
	return fmt.Sprintf("invalid version constraint %q: %s", e.constraint, e.err)
}

func (e ConstraintParseErr) Unwrap() error {
	return e.err
}

// NoMatchingVersionErr is returned when no published module version satisfies
// the requested version constraint.
type NoMatchingVersionErr struct {
	constraint     string
	modulePath     string
	registryDomain string
}

func (e NoMatchingVersionErr) Error() string {
	return fmt.Sprintf(
		"no published version of module %s on registry %s satisfies constraint %q",
		e.modulePath, e.registryDomain, e.constraint,
	)
}

// GetModuleRegistryURLBasePath performs the OpenTofu/Terraform
// service-discovery protocol against `domain` and returns the base path
// under which modules are hosted.
//
// See https://www.terraform.io/docs/internals/remote-service-discovery.html.
func GetModuleRegistryURLBasePath(
	ctx context.Context,
	l log.Logger,
	httpClient *http.Client,
	domain string,
) (string, error) {
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
		return "", ServiceDiscoveryErr{reason: fmt.Sprintf("Error parsing response body %s: %s", string(bodyData), err)}
	}

	if respJSON.ModulesPath == "" {
		return "", ServiceDiscoveryErr{reason: "modules.v1 missing in discovery response"}
	}

	return respJSON.ModulesPath, nil
}

// GetTerraformGetHeader makes a GET against `url` and returns the value of
// the X-Terraform-Get header (or the body's `location` field as a fallback).
func GetTerraformGetHeader(ctx context.Context, l log.Logger, httpClient *http.Client, url *url.URL) (string, error) {
	body, header, err := httpGETAndGetResponse(ctx, l, httpClient, url)
	if err != nil {
		return "", ModuleDownloadErr{sourceURL: url.String(), details: "error receiving HTTP data"}
	}

	terraformGet := header.Get("X-Terraform-Get")
	if terraformGet != "" {
		return terraformGet, nil
	}

	var responseJSON map[string]string
	if err := json.Unmarshal(body, &responseJSON); err != nil {
		return "", ModuleDownloadErr{
			sourceURL: url.String(),
			details:   fmt.Sprintf("Error parsing response body %s: %s", string(body), err),
		}
	}

	terraformGet = responseJSON["location"]
	if terraformGet != "" {
		return terraformGet, nil
	}

	return "", ModuleDownloadErr{
		sourceURL: url.String(),
		details:   "no source URL was returned in header X-Terraform-Get and in location response from download URL",
	}
}

// GetDownloadURLFromHeader resolves a relative X-Terraform-Get value
// against the module URL.
func GetDownloadURLFromHeader(moduleURL *url.URL, terraformGet string) (string, error) {
	if strings.HasPrefix(terraformGet, "/") || strings.HasPrefix(terraformGet, "./") ||
		strings.HasPrefix(terraformGet, "../") {
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

// GetLatestModuleVersion queries the OpenTofu or Terraform module registry to
// list available versions for the given module and returns the latest stable
// (non-prerelease) version. Prereleases are excluded to match OpenTofu and
// Terraform's default behavior when resolving an unconstrained module
// version; if a user wants a prerelease, they must pin it explicitly via
// `?version=`.
func GetLatestModuleVersion(
	ctx context.Context,
	l log.Logger,
	httpClient *http.Client,
	registryDomain, moduleRegistryBasePath, modulePath string,
) (string, error) {
	versions, err := listModuleVersions(ctx, l, httpClient, registryDomain, moduleRegistryBasePath, modulePath)
	if err != nil {
		return "", err
	}

	stable := make([]*goversion.Version, 0, len(versions))

	for _, v := range versions {
		if v.Prerelease() != "" {
			l.Debugf("Skipping prerelease version %q for module %s", v.Original(), modulePath)
			continue
		}

		stable = append(stable, v)
	}

	if len(stable) == 0 {
		return "", fmt.Errorf(
			"no stable versions found for module %s on registry %s; pin a version explicitly with ?version=",
			modulePath,
			registryDomain,
		)
	}

	latest := slices.MaxFunc(stable, func(a, b *goversion.Version) int { return a.Compare(b) })

	return latest.Original(), nil
}

// GetMatchingModuleVersion queries the OpenTofu or Terraform module registry
// and returns the highest published version of the module that satisfies
// constraint (for example "~> 3.3" or ">= 1.0.0, < 2.0.0").
//
// Prerelease eligibility follows OpenTofu and Terraform: a prerelease version
// is a candidate only when the constraint operand itself names a prerelease
// with the same base version. hashicorp/go-version enforces this in
// Constraints.Check, so every published version is passed through unfiltered.
func GetMatchingModuleVersion(
	ctx context.Context,
	l log.Logger,
	httpClient *http.Client,
	registryDomain, moduleRegistryBasePath, modulePath, constraint string,
) (string, error) {
	constraints, err := goversion.NewConstraint(constraint)
	if err != nil {
		return "", ConstraintParseErr{constraint: constraint, err: err}
	}

	versions, err := listModuleVersions(ctx, l, httpClient, registryDomain, moduleRegistryBasePath, modulePath)
	if err != nil {
		return "", err
	}

	matching := make([]*goversion.Version, 0, len(versions))

	for _, v := range versions {
		if constraints.Check(v) {
			matching = append(matching, v)
		}
	}

	if len(matching) == 0 {
		return "", NoMatchingVersionErr{constraint: constraint, modulePath: modulePath, registryDomain: registryDomain}
	}

	match := slices.MaxFunc(matching, func(a, b *goversion.Version) int { return a.Compare(b) })

	return match.Original(), nil
}

// listModuleVersions queries the registry's list-versions endpoint for the
// module and returns every published version that parses as semver, preserving
// the order the registry returned. Unparsable entries are skipped with a debug
// log so a single malformed row cannot block resolution; prereleases are kept,
// leaving the prerelease policy to the caller. This implements the "List
// Available Versions for a Specific Module" endpoint of the Terraform Module
// Registry Protocol.
// See: https://developer.hashicorp.com/terraform/registry/api-docs#list-available-versions-for-a-specific-module
func listModuleVersions(
	ctx context.Context,
	l log.Logger,
	httpClient *http.Client,
	registryDomain, moduleRegistryBasePath, modulePath string,
) ([]*goversion.Version, error) {
	moduleRegistryBasePath = strings.TrimSuffix(moduleRegistryBasePath, "/")
	modulePath = strings.TrimSuffix(modulePath, "/")
	modulePath = strings.TrimPrefix(modulePath, "/")

	versionsPath := fmt.Sprintf("%s/%s/versions", moduleRegistryBasePath, modulePath)

	versionsURL, err := url.Parse(versionsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse versions URL for %s: %w", modulePath, err)
	}

	// If the base path is relative (no scheme), construct the full URL using the registry domain.
	if versionsURL.Scheme == "" {
		versionsURL = &url.URL{
			Scheme: "https",
			Host:   registryDomain,
			Path:   versionsPath,
		}
	}

	bodyData, _, err := httpGETAndGetResponse(ctx, l, httpClient, versionsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query module versions for %s: %w", modulePath, err)
	}

	var versionsResp moduleVersionsResponse
	if err := json.Unmarshal(bodyData, &versionsResp); err != nil {
		return nil, fmt.Errorf("failed to parse module versions response for %s: %w", modulePath, err)
	}

	if len(versionsResp.Modules) == 0 || len(versionsResp.Modules[0].Versions) == 0 {
		return nil, fmt.Errorf("no versions found for module %s on registry %s", modulePath, registryDomain)
	}

	parsed := make([]*goversion.Version, 0, len(versionsResp.Modules[0].Versions))

	for _, v := range versionsResp.Modules[0].Versions {
		pv, err := goversion.NewVersion(v.Version)
		if err != nil {
			l.Debugf("Skipping unparsable version %q for module %s: %v", v.Version, modulePath, err)
			continue
		}

		parsed = append(parsed, pv)
	}

	return parsed, nil
}

// moduleVersionsResponse is the registry API response for the list-versions endpoint.
type moduleVersionsResponse struct {
	Modules []moduleVersionsEntry `json:"modules"`
}

// moduleVersionsEntry holds the versions list for a single module.
type moduleVersionsEntry struct {
	Versions []moduleVersion `json:"versions"`
}

// moduleVersion is a single version record in the registry response.
type moduleVersion struct {
	Version string `json:"version"`
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
func httpGETAndGetResponse(
	ctx context.Context,
	l log.Logger,
	httpClient *http.Client,
	getURL *url.URL,
) ([]byte, *http.Header, error) {
	if httpClient == nil {
		httpClient = cleanhttp.DefaultClient()
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
		return nil, nil, RegistryAPIErr{url: getURL.String(), statusCode: resp.StatusCode}
	}

	bodyData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading registry response body from %s: %w", getURL, err)
	}

	return bodyData, &resp.Header, nil
}
