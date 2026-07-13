package getter

import (
	"context"
	"errors"
	"io"
	"net/url"

	getter "github.com/hashicorp/go-getter/v2"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// ErrOCIGetFileUnsupported reports that oci sources always resolve to module
// directories, so single-file downloads are not supported.
var ErrOCIGetFileUnsupported = errors.New("GetFile is not supported for the OCI Getter")

// ErrOCIGetNotImplemented reports that the oci download logic has not landed
// yet; Get is stubbed until then.
var ErrOCIGetNotImplemented = errors.New("Get is not implemented yet for the OCI Getter")

// OCIRepositoryStore is the narrow seam between the OCI getter and the
// registry client that serves it. Resolve turns a tag or digest reference
// into a manifest descriptor; Fetch streams the blob a descriptor points at.
// The method set intentionally matches oras-go's content.Fetcher signature so
// manifest and blob helpers work through the seam without adapters, and unit
// tests can drive the getter with a fake store and no network.
type OCIRepositoryStore interface {
	Resolve(ctx context.Context, ref string) (ociv1.Descriptor, error)
	Fetch(ctx context.Context, desc ociv1.Descriptor) (io.ReadCloser, error)
}

// OCIGetter is the go-getter v2 implementation of the oci:// protocol.
//
// Source URLs take the form:
//
//	oci://REGISTRY_DOMAIN/REPOSITORY[//SUBDIR][?tag=TAG|?digest=DIGEST]
//
// where REGISTRY_DOMAIN is an OCI Distribution registry host (host:port
// allowed) and REPOSITORY is the full, possibly multi-segment repository
// name. The manifest contract the getter enforces is centralized in
// [ArtifactTypeModulePkg] and [MediaTypeModuleZip].
//
// NewStore is the dependency-injection seam: credential resolution and
// registry transport live entirely behind it, so credential tiers are
// additive changes and tests inject a fake [OCIRepositoryStore]. The seam
// rides as a struct field rather than a function parameter only because the
// go-getter Getter interface fixes the method set, leaving no parameter to
// thread it through.
type OCIGetter struct {
	NewStore func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error)
}

var _ getter.Getter = (*OCIGetter)(nil)

// Mode reports directory mode for all oci sources, since oci always
// downloads a module directory.
func (g *OCIGetter) Mode(_ context.Context, _ *url.URL) (getter.Mode, error) {
	return getter.ModeDir, nil
}

// Detect recognizes oci:// sources and oci-forced sources.
func (g *OCIGetter) Detect(req *getter.Request) (bool, error) {
	if req.Forced == SchemeOCI {
		return true, nil
	}

	u, err := url.Parse(req.Src)
	if err == nil && u.Scheme == SchemeOCI {
		return true, nil
	}

	return false, nil
}

// Get is stubbed until the module download logic lands.
func (g *OCIGetter) Get(_ context.Context, _ *getter.Request) error {
	return ErrOCIGetNotImplemented
}

// GetFile always fails, per [ErrOCIGetFileUnsupported].
func (g *OCIGetter) GetFile(_ context.Context, _ *getter.Request) error {
	return ErrOCIGetFileUnsupported
}
