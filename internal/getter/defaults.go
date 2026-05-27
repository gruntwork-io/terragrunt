package getter

import (
	"net/http"

	gcs "github.com/hashicorp/go-getter/gcs/v2"
	s3 "github.com/hashicorp/go-getter/s3/v2"
	getter "github.com/hashicorp/go-getter/v2"
)

// Registry keys for the non-git fetcher and resolver maps. They match
// the lowercased scheme strings CASGetter.Detect produces. Exported so
// callers can extend or replace specific entries in
// DefaultGenericFetchers and DefaultSourceResolvers.
const (
	SchemeS3    = "s3"
	SchemeGCS   = "gcs"
	SchemeHTTP  = "http"
	SchemeHTTPS = "https"
	SchemeHg    = "hg"
	SchemeSMB   = "smb"
)

// GenericFetcherOption configures DefaultGenericFetchers.
type GenericFetcherOption func(*genericFetcherConfig)

type genericFetcherConfig struct {
	httpExtra  http.Header
	httpsExtra http.Header
}

// WithHTTPExtraHeaders attaches header to the bare http getter so
// auth headers reach the wire on a CAS miss. Intended for tests that
// talk to net/http/httptest; production callers want
// WithHTTPSExtraHeaders.
func WithHTTPExtraHeaders(header http.Header) GenericFetcherOption {
	return func(c *genericFetcherConfig) { c.httpExtra = header }
}

// WithHTTPSExtraHeaders attaches header to the bare https getter.
func WithHTTPSExtraHeaders(header http.Header) GenericFetcherOption {
	return func(c *genericFetcherConfig) { c.httpsExtra = header }
}

// DefaultGenericFetchers returns the per-scheme bare getters CASGetter
// uses on a cache miss. Exported so callers that build dedicated
// CAS-only clients (the CAS-experiment path in
// runner/run/download_source.go) share the fetcher set NewClient uses.
func DefaultGenericFetchers(opts ...GenericFetcherOption) map[string]getter.Getter {
	var cfg genericFetcherConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	return map[string]getter.Getter{
		SchemeS3:    new(s3.Getter),
		SchemeGCS:   new(gcs.Getter),
		SchemeHTTP:  &HTTPSchemeGetter{Inner: newHTTPGetter(cfg.httpExtra), Scheme: SchemeHTTP},
		SchemeHTTPS: &HTTPSchemeGetter{Inner: newHTTPGetter(cfg.httpsExtra), Scheme: SchemeHTTPS},
		SchemeHg:    new(getter.HgGetter),
		SchemeSMB:   new(getter.SmbClientGetter),
	}
}

// buildGetters realizes the option set into the ordered Getter slice
// the client iterates. v2 uses first-match detection so order matters:
//
//  1. tfr (Terraform Registry): must precede git so tfr:// wins forced
//     detection.
//  2. CAS protocol getter (when CAS is enabled): resolves `cas::`
//     source references produced by `update_source_with_cas` stack
//     rewrites.
//  3. CAS getter (when CAS is enabled): intercepts git, s3, gcs,
//     http(s), hg, and smb sources ahead of the bare protocol getters.
//  4. Caller-prepended getters (tests).
//  5. The default protocol set: git, hg, smb, http(s), s3, gcs, file.
//
// file goes last so it does not claim sources another detector
// recognizes.
func buildGetters(b *builder) []Getter {
	var (
		out         []Getter
		fileGetter  Getter
		gitGetter   Getter
		httpGetter  Getter
		httpsGetter Getter
	)

	fileGetter = new(getter.FileGetter)
	if b.fileCopy != nil {
		fileGetter = b.fileCopy
	}

	gitGetter = NewGitGetter()

	httpGetter = &HTTPSchemeGetter{Inner: newHTTPGetter(b.httpExtraHeader), Scheme: SchemeHTTP}
	httpsGetter = &HTTPSchemeGetter{Inner: newHTTPGetter(b.httpsExtraHeader), Scheme: SchemeHTTPS}

	hgGetter := new(getter.HgGetter)
	smbClientGetter := new(getter.SmbClientGetter)
	smbMountGetter := new(getter.SmbMountGetter)
	s3Getter := new(s3.Getter)
	gcsGetter := new(gcs.Getter)

	if b.tfRegistry != nil {
		out = append(out, b.tfRegistry)
	}

	if b.casStore != nil {
		fetchers := map[string]getter.Getter{
			SchemeS3:    s3Getter,
			SchemeGCS:   gcsGetter,
			SchemeHTTP:  httpGetter,
			SchemeHTTPS: httpsGetter,
			SchemeHg:    hgGetter,
			SchemeSMB:   smbClientGetter,
		}

		out = append(out,
			NewCASProtocolGetter(b.logger, b.casStore, b.casVenv),
			NewCASGetter(b.logger, b.casStore, b.casVenv, b.casCloneOpts,
				WithGenericFetchers(fetchers),
				WithGenericResolvers(DefaultSourceResolvers()),
			),
		)
	}

	out = append(out, b.prepended...)

	out = append(out,
		gitGetter,
		hgGetter,
		smbClientGetter,
		smbMountGetter,
		httpGetter,
		httpsGetter,
		s3Getter,
		gcsGetter,
		fileGetter,
	)

	return out
}

// newHTTPGetter constructs an HttpGetter with Netrc enabled (matching
// Terragrunt's historic behavior under v1's UpdateGetters customization)
// and an optional set of extra headers. Pass nil for `extra` to get the
// default getter; pass a non-nil header set to inject auth (used by
// WithHTTPAuth and WithHTTPSAuth for GitHub release downloads).
//
// XTerraformGet is left enabled (the default) so X-Terraform-Get
// redirects continue to work.
func newHTTPGetter(extra http.Header) *getter.HttpGetter {
	return &getter.HttpGetter{Netrc: true, Header: extra}
}
