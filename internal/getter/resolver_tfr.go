package getter

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-cleanhttp"
)

// tfrResolverTimeout caps the registry probe so a slow registry can't
// stall CAS dispatch. On timeout the resolver returns
// [cas.ErrNoVersionMetadata] and CAS falls back to content hashing
// after the fetch.
const tfrResolverTimeout = 10 * time.Second

// TFRResolver is a [cas.SourceResolver] for tfr:// URLs.
//
// Probe resolves the source via the Terraform/OpenTofu registry's module
// download endpoint and returns the resolved X-Terraform-Get URL as a
// content-addressed cache key. That URL encodes the immutable underlying
// archive (a versioned tarball, a git commit SHA, etc.), so two identical
// tfr:// requests share one CAS entry while a republish under the same
// version pins to a new key.
type TFRResolver struct {
	HTTPClient         *http.Client
	Logger             log.Logger
	TofuImplementation tfimpl.Type
}

// NewTFRResolver returns a [TFRResolver] with sensible defaults: a
// [github.com/hashicorp/go-cleanhttp.DefaultClient] capped at
// [tfrResolverTimeout], [log.Default] for diagnostic output, and
// [tfimpl.OpenTofu] as the default implementation.
func NewTFRResolver() *TFRResolver {
	client := cleanhttp.DefaultClient()
	client.Timeout = tfrResolverTimeout

	return &TFRResolver{
		HTTPClient:         client,
		Logger:             log.Default(),
		TofuImplementation: tfimpl.OpenTofu,
	}
}

// WithHTTPClient overrides the HTTP client used for registry-protocol
// requests. Intended for tests routing through a
// [net/http/httptest.Server].
func (r *TFRResolver) WithHTTPClient(c *http.Client) *TFRResolver {
	r.HTTPClient = c
	return r
}

// WithLogger overrides the logger used for diagnostic output.
func (r *TFRResolver) WithLogger(l log.Logger) *TFRResolver {
	r.Logger = l
	return r
}

// WithTofuImplementation selects which default registry domain is used
// when a tfr:// URL omits its host. See [tfimpl.DefaultRegistryDomain].
func (r *TFRResolver) WithTofuImplementation(impl tfimpl.Type) *TFRResolver {
	r.TofuImplementation = impl
	return r
}

// Scheme returns "tfr".
func (r *TFRResolver) Scheme() string { return SchemeTFR }

// Probe runs the registry's service-discovery + module-download protocol
// against rawURL and returns the resolved X-Terraform-Get URL as a
// content-addressed cache key.
//
// Any failure — malformed URL, missing version query, registry error —
// returns [cas.ErrNoVersionMetadata] so the fetch falls through to the
// download-then-content-hash path. The underlying error surfaces on the
// real fetch attempt.
func (r *TFRResolver) Probe(ctx context.Context, rawURL string) (string, error) {
	srcURL, err := url.Parse(rawURL)
	if err != nil || srcURL.Scheme != SchemeTFR {
		return "", cas.ErrNoVersionMetadata
	}

	registryDomain := srcURL.Host
	if registryDomain == "" {
		registryDomain = tfimpl.DefaultRegistryDomain(r.TofuImplementation)
	}

	versionList, hasVersion := srcURL.Query()[versionQueryKey]
	if !hasVersion || len(versionList) != 1 || versionList[0] == "" {
		return "", cas.ErrNoVersionMetadata
	}

	version := versionList[0]

	// Strip the //subdir selector so two URLs that only differ in subdir
	// produce the same probe key. The fetcher still sees the original
	// URL and handles subdir extraction itself.
	modulePath, _ := SourceDirSubdir(srcURL.Path)

	ctx, cancel := context.WithTimeout(ctx, tfrResolverTimeout)
	defer cancel()

	moduleRegistryBasePath, err := GetModuleRegistryURLBasePath(ctx, r.Logger, r.HTTPClient, registryDomain)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	moduleURL, err := BuildRequestURL(registryDomain, moduleRegistryBasePath, modulePath, version)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	terraformGet, err := GetTerraformGetHeader(ctx, r.Logger, r.HTTPClient, moduleURL)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	downloadURL, err := GetDownloadURLFromHeader(moduleURL, terraformGet)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	return cas.ContentKey("tfr-xtg", downloadURL), nil
}
