package getter

import (
	"context"
	"net/url"

	getter "github.com/hashicorp/go-getter/v2"
)

// defaultGitDetectors mirrors the v2 default upstream git getter detector chain.
func defaultGitDetectors() []Detector {
	return []Detector{
		new(GitHubDetector),
		new(GitDetector),
		new(BitBucketDetector),
		new(GitLabDetector),
	}
}

// newDefaultUpstreamGitGetter constructs the upstream go-getter/v2 GitGetter
// wired with the default detector chain. GitGetter wraps an instance of this
// to flip DisableSymlinks per-request.
func newDefaultUpstreamGitGetter() *getter.GitGetter {
	return &getter.GitGetter{Detectors: defaultGitDetectors()}
}

// GitGetter is Terragrunt's git-protocol getter. It wraps the upstream
// go-getter/v2 GitGetter and forces req.DisableSymlinks=false on every Get,
// so symlinks inside cloned trees survive the copy.
//
// go-getter v2's bare GitGetter honors req.DisableSymlinks per-request and
// defaults the flag to "disabled". Terragrunt sources are user-configured,
// not arbitrary downloads, so we override that default at request time and
// restore the caller's value on return.
type GitGetter struct {
	inner getter.Getter
}

// NewGitGetter returns a GitGetter wrapping a default upstream git getter.
func NewGitGetter() *GitGetter {
	return &GitGetter{inner: newDefaultUpstreamGitGetter()}
}

// WithInner overrides the wrapped git getter so tests can pin the
// symlink-forcing behavior without standing up a real git remote.
func (g *GitGetter) WithInner(inner getter.Getter) *GitGetter {
	g.inner = inner
	return g
}

// Get forces req.DisableSymlinks=false for the duration of the inner Get call.
func (g *GitGetter) Get(ctx context.Context, req *getter.Request) error {
	saved := req.DisableSymlinks
	req.DisableSymlinks = false

	defer func() { req.DisableSymlinks = saved }()

	return g.inner.Get(ctx, req)
}

// GetFile is a passthrough; git getters don't fetch single files but the
// interface requires the method.
func (g *GitGetter) GetFile(ctx context.Context, req *getter.Request) error {
	return g.inner.GetFile(ctx, req)
}

// Mode delegates to the inner git getter.
func (g *GitGetter) Mode(ctx context.Context, u *url.URL) (getter.Mode, error) {
	return g.inner.Mode(ctx, u)
}

// Detect delegates to the inner git getter.
func (g *GitGetter) Detect(req *getter.Request) (bool, error) {
	return g.inner.Detect(req)
}
