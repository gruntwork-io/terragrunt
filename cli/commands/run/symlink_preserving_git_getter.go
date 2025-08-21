package run

import (
	"net/url"

	"github.com/hashicorp/go-getter"
)

// Since go-getter v1.7.9, symbolic links are disabled by default and are automatically
// disabled during git submodule operations. This wrapper preserves the original
// DisableSymlinks setting to ensure symlinks remain enabled when configured.

// symlinkPreservingGitGetter wraps the original git getter to preserve symlink settings
type symlinkPreservingGitGetter struct {
	original getter.Getter
	client   *getter.Client
}

// Get overrides the original GitGetter to preserve symlink settings
func (g *symlinkPreservingGitGetter) Get(dst string, u *url.URL) error {
	// Store the original DisableSymlinks setting
	originalDisableSymlinks := g.client.DisableSymlinks

	// Call the original getter
	err := g.original.Get(dst, u)

	// Restore the original DisableSymlinks setting
	g.client.DisableSymlinks = originalDisableSymlinks

	return err
}

// GetFile overrides the original GitGetter to preserve symlink settings
func (g *symlinkPreservingGitGetter) GetFile(dst string, u *url.URL) error {
	return g.original.GetFile(dst, u)
}

// ClientMode overrides the original GitGetter to preserve symlink settings
func (g *symlinkPreservingGitGetter) ClientMode(u *url.URL) (getter.ClientMode, error) {
	return g.original.ClientMode(u)
}

// SetClient overrides the original GitGetter to preserve symlink settings
func (g *symlinkPreservingGitGetter) SetClient(c *getter.Client) {
	g.client = c
	g.original.SetClient(c)
}
