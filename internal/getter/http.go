package getter

import (
	"context"
	"net/url"
	"strings"

	getter "github.com/hashicorp/go-getter/v2"
)

// HTTPSchemeGetter wraps a [getter.HttpGetter] so its Detect only matches
// one scheme. The upstream HttpGetter.Detect claims both http and https,
// so registering two HttpGetters for per-scheme auth would have the first
// shadow the second; the wrapper is what makes [WithHTTPAuth] and
// [WithHTTPSAuth] route to their intended slots.
type HTTPSchemeGetter struct {
	Inner  *getter.HttpGetter
	Scheme string
}

// Get delegates to the inner getter.
func (g *HTTPSchemeGetter) Get(ctx context.Context, req *getter.Request) error {
	return g.Inner.Get(ctx, req)
}

// GetFile delegates to the inner getter.
func (g *HTTPSchemeGetter) GetFile(ctx context.Context, req *getter.Request) error {
	return g.Inner.GetFile(ctx, req)
}

// Mode delegates to the inner getter.
func (g *HTTPSchemeGetter) Mode(ctx context.Context, u *url.URL) (getter.Mode, error) {
	return g.Inner.Mode(ctx, u)
}

// Detect claims only requests whose scheme (or forced-getter prefix)
// equals [HTTPSchemeGetter.Scheme]. URLs are canonical lowercase by the
// time they reach this getter, so the comparison is case-sensitive.
func (g *HTTPSchemeGetter) Detect(req *getter.Request) (bool, error) {
	if req.Forced != "" {
		return req.Forced == g.Scheme, nil
	}

	return strings.HasPrefix(req.Src, g.Scheme+"://"), nil
}
