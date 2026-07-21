package getter

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"errors"

	getter "github.com/hashicorp/go-getter/v2"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ErrDirectoryNotFound is returned when CASGetter cannot stat a local source.
var ErrDirectoryNotFound = errors.New("directory not found")

// SchemeGit is the forced-getter marker for git sources.
const SchemeGit = "git"

// CASGetter is the go-getter implementation that routes git, local, and
// configured non-git sources through Terragrunt's content-addressable store.
type CASGetter struct {
	CAS         *cas.CAS
	Logger      log.Logger
	Opts        *cas.CloneOptions
	Venv        venv.Venv
	fetchers    map[string]getter.Getter
	resolvers   map[string]cas.SourceResolver
	innerClient InnerClientBuilder
	Detectors   []Detector

	// userDisabledArchive records that the source for the current
	// request carried an explicit archive=false. Detect sets it; Get
	// reads it when building the inner fetch URL. CAS injects archive=false
	// itself so the outer v2 client skips pre-decompression, which means
	// the URL alone cannot tell a user's "do not extract" from CAS's own
	// marker. This field keeps them apart.
	//
	// Detect always runs immediately before Get within a single outer
	// client.Get, and production callers build a CASGetter per download
	// (runner/run/download_source.go), so the value is never read across
	// concurrent requests. Do not share one CASGetter across goroutines.
	userDisabledArchive bool
}

// InnerClientBuilder builds the per-fetch [getter.Client] used to invoke
// the bare scheme-specific getter from a [SourceFetcher]. Schemes that
// download in a single step want a one-getter client; multi-stage
// protocols (notably tfr://, where [RegistryGetter] resolves an archive
// URL and delegates the actual download through
// [getter.ClientFromContext]) need a richer client.
type InnerClientBuilder func(bare getter.Getter, scheme string) *getter.Client

// CASGetterOption mutates a CASGetter at construction time.
type CASGetterOption func(*CASGetter)

// WithGenericFetchers registers a scheme→getter map for non-git
// sources. Schemes not in the map fall through to whichever bare
// go-getter the outer client registers next.
func WithGenericFetchers(m map[string]getter.Getter) CASGetterOption {
	return func(g *CASGetter) { g.fetchers = m }
}

// WithGenericResolvers registers a scheme→resolver map for probing
// non-git sources. Schemes not in the map go through the fetch path
// without a probe (download then content-hash).
func WithGenericResolvers(m map[string]cas.SourceResolver) CASGetterOption {
	return func(g *CASGetter) { g.resolvers = m }
}

// WithInnerClientBuilder overrides the per-fetch inner [getter.Client]
// builder. The default builder is sufficient for production wiring;
// tests use this hook to inject a TLS-trusting [getter.HttpGetter] for
// the delegated tfr:// archive download.
func WithInnerClientBuilder(b InnerClientBuilder) CASGetterOption {
	return func(g *CASGetter) { g.innerClient = b }
}

// WithDefaultGenericDispatch is the shorthand for the canonical pairing of
// [WithGenericFetchers]([DefaultGenericFetchers]) and [WithGenericResolvers]
// ([DefaultSourceResolvers]). opts are forwarded to both helpers so HTTP
// auth headers reach the fetcher and tfr config reaches both.
func WithDefaultGenericDispatch(opts ...GenericFetcherOption) CASGetterOption {
	return func(g *CASGetter) {
		g.fetchers = DefaultGenericFetchers(opts...)
		g.resolvers = DefaultSourceResolvers(opts...)
	}
}

// NewCASGetter constructs a CASGetter with the standard detector chain for
// git and file canonicalization. Pass [WithDefaultGenericDispatch] (or
// [WithGenericFetchers] + [WithGenericResolvers]) to enable the non-git
// dispatch path.
//
// Requires v.FS and v.Exec: the file-path branch in Detect stats through
// v.FS, and the git-path branch in Get clones through a runner derived
// from v.Exec. Panics with [venv.ErrVenvFSUnset] or [venv.ErrVenvExecUnset]
// respectively when either is nil. Production callers thread the root
// [venv.Venv], which always supplies both.
func NewCASGetter(
	l log.Logger,
	c *cas.CAS,
	v venv.Venv,
	opts *cas.CloneOptions,
	options ...CASGetterOption,
) *CASGetter {
	v.RequireFS()
	v.RequireExec()

	if opts == nil {
		opts = &cas.CloneOptions{}
	}

	g := &CASGetter{
		Detectors: []Detector{
			new(GitHubDetector),
			new(GitDetector),
			new(BitBucketDetector),
			new(GitLabDetector),
			new(FileDetector),
		},
		CAS:         c,
		Logger:      l,
		Opts:        opts,
		Venv:        v,
		innerClient: defaultInnerClientBuilder,
	}

	for _, opt := range options {
		opt(g)
	}

	return g
}

// Get clones (or copies) the source into the CAS store and links it into
// req.Dst. Behavior is selected by req.Forced (the scheme).
func (g *CASGetter) Get(ctx context.Context, req *getter.Request) error {
	if req.Copy {
		// Local directory.
		var linkOpts []cas.LinkTreeOption
		if g.Opts.Mutable {
			linkOpts = append(linkOpts, cas.WithForceCopy())
		}

		return g.CAS.StoreLocalDirectory(ctx, g.Logger, g.Venv, req.Src, req.Dst, linkOpts...)
	}

	if g.isGenericScheme(req.Forced) {
		return g.getGeneric(ctx, req)
	}

	return g.getGit(ctx, req)
}

// GetFile is not supported for the CAS getter.
func (g *CASGetter) GetFile(_ context.Context, _ *getter.Request) error {
	return cas.ErrGetFileNotSupported
}

// Mode reports directory mode for all CAS sources.
func (g *CASGetter) Mode(_ context.Context, _ *url.URL) (getter.Mode, error) {
	return getter.ModeDir, nil
}

// Detect canonicalizes the source via the detector chain. Local
// sources get req.Copy = true so Get takes the StoreLocalDirectory
// path. Non-git schemes covered by Fetchers get archive=false appended
// to the URL so the outer client does not pre-decompress before
// invoking Get; a source that already disabled archiving is recorded in
// userDisabledArchive so Get can keep the inner fetch from extracting.
func (g *CASGetter) Detect(req *getter.Request) (bool, error) {
	if req.Forced == SchemeGit {
		return true, nil
	}

	if after, ok := strings.CutPrefix(req.Src, "git::"); ok {
		req.Src = after
		req.Forced = SchemeGit

		return true, nil
	}

	if scheme, src, ok := g.matchGenericScheme(req); ok {
		req.Forced = scheme
		req.Src, g.userDisabledArchive = disableOuterArchive(src)

		return true, nil
	}

	for _, detector := range g.Detectors {
		src, ok, err := detector.Detect(req.Src, req.Pwd)
		if err != nil {
			return false, err
		}

		if !ok {
			continue
		}

		if _, isFileDetector := detector.(*getter.FileDetector); isFileDetector {
			// Repeats the NewCASGetter check so a caller that hand-
			// assembles a CASGetter and skips the constructor still
			// gets the typed panic instead of a runtime nil-deref.
			g.Venv.RequireFS()

			info, statErr := g.Venv.FS.Stat(src)
			if statErr != nil {
				return false, fmt.Errorf("%w: %s", ErrDirectoryNotFound, src)
			}

			if !info.IsDir() {
				return false, fmt.Errorf("%w: %s", cas.ErrNotADirectory, src)
			}

			// Indicates a local directory to Get.
			req.Copy = true
		}

		req.Src = src

		return true, nil
	}

	return false, nil
}

// GitCloneURL turns a v2-detected URL string into a clone target the
// underlying git client accepts.
//
// Two normalizations are needed:
//
//  1. Strip a leading "git::". The v2 outer client only splits the
//     forced prefix into req.Forced when the source carried it on
//     entry; when CASGetter.Detect runs its own detector chain (e.g.
//     for github shorthand or git@host:path SCP), the v2 GitDetector
//     reattaches "git::" to its result, and req.URL().String()
//     preserves it. Passing it through to git makes git look up the
//     missing "git-remote-git" helper.
//  2. Convert "ssh://git@host/path" into the SCP-style
//     "git@host:path" git expects for SSH cloning. URLs that carry an
//     explicit port (e.g. "ssh://git@host:2222/path") keep the URL
//     form because git's SCP shorthand has no syntax for a port.
func GitCloneURL(urlStr string) string {
	urlStr = strings.TrimPrefix(urlStr, "git::")

	if !strings.HasPrefix(urlStr, "ssh://") {
		return urlStr
	}

	if u, err := url.Parse(urlStr); err == nil && u.Port() != "" {
		return urlStr
	}

	after := strings.TrimPrefix(urlStr, "ssh://")

	return strings.Replace(after, "/", ":", 1)
}

// matchGenericScheme reports whether req should route through the non-git
// generic path and returns the scheme plus the (possibly canonicalized)
// source URL. req.Forced (set by the outer client when it stripped a
// "<scheme>::" prefix) wins; otherwise the URL scheme is consulted.
//
// URL-scheme claiming is restricted to http, https, tfr, and oci. The
// bare go-getter v2 protocol getters for s3, gcs, hg, smb reject
// `<scheme>://...` URLs (they expect canonical HTTPS forms or the
// forced-prefix syntax), so claiming those schemes here would set up a
// doomed inner fetch on every cache miss. The tfr ([RegistryGetter]) and
// oci ([OCIGetter]) fetchers accept their scheme URLs natively.
//
// HTTPS URLs against AWS S3 hosts are an exception: virtual-host forms
// (`<bucket>.s3.amazonaws.com/<key>`) would route through the HTTPS
// fetcher and bypass S3 auth, so the matcher rewrites them to the path-
// style form the bare s3 getter accepts and claims the s3 scheme.
func (g *CASGetter) matchGenericScheme(req *getter.Request) (string, string, bool) {
	if g.fetchers == nil {
		return "", req.Src, false
	}

	if scheme, ok := g.lookupFetcher(strings.ToLower(req.Forced)); ok {
		src := req.Src

		// A forced s3 prefix with an AWS virtual-host URL still needs
		// the rewrite, since the bare s3 getter rejects virtual-host
		// hosts regardless of how the scheme was claimed.
		if scheme == SchemeS3 {
			if u, perr := url.Parse(req.Src); perr == nil {
				if canonical, cok := canonicalAWSS3HTTPSURL(u); cok {
					src = canonical
				}
			}
		}

		return scheme, src, true
	}

	u, err := url.Parse(req.Src)
	if err != nil || u.Scheme == "" {
		return "", req.Src, false
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case SchemeHTTP, SchemeHTTPS, SchemeTFR, SchemeOCI:
		if canonical, ok := canonicalAWSS3HTTPSURL(u); ok {
			if _, fok := g.fetchers[SchemeS3]; fok {
				return SchemeS3, canonical, true
			}
		}

		if _, ok := g.fetchers[scheme]; ok {
			return scheme, req.Src, true
		}
	}

	return "", req.Src, false
}

// isGenericScheme reports whether the forced scheme corresponds to a
// fetcher registered for the generic (non-git) dispatch path.
func (g *CASGetter) isGenericScheme(forced string) bool {
	if g.fetchers == nil {
		return false
	}

	_, ok := g.lookupFetcher(strings.ToLower(forced))

	return ok
}

// lookupFetcher resolves scheme (or an alias of it) to a fetcher entry
// and returns the registry key on a hit. "gs" maps to "gcs"; all other
// inputs are taken as the registry key directly.
func (g *CASGetter) lookupFetcher(scheme string) (string, bool) {
	if scheme == "gs" {
		scheme = SchemeGCS
	}

	if _, ok := g.fetchers[scheme]; ok {
		return scheme, true
	}

	return "", false
}

// disableOuterArchive ensures rawURL carries archive=false so the outer
// v2 client skips its archive-extension pre-decompression and req.Dst
// reaches Get pointing at the original destination instead of a
// temporary archive path. It reports whether the caller had already
// disabled archiving: a pre-existing archive value is left untouched,
// and the second return is true only when that value parses to a false
// boolean. Get needs the distinction because the marker CAS injects and
// a user's explicit archive=false are indistinguishable once the outer
// client strips the parameter.
func disableOuterArchive(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, false
	}

	q := u.Query()

	if v := q.Get("archive"); v != "" {
		disabled, perr := strconv.ParseBool(v)
		return rawURL, perr == nil && !disabled
	}

	q.Set("archive", "false")
	u.RawQuery = q.Encode()

	return u.String(), false
}

// innerArchiveURL builds the URL for the inner single-getter client. The
// outer v2 client consumes and removes the archive parameter before Get
// runs, so neither CAS's injected marker nor a user's archive=false
// survives on its own. When userDisabled is set, archive=false is
// re-applied so the inner client also skips extraction; otherwise the
// URL carries no archive parameter and the inner client's extension-based
// detection extracts .tar.gz/.zip sources before ingest.
func innerArchiveURL(u *url.URL, userDisabled bool) string {
	if u == nil {
		return ""
	}

	clone := *u
	q := clone.Query()
	q.Del("archive")

	if userDisabled {
		q.Set("archive", "false")
	}

	clone.RawQuery = q.Encode()

	return clone.String()
}

// getGit clones via [cas.CAS.Clone] after lifting ?ref= out of the URL
// into [cas.CloneOptions.Branch].
func (g *CASGetter) getGit(ctx context.Context, req *getter.Request) error {
	ref := ""

	u := req.URL()

	q := u.Query()
	if len(q) > 0 {
		ref = q.Get("ref")
		q.Del("ref")

		u.RawQuery = q.Encode()
	}

	return g.CAS.Clone(ctx, g.Logger, g.Venv, GitCloneURL(u.String()),
		cas.WithDir(req.Dst),
		cas.WithBranch(ref),
		cas.WithDepth(g.Opts.Depth),
		cas.WithMutable(g.Opts.Mutable),
		cas.WithIncludedGitFiles(g.Opts.IncludedGitFiles))
}

// getGeneric routes a non-git source through CAS. The inner getter.Client
// performs archive extraction, so the URL passed to it carries archive=false
// only when the user asked to disable archiving; otherwise the marker
// Detect injected is dropped so extension-based extraction runs there.
func (g *CASGetter) getGeneric(ctx context.Context, req *getter.Request) error {
	scheme, ok := g.lookupFetcher(strings.ToLower(req.Forced))
	if !ok {
		return fmt.Errorf(
			"CASGetter: no fetcher registered for scheme %q",
			strings.ToLower(req.Forced),
		)
	}

	bare := g.fetchers[scheme]

	innerURL := innerArchiveURL(req.URL(), g.userDisabledArchive)

	opts := *g.Opts
	opts.Dir = req.Dst

	return g.CAS.FetchSource(ctx, g.Logger, g.Venv, &opts, cas.SourceRequest{
		Scheme:   scheme,
		URL:      innerURL,
		Resolver: g.resolvers[scheme],
		Fetch:    g.buildInnerFetch(bare, scheme, innerURL),
	})
}

// buildInnerFetch returns a SourceFetcher that downloads urlStr into a
// fresh temp directory through an inner [getter.Client] built by
// [InnerClientBuilder] and ingests the result via
// [cas.CAS.IngestDirectory]. The inner client uses the default
// decompressor map so `.tar.gz`/`.zip` URLs extract before ingest.
//
// scheme is set on the inner request's Forced field so the bare
// scheme-specific getter still claims the request. The bare go-getter v2
// s3 and gcs getters reject `http://`/`gs://` URLs unless Forced matches
// their validScheme; without this the inner client falls through with a
// generic "error downloading".
func (g *CASGetter) buildInnerFetch(bare getter.Getter, scheme, urlStr string) cas.SourceFetcher {
	return func(ctx context.Context, l log.Logger, v venv.Venv, suggestedKey string) (string, error) {
		tempDir, cleanup, err := g.CAS.MakeFetchTempDir(l, v)
		if err != nil {
			return "", err
		}

		defer cleanup()

		inner := g.innerClient(bare, scheme)

		fetchURL, treeKey := g.pinOCIDigest(ctx, scheme, urlStr, suggestedKey)

		if _, err := inner.Get(ctx, &getter.Request{
			Src:     fetchURL,
			Dst:     tempDir,
			Forced:  scheme,
			GetMode: getter.ModeAny,
		}); err != nil {
			return "", err
		}

		return g.CAS.IngestDirectory(l, v, tempDir, treeKey)
	}
}

// ociDigestResolver binds a mutable oci reference to the digest it resolves
// to at download time.
type ociDigestResolver interface {
	ResolveDigest(ctx context.Context, rawURL string) (string, error)
}

// pinOCIDigest rewrites a mutable oci reference to the digest it resolves to
// right now, so the download and the cache key name one immutable manifest
// and a tag moving mid-fetch can never be stored under a stale key.
func (g *CASGetter) pinOCIDigest(ctx context.Context, scheme, rawURL, suggestedKey string) (string, string) {
	if scheme != SchemeOCI {
		return rawURL, suggestedKey
	}

	resolver, ok := g.resolvers[scheme].(ociDigestResolver)
	if !ok {
		// No digest contract: content-hash rather than trust a mutable probe key.
		return rawURL, ""
	}

	digestValue, err := resolver.ResolveDigest(ctx, rawURL)
	if err != nil {
		// Unresolvable right now: content-hash instead of trusting the probe key.
		if g.Logger != nil {
			g.Logger.Debugf("OCI digest pin of %q failed, content-hashing instead: %v", rawURL, err)
		}

		return rawURL, ""
	}

	return pinnedOCIURL(rawURL, digestValue), cas.ContentKey(ociManifestKeyAlg, digestValue)
}

// pinnedOCIURL swaps a tag reference for the resolved digest pin.
func pinnedOCIURL(rawURL, digestValue string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	q := u.Query()
	q.Del(ociTagQueryKey)
	q.Set(ociDigestQueryKey, digestValue)
	u.RawQuery = q.Encode()

	return u.String()
}

// defaultInnerClientBuilder is the inner-client builder used when none
// is configured via [WithInnerClientBuilder]. For tfr it returns a
// Terragrunt client with the bare [RegistryGetter] prepended so the
// default protocol set is available for [RegistryGetter]'s delegated
// archive download. Every other scheme uses a single-getter client.
func defaultInnerClientBuilder(bare getter.Getter, scheme string) *getter.Client {
	if scheme == SchemeTFR {
		return NewClient(WithCustomGettersPrepended(bare))
	}

	return &getter.Client{Getters: []getter.Getter{bare}}
}
