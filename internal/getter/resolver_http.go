package getter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cas"
)

// httpResolverTimeout caps the HEAD request so a slow remote can't stall
// CAS dispatch. On timeout the resolver returns ErrNoVersionMetadata
// and CAS falls back to content hashing.
const httpResolverTimeout = 10 * time.Second

// HTTPResolver is a [cas.SourceResolver] for HTTP and HTTPS URLs.
type HTTPResolver struct {
	// Client overrides the http.Client used for the HEAD probe.
	// Nil means a copy of http.DefaultClient with httpResolverTimeout.
	Client *http.Client
	// scheme is what Scheme() reports; set by [NewHTTPResolver] and
	// [NewHTTPSResolver].
	scheme string
}

// NewHTTPResolver returns a resolver for the http scheme.
func NewHTTPResolver() *HTTPResolver { return &HTTPResolver{scheme: "http"} }

// NewHTTPSResolver returns a resolver for the https scheme. The same
// type handles both; separate constructors keep the [SourceResolver]
// Scheme() contract honest for each instance.
func NewHTTPSResolver() *HTTPResolver { return &HTTPResolver{scheme: "https"} }

// Scheme returns the URL scheme this resolver handles ("http" or
// "https").
func (r *HTTPResolver) Scheme() string {
	if r.scheme == "" {
		return "http"
	}

	return r.scheme
}

// Probe HEADs rawURL and returns a URL-scoped opaque cache key derived
// from the ETag (preferred) or Last-Modified header.
//
// ETag is treated as opaque even when the server claims it is a strong
// content hash: there is no portable way to distinguish content hashes
// from server-assigned tokens. Network errors and non-2xx responses
// surface as [cas.ErrNoVersionMetadata].
func (r *HTTPResolver) Probe(ctx context.Context, rawURL string) (string, error) {
	client := r.Client
	if client == nil {
		c := *http.DefaultClient
		c.Timeout = httpResolverTimeout
		client = &c
	}

	// The outer client strips these before invoking the HTTP getter,
	// so probing with them attached would split cache entries that
	// resolve to the same fetched bytes.
	probeURL := stripHTTPMagicParams(rawURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, probeURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("build HEAD request for %s: %w", rawURL, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	key, probeErr := r.pickHTTPCacheKey(probeURL, resp)

	// Body close errors only surface on the success path; on a
	// probe failure the primary error already explains the outcome
	// and joining would mask the ErrNoVersionMetadata sentinel
	// callers test for.
	closeErr := resp.Body.Close()

	if probeErr != nil {
		return "", probeErr
	}

	if closeErr != nil {
		return "", fmt.Errorf("close HTTP response body for %s: %w", rawURL, closeErr)
	}

	return key, nil
}

// pickHTTPCacheKey reads the cache-key-bearing headers from resp and
// returns the OpaqueKey for the strongest available signal.
func (r *HTTPResolver) pickHTTPCacheKey(rawURL string, resp *http.Response) (string, error) {
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", cas.ErrNoVersionMetadata
	}

	scheme := r.Scheme()
	if u, parseErr := url.Parse(rawURL); parseErr == nil && u.Scheme != "" {
		scheme = strings.ToLower(u.Scheme)
	}

	if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" {
		if normalized := normalizeETag(etag); normalized != "" {
			return cas.OpaqueKey(scheme, rawURL, normalized), nil
		}
	}

	if lm := strings.TrimSpace(resp.Header.Get("Last-Modified")); lm != "" {
		return cas.OpaqueKey(scheme, rawURL, lm), nil
	}

	return "", cas.ErrNoVersionMetadata
}

// normalizeETag strips the weak-validator W/ prefix and the surrounding
// quotes so the same bytes served with either form produce the same
// cache key.
func normalizeETag(etag string) string {
	etag = strings.TrimPrefix(etag, "W/")
	etag = strings.TrimPrefix(etag, "w/")
	etag = strings.TrimPrefix(etag, "\"")
	etag = strings.TrimSuffix(etag, "\"")

	return etag
}

// httpMagicParams are the query keys the go-getter v2 outer client
// consumes itself rather than forwarding to the HTTP getter.
var httpMagicParams = []string{"archive", "checksum", "filename"}

// stripHTTPMagicParams returns rawURL with [httpMagicParams] removed.
// Unparsable inputs are returned unchanged so the HEAD request
// surfaces the same error a fetch would.
func stripHTTPMagicParams(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	q := u.Query()

	changed := false

	for _, k := range httpMagicParams {
		if q.Has(k) {
			q.Del(k)

			changed = true
		}
	}

	if !changed {
		return rawURL
	}

	u.RawQuery = q.Encode()

	return u.String()
}
