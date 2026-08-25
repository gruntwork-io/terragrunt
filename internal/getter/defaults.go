package getter

import (
	"errors"
	"net/http"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	getter "github.com/hashicorp/go-getter/v2"
)

// ErrNilVenv reports a nil venv handed to a constructor that authenticates through one.
var ErrNilVenv = errors.New("getter: venv must not be nil")

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
	SchemeTFR   = "tfr"
	SchemeOCI   = "oci"
)

// GenericFetcherOption configures the generic-dispatch defaults consumed
// by DefaultGenericFetchers and DefaultSourceResolvers.
type GenericFetcherOption func(*genericFetcherConfig)

type genericFetcherConfig struct {
	logger     log.Logger
	fsys       vfs.FS
	venv       *venv.Venv
	ociHolder  *ociStoreHolder
	httpExtra  http.Header
	httpsExtra http.Header
	httpClient vhttp.Client
	tfrImpl    tfimpl.Type
	tfrEnabled bool
}

// ociStoreHolder builds one store seam shared by the oci fetcher and resolver.
type ociStoreHolder struct {
	fn   OCINewStoreFunc
	v    *venv.Venv
	once sync.Once
}

func (h *ociStoreHolder) store(l log.Logger) OCINewStoreFunc {
	h.once.Do(func() {
		h.fn = NewOCIRepositoryStore(l, h.v)
	})

	return h.fn
}

// requireLoggerFS panics when logger or fsys is unset for a scheme that needs them.
func requireLoggerFS(c *genericFetcherConfig, scheme string) {
	if c.logger == nil {
		panic("getter: " + scheme + " requires WithDispatchLogger")
	}

	if c.fsys == nil {
		panic("getter: " + scheme + " requires WithDispatchFS")
	}
}

// WithDispatchLogger sets the shared logger for tfr and oci dispatch entries.
func WithDispatchLogger(l log.Logger) GenericFetcherOption {
	return func(c *genericFetcherConfig) {
		if l == nil {
			panic("getter: WithDispatchLogger requires a non-nil logger")
		}

		c.logger = l
	}
}

// WithDispatchFS sets the shared filesystem for tfr and oci dispatch entries.
func WithDispatchFS(fsys vfs.FS) GenericFetcherOption {
	return func(c *genericFetcherConfig) {
		if fsys == nil {
			panic("getter: WithDispatchFS requires a non-nil filesystem")
		}

		c.fsys = fsys
	}
}

// WithDispatchVenv sets the virtualized environment the tfr dispatch entries
// authenticate through, so the fetcher and the resolver read the registry
// token and the user's CLI config from the same handles.
func WithDispatchVenv(v *venv.Venv) GenericFetcherOption {
	if v == nil {
		panic(ErrNilVenv)
	}

	return func(c *genericFetcherConfig) { c.venv = v }
}

// dispatchVenv returns the venv the tfr dispatch entries ride: the one
// [WithDispatchVenv] persisted, or v when the caller left it unset.
func dispatchVenv(v *venv.Venv, c *genericFetcherConfig) *venv.Venv {
	if c.venv != nil {
		return c.venv
	}

	return v
}

// WithOCIConfig enables oci:// registration; callers must also pass [WithDispatchLogger] and [WithDispatchFS].
func WithOCIConfig(v *venv.Venv) GenericFetcherOption {
	holder := &ociStoreHolder{v: v}

	return func(c *genericFetcherConfig) {
		c.ociHolder = holder
	}
}

// WithHTTPClient overrides the outbound-HTTP client the generic-dispatch
// fetchers and resolvers probe and fetch through, replacing the venv
// client [WithDefaultGenericDispatch] supplies. Required by
// [DefaultGenericFetchers] when [WithTFRConfig] registers the tfr
// fetcher; [DefaultSourceResolvers] takes its client as a parameter
// instead.
func WithHTTPClient(c vhttp.Client) GenericFetcherOption {
	return func(cfg *genericFetcherConfig) { cfg.httpClient = c }
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

// WithTFRConfig enables tfr:// registration; callers must also pass [WithDispatchLogger] and [WithDispatchFS].
func WithTFRConfig(impl tfimpl.Type) GenericFetcherOption {
	return func(c *genericFetcherConfig) {
		c.tfrImpl = impl
		c.tfrEnabled = true
	}
}

// DefaultGenericFetchers returns the per-scheme bare getters CASGetter
// uses on a cache miss. Exported so callers that build dedicated
// CAS-only clients (the CAS-experiment path in
// runner/run/download_source.go) share the fetcher set NewClient uses.
func DefaultGenericFetchers(v *venv.Venv, opts ...GenericFetcherOption) map[string]getter.Getter {
	var cfg genericFetcherConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	m := map[string]getter.Getter{
		SchemeS3:    NewS3Getter(v),
		SchemeGCS:   NewGCSGetter(v),
		SchemeHTTP:  &HTTPSchemeGetter{Inner: newHTTPGetter(cfg.httpClient, cfg.httpExtra), Scheme: SchemeHTTP},
		SchemeHTTPS: &HTTPSchemeGetter{Inner: newHTTPGetter(cfg.httpClient, cfg.httpsExtra), Scheme: SchemeHTTPS},
		SchemeHg:    new(getter.HgGetter),
		SchemeSMB:   new(getter.SmbClientGetter),
	}

	if cfg.tfrEnabled {
		requireLoggerFS(&cfg, SchemeTFR)

		if cfg.httpClient == nil {
			panic(
				"getter.DefaultGenericFetchers: WithHTTPClient is required when WithTFRConfig registers the tfr fetcher",
			)
		}

		m[SchemeTFR] = NewRegistryGetter(cfg.logger, dispatchVenv(v, &cfg)).
			WithTofuImplementation(cfg.tfrImpl)
	}

	if cfg.ociHolder != nil {
		requireLoggerFS(&cfg, SchemeOCI)
		m[SchemeOCI] = &OCIGetter{
			NewStore: cfg.ociHolder.store(cfg.logger),
			Logger:   cfg.logger,
			FS:       cfg.fsys,
		}
	}

	return m
}

// buildGetters realizes the option set into the ordered Getter slice
// the client iterates. v2 uses first-match detection so order matters:
//
//  1. tfr (Terraform Registry): must precede git so tfr:// wins forced
//     detection.
//  2. oci (OCI Distribution registries, when registered): precedes the
//     generic getters so oci:// wins forced detection.
//  3. CAS protocol getter (when CAS is enabled): resolves `cas::`
//     source references produced by `update_source_with_cas` stack
//     rewrites.
//  4. CAS getter (when CAS is enabled): intercepts git, s3, gcs,
//     http(s), hg, and smb sources ahead of the bare protocol getters.
//  5. Caller-prepended getters (tests).
//  6. The default protocol set: git, hg, smb, http(s), s3, gcs, file.
//
// file goes last so it does not claim sources another detector
// recognizes.
func buildGetters(b *builder) []Getter {
	var out []Getter

	var fileGetter Getter = new(getter.FileGetter)
	if b.fileCopy != nil {
		fileGetter = b.fileCopy
	}

	if b.tfRegistry != nil {
		out = append(out, b.tfRegistry)
	}

	if b.oci != nil {
		out = append(out, b.oci)
	}

	gitGetter := NewGitGetter()

	var httpGetter Getter = &HTTPSchemeGetter{
		Inner:  newHTTPGetter(b.httpClient, b.httpExtraHeader),
		Scheme: SchemeHTTP,
	}

	var httpsGetter Getter = &HTTPSchemeGetter{
		Inner:  newHTTPGetter(b.httpClient, b.httpsExtraHeader),
		Scheme: SchemeHTTPS,
	}

	hgGetter := new(getter.HgGetter)
	smbClientGetter := new(getter.SmbClientGetter)
	smbMountGetter := new(getter.SmbMountGetter)

	s3Getter := NewS3Getter(b.v)
	gcsGetter := NewGCSGetter(b.v)

	if b.casStore != nil {
		if b.httpClient == nil {
			panic("getter: WithCAS requires WithHTTP; wire the venv client at construction")
		}

		fetchers := map[string]getter.Getter{
			SchemeS3:    s3Getter,
			SchemeGCS:   gcsGetter,
			SchemeHTTP:  httpGetter,
			SchemeHTTPS: httpsGetter,
			SchemeHg:    hgGetter,
			SchemeSMB:   smbClientGetter,
		}

		var resolverOpts []GenericFetcherOption

		if b.tfRegistry != nil {
			fetchers[SchemeTFR] = b.tfRegistry
			resolverOpts = append(
				resolverOpts,
				WithDispatchLogger(b.logger),
				WithDispatchFS(b.tfRegistry.Venv.FS),
				WithDispatchVenv(b.tfRegistry.Venv),
				WithTFRConfig(b.tfRegistry.TofuImplementation),
			)
		}

		if b.oci != nil {
			fetchers[SchemeOCI] = b.oci
		}

		out = append(out,
			NewCASProtocolGetter(b.logger, b.casStore, b.v),
			NewCASGetter(b.logger, b.casStore, b.v, b.casCloneOpts,
				WithGenericFetchers(fetchers),
				WithGenericResolvers(DefaultSourceResolvers(b.v.WithHTTP(b.httpClient), resolverOpts...)),
			),
		)
	}

	out = append(out, b.prepended...)

	return append(out,
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
}

// newHTTPGetter constructs an HttpGetter with Netrc enabled (matching
// Terragrunt's historic behavior under v1's UpdateGetters customization)
// and an optional set of extra headers. Pass nil for `extra` to get the
// default getter; pass a non-nil header set to inject auth (used by
// WithHTTPAuth and WithHTTPSAuth for GitHub release downloads).
//
// c is the venv HTTP client so http(s) fetches stay on the venv rather
// than escaping to go-getter's default client; a nil c preserves that
// default.
//
// XTerraformGet is left enabled (the default) so X-Terraform-Get
// redirects continue to work.
func newHTTPGetter(c vhttp.Client, extra http.Header) *getter.HttpGetter {
	return &getter.HttpGetter{Netrc: true, Header: extra, Client: c}
}
