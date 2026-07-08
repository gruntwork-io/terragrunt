package getter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// ociResolverTimeout caps the registry probe so a slow registry can't stall
// CAS dispatch. On timeout the resolver returns [cas.ErrNoVersionMetadata]
// and CAS falls back to content-hashing after the fetch.
const ociResolverTimeout = 10 * time.Second

// OCIResolver is a [cas.SourceResolver] for oci:// URLs.
//
// Probe resolves the source reference to an immutable digest via the OCI
// Distribution Spec HEAD manifest endpoint. The digest is used as the CAS
// cache key, so two requests for the same tag share one CAS entry only when
// the tag still points at the same digest — a re-push under the same tag
// correctly busts the cache.
type OCIResolver struct {
	// HTTPClient is used for all registry requests. When nil, http.DefaultClient
	// is used. Set this to inject a custom transport (e.g. a TLS-configured
	// client for a test server).
	HTTPClient *http.Client
	// PlainHTTP forces plain HTTP for all resolver requests. Loopback registries
	// (localhost, 127.0.0.1, ::1) use plain HTTP automatically regardless of
	// this setting.
	PlainHTTP bool
}

// NewOCIResolver returns an [OCIResolver].
func NewOCIResolver() *OCIResolver { return &OCIResolver{} }

// Scheme returns "oci".
func (r *OCIResolver) Scheme() string { return SchemeOCI }

// Probe resolves a tag reference to its immutable digest and returns it as a
// CAS cache key.
//
// Returns [cas.ErrNoVersionMetadata] when the URL cannot be parsed or the
// registry probe fails; FetchSource then falls back to download + content-hash.
func (r *OCIResolver) Probe(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != SchemeOCI {
		return "", cas.ErrNoVersionMetadata
	}

	// Strip any //subdir suffix before resolving — two URLs that differ only in
	// subdir map to the same artifact and should share one CAS entry.
	rawPath, _ := SourceDirSubdir(u.Path)
	u.Path = rawPath

	ref := ociRefFromURL(u)

	ctx, cancel := context.WithTimeout(ctx, ociResolverTimeout)
	defer cancel()

	repo, err := remote.NewRepository(ref)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	repo.PlainHTTP = r.PlainHTTP || isLocalhostRegistry(repo.Reference.Registry)
	repo.Client = &auth.Client{
		Client:     r.HTTPClient,
		Credential: ociCredentialFunc(repo.Reference.Registry, nil),
	}

	desc, err := repo.Resolve(ctx, repo.Reference.Reference)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	// The digest is content-addressed and immutable — it is the ideal CAS key.
	return cas.ContentKey("oci", fmt.Sprintf("%s@%s", ref, desc.Digest)), nil
}
