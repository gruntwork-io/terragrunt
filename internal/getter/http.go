package getter

import (
	"context"
	"net/url"
	"strings"

	getter "github.com/hashicorp/go-getter/v2"
)

// HTTPSchemeGetter wraps an [getter.HttpGetter] so its Detect only matches a
// specific scheme. Two of these (one for "http", one for "https") are
// registered by [buildGetters] so the per-scheme auth headers configured via
// [WithHTTPAuth] and [WithHTTPSAuth] route to the correct slot.
//
// Without the wrapper the upstream HttpGetter.Detect matches both http and
// https schemes, so the first registered instance wins for both and the
// second slot's auth headers never reach the wire.
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

// Detect returns true only when the request's scheme (or forced-getter
// prefix) matches the configured scheme.
//
// The prefix check is case-sensitive. URLs reach this point through
// Terragrunt's detector chain in canonical lowercase form, so the
// case-sensitive check is intentional.
func (g *HTTPSchemeGetter) Detect(req *getter.Request) (bool, error) {
	if req.Forced != "" {
		return req.Forced == g.Scheme, nil
	}

	return strings.HasPrefix(req.Src, g.Scheme+"://"), nil
}
