package getter

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
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
type OCIResolver struct{}

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

	// Use the same credential resolution and localhost detection as OCIGetter.
	g := &OCIGetter{}
	repo.PlainHTTP = isLocalhostRegistry(repo.Reference.Registry)
	repo.Client = &auth.Client{
		Credential: g.credentialFunc(repo.Reference.Registry),
	}

	desc, err := repo.Resolve(ctx, repo.Reference.Reference)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	// The digest is content-addressed and immutable — it is the ideal CAS key.
	return cas.ContentKey("oci", fmt.Sprintf("%s@%s", ref, desc.Digest)), nil
}

// credentialFunc mirrors OCIGetter.credentialFunc but without a logger
// (probes should be quiet).
func (r *OCIResolver) credentialFunc(registry string) auth.CredentialFunc {
	store, err := credentials.NewStoreFromDocker(credentials.StoreOptions{AllowPlaintextPut: false})
	if err == nil {
		return credentials.Credential(store)
	}

	return auth.StaticCredential(registry, auth.Credential{})
}
