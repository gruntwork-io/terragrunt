package getter

import (
	"context"

	getter "github.com/hashicorp/go-getter/v2"
)

// NewClient returns a *go-getter/v2.Client configured for Terragrunt.
//
// Prefer using this over manually constructing a *go-getter/v2.Client
// directly, as it will consistently configure the client with the
// default protocol set (s3, gcs, git, hg, smb, http(s), file) plus
// the FileCopy and tfr customizations.
func NewClient(opts ...Option) *getter.Client {
	b := &builder{}
	for _, opt := range opts {
		opt(b)
	}

	c := &getter.Client{
		Getters: buildGetters(b),
	}

	if b.decompressors != nil {
		c.Decompressors = b.decompressors
	}

	return c
}

// Get is a convenience wrapper for downloading directories.
func Get(ctx context.Context, dst, src string, opts ...Option) (*GetResult, error) {
	return NewClient(opts...).Get(ctx, &Request{
		Src:     src,
		Dst:     dst,
		GetMode: ModeDir,
	})
}

// GetAny is a convenience wrapper for downloading either files or directories
// through a Terragrunt-configured client whose getter list includes s3 and gcs.
func GetAny(ctx context.Context, dst, src string, opts ...Option) (*GetResult, error) {
	return NewClient(opts...).Get(ctx, &Request{
		Src:     src,
		Dst:     dst,
		GetMode: ModeAny,
	})
}

// GetFile is a convenience wrapper for downloading a single file.
func GetFile(ctx context.Context, dst, src string, opts ...Option) (*GetResult, error) {
	return NewClient(opts...).Get(ctx, &Request{
		Src:     src,
		Dst:     dst,
		GetMode: ModeFile,
	})
}
