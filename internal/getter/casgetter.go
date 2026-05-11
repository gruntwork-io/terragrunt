package getter

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	getter "github.com/hashicorp/go-getter/v2"
)

// ErrDirectoryNotFound is returned when CASGetter cannot stat a local source.
var ErrDirectoryNotFound = errors.New("directory not found")

// SchemeGit is the forced-getter marker for git sources.
const SchemeGit = "git"

// CASGetter is the go-getter implementation that routes git, local, and
// configured non-git sources through Terragrunt's content-addressable store.
type CASGetter struct {
	CAS       *cas.CAS
	Logger    log.Logger
	Opts      *cas.CloneOptions
	Venv      cas.Venv
	fetchers  map[string]getter.Getter
	resolvers map[string]cas.SourceResolver
	Detectors []Detector
}

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

// WithDefaultGenericDispatch is the shorthand for the canonical pairing of
// [WithGenericFetchers]([DefaultGenericFetchers]) and [WithGenericResolvers]
// ([DefaultSourceResolvers]). fetcherOpts are forwarded so HTTP auth headers
// still reach the fetcher.
func WithDefaultGenericDispatch(fetcherOpts ...GenericFetcherOption) CASGetterOption {
	return func(g *CASGetter) {
		g.fetchers = DefaultGenericFetchers(fetcherOpts...)
		g.resolvers = DefaultSourceResolvers()
	}
}

// NewCASGetter constructs a CASGetter with the standard detector chain for
// git and file canonicalization. Pass [WithDefaultGenericDispatch] (or
// [WithGenericFetchers] + [WithGenericResolvers]) to enable the non-git
// dispatch path.
func NewCASGetter(l log.Logger, c *cas.CAS, v cas.Venv, opts *cas.CloneOptions, options ...CASGetterOption) *CASGetter {
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
		CAS:    c,
		Logger: l,
		Opts:   opts,
		Venv:   v,
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
// invoking Get.
func (g *CASGetter) Detect(req *getter.Request) (bool, error) {
	if req.Forced == SchemeGit {
		return true, nil
	}

	if after, ok := strings.CutPrefix(req.Src, "git::"); ok {
		req.Src = after
		req.Forced = SchemeGit

		return true, nil
	}

	if scheme, ok := g.matchGenericScheme(req); ok {
		req.Forced = scheme
		req.Src = appendDisableArchive(req.Src)

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
// generic path. req.Forced (set by the outer client when it stripped a
// "<scheme>::" prefix) wins; otherwise the URL scheme is consulted.
//
// URL-scheme claiming is restricted to http and https. The bare go-getter
// v2 protocol getters for s3, gcs, hg, smb reject `<scheme>://...` URLs
// (they expect canonical HTTPS forms or the forced-prefix syntax), so
// claiming those schemes here would set up a doomed inner fetch on every
// cache miss.
func (g *CASGetter) matchGenericScheme(req *getter.Request) (string, bool) {
	if g.fetchers == nil {
		return "", false
	}

	if scheme, ok := g.lookupFetcher(strings.ToLower(req.Forced)); ok {
		return scheme, true
	}

	u, err := url.Parse(req.Src)
	if err != nil || u.Scheme == "" {
		return "", false
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case SchemeHTTP, SchemeHTTPS:
		if _, ok := g.fetchers[scheme]; ok {
			return scheme, true
		}
	}

	return "", false
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

// appendDisableArchive adds archive=false to the URL query, preserving
// any existing value. The marker tells the outer v2 client to skip its
// archive-extension pre-decompression so req.Dst reaches Get pointing
// at the original destination instead of a temporary archive path.
func appendDisableArchive(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	q := u.Query()
	if q.Get("archive") != "" {
		return rawURL
	}

	q.Set("archive", "false")
	u.RawQuery = q.Encode()

	return u.String()
}

// stripDisableArchive removes the archive=false marker before handing the
// URL to the inner client so archive extension detection runs there.
func stripDisableArchive(u *url.URL) string {
	if u == nil {
		return ""
	}

	clone := *u
	q := clone.Query()

	if q.Get("archive") == "false" {
		q.Del("archive")
		clone.RawQuery = q.Encode()
	}

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

	// Copy so concurrent Get calls against the same getter don't race
	// on Branch/Dir mutation.
	opts := *g.Opts
	opts.Branch = ref
	opts.Dir = req.Dst

	return g.CAS.Clone(ctx, g.Logger, g.Venv, &opts, GitCloneURL(u.String()))
}

// getGeneric routes a non-git source through CAS. The archive=false
// marker Detect injected gets stripped before passing the URL to the
// inner getter.Client so archive extraction runs there.
func (g *CASGetter) getGeneric(ctx context.Context, req *getter.Request) error {
	scheme, ok := g.lookupFetcher(strings.ToLower(req.Forced))
	if !ok {
		return fmt.Errorf("CASGetter: no fetcher registered for scheme %q", strings.ToLower(req.Forced))
	}

	bare := g.fetchers[scheme]

	innerURL := stripDisableArchive(req.URL())

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
// fresh temp directory through a single-getter inner [getter.Client] and
// ingests the result via [cas.CAS.IngestDirectory]. The inner client uses
// the default decompressor map so `.tar.gz`/`.zip` URLs extract before
// ingest.
//
// scheme is set on the inner request's Forced field so the bare
// scheme-specific getter still claims the request. The bare go-getter v2
// s3 and gcs getters reject `http://`/`gs://` URLs unless Forced matches
// their validScheme; without this the inner client falls through with a
// generic "error downloading".
func (g *CASGetter) buildInnerFetch(bare getter.Getter, scheme, urlStr string) cas.SourceFetcher {
	return func(ctx context.Context, l log.Logger, v cas.Venv, suggestedKey string) (string, error) {
		tempDir, cleanup, err := g.CAS.MakeFetchTempDir(l, v)
		if err != nil {
			return "", err
		}

		defer cleanup()

		inner := &getter.Client{
			Getters: []getter.Getter{bare},
		}

		if _, err := inner.Get(ctx, &getter.Request{
			Src:     urlStr,
			Dst:     tempDir,
			Forced:  scheme,
			GetMode: getter.ModeAny,
		}); err != nil {
			return "", err
		}

		return g.CAS.IngestDirectory(l, v, tempDir, suggestedKey)
	}
}
