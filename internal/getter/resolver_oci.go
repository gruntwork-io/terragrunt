package getter

import (
	"context"
	"net/url"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cas"
)

// ociResolverTimeout caps the registry probe so a slow registry cannot stall CAS dispatch.
const ociResolverTimeout = 10 * time.Second

// ociManifestKeyAlg namespaces CAS keys derived from OCI manifest digests.
const ociManifestKeyAlg = "oci-manifest"

// OCIResolver is a [cas.SourceResolver] for oci:// URLs.
//
// A digest pin is already a content hash, so it becomes the cache key without
// touching the registry. A tag is mutable, so Probe re-resolves it to its
// manifest digest on every call; an unchanged tag therefore hits the same
// entry while a re-pushed tag misses and re-fetches.
type OCIResolver struct {
	NewStore OCINewStoreFunc
}

// NewOCIResolver returns an [OCIResolver] probing through newStore, so
// resolution shares the getter's credential path.
func NewOCIResolver(newStore OCINewStoreFunc) *OCIResolver {
	return &OCIResolver{NewStore: newStore}
}

// Scheme returns "oci".
func (r *OCIResolver) Scheme() string { return SchemeOCI }

// Probe returns the manifest digest of rawURL as a content-addressed cache
// key. Any failure returns [cas.ErrNoVersionMetadata] so the fetch falls
// through to the download-then-content-hash path, surfacing the underlying
// error on the real fetch attempt.
func (r *OCIResolver) Probe(ctx context.Context, rawURL string) (string, error) {
	srcURL, err := url.Parse(rawURL)
	if err != nil || srcURL.Scheme != SchemeOCI {
		return "", cas.ErrNoVersionMetadata
	}

	queryValues := srcURL.Query()
	// Drop the archive marker the CAS dispatch injects.
	queryValues.Del("archive")
	srcURL.RawQuery = queryValues.Encode()

	registryDomain, repositoryName, _, ref, err := parseOCISource(srcURL)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	// A digest pin already names the manifest content; no resolution needed.
	if queryValues.Has(ociDigestQueryKey) {
		return cas.ContentKey(ociManifestKeyAlg, ref), nil
	}

	if r.NewStore == nil {
		return "", cas.ErrNoVersionMetadata
	}

	ctx, cancel := context.WithTimeout(ctx, ociResolverTimeout)
	defer cancel()

	store, err := r.NewStore(ctx, registryDomain, repositoryName)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	// Tags are mutable: resolve on every probe so a re-pushed tag can never
	// serve a stale cache entry.
	desc, err := store.Resolve(ctx, ref)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	return cas.ContentKey(ociManifestKeyAlg, desc.Digest.String()), nil
}
