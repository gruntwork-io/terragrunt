package getter

import (
	"net/http"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Option mutates a Client builder.
type Option func(*builder)

// WithLogger sets a default logger used by getters that don't carry their own.
func WithLogger(l log.Logger) Option {
	return func(b *builder) { b.logger = l }
}

// WithFileCopy substitutes the default file-protocol getter with the supplied
// FileCopyGetter, which copies directories instead of symlinking them. Use
// NewFileCopyGetter to build one with sensible defaults.
func WithFileCopy(g *FileCopyGetter) Option {
	return func(b *builder) { b.fileCopy = g }
}

// WithTFRegistry registers the supplied RegistryGetter for tfr:// sources.
// Use NewRegistryGetter to build one with sensible defaults.
func WithTFRegistry(g *RegistryGetter) Option {
	return func(b *builder) { b.tfRegistry = g }
}

// WithCAS registers CASGetter, which intercepts git/file sources and routes
// them through Terragrunt's content-addressable storage. v supplies the
// filesystem and git runner used by every CAS operation.
func WithCAS(c *cas.CAS, v cas.Venv, cloneOpts *cas.CloneOptions) Option {
	return func(b *builder) {
		b.casStore = c
		b.casVenv = v
		b.casCloneOpts = cloneOpts
	}
}

// WithHTTPSAuth substitutes the https getter with an HttpGetter that sends
// the supplied extra headers (e.g. an Authorization bearer token) on every
// request, while still honoring .netrc. The plain-http getter is left at its
// default so bearer tokens can't leak over an unencrypted redirect.
func WithHTTPSAuth(extra http.Header) Option {
	return func(b *builder) { b.httpsExtraHeader = extra }
}

// WithHTTPAuth substitutes the plain-http getter with an HttpGetter that
// sends the supplied extra headers on every request, while still honoring
// .netrc. Intended for tests that talk to [net/http/httptest.Server], which
// serves over plain HTTP. Production callers should prefer WithHTTPSAuth.
func WithHTTPAuth(extra http.Header) Option {
	return func(b *builder) { b.httpExtraHeader = extra }
}

// WithDecompressors overrides the default decompressor map. To disable
// archive decompression entirely (used by github.Client which downloads
// release assets verbatim), pass a non-nil empty map. A nil map is ignored
// (go-getter's default decompressors are kept) because the option carries
// no signal distinguishing "don't override" from "override with nothing".
func WithDecompressors(d map[string]Decompressor) Option {
	return func(b *builder) {
		if d == nil {
			return
		}

		b.decompressors = d
	}
}

// WithCustomGettersPrepended inserts ad-hoc Getters at the front of the list
// for tests or one-off integrations.
func WithCustomGettersPrepended(g ...Getter) Option {
	return func(b *builder) { b.prepended = append(b.prepended, g...) }
}

// builder accumulates Options before they're realized into a *Client by
// NewClient. It is unexported because callers should always pass options
// rather than poke at the builder directly.
type builder struct {
	logger log.Logger

	fileCopy         *FileCopyGetter
	tfRegistry       *RegistryGetter
	casStore         *cas.CAS
	casCloneOpts     *cas.CloneOptions
	casVenv          cas.Venv
	httpExtraHeader  http.Header
	httpsExtraHeader http.Header
	decompressors    map[string]Decompressor
	prepended        []Getter
}
