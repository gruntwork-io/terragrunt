package getter

import (
	"context"

	getter "github.com/hashicorp/go-getter/v2"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// NewClient returns a *go-getter/v2.Client configured for Terragrunt.
//
// Prefer using this over manually constructing a *go-getter/v2.Client
// directly, as it will consistently configure the client with the
// default protocol set (s3, gcs, git, hg, smb, http(s), file) plus
// the FileCopy and tfr customizations.
//
// Every http(s) fetch goes through v's client, so no source reaches the
// network outside the venv. A caller fetching through a different client
// passes [WithHTTP].
func NewClient(l log.Logger, v *venv.Venv, opts ...Option) *getter.Client {
	v.RequireHTTP()

	b := &builder{logger: l, v: v, httpClient: v.HTTP}
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

// Download runs c.Get for req. When it fails, the error has req's source
// address redacted, both as the caller wrote it and as go-getter rewrote it.
func Download(ctx context.Context, c *Client, req *Request) (*GetResult, error) {
	src := req.Src

	res, err := c.Get(ctx, req)
	if err != nil {
		return nil, redactSourceError(err, src, req.Src)
	}

	return res, nil
}

// Get is a convenience wrapper for downloading directories.
func Get(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	dst, src string,
	opts ...Option,
) (*GetResult, error) {
	return Download(ctx, NewClient(l, v, opts...), &Request{
		Src:     src,
		Dst:     dst,
		GetMode: ModeDir,
	})
}

// GetAny is a convenience wrapper for downloading either files or directories
// through a Terragrunt-configured client whose getter list includes s3 and gcs.
func GetAny(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	dst, src string,
	opts ...Option,
) (*GetResult, error) {
	return Download(ctx, NewClient(l, v, opts...), &Request{
		Src:     src,
		Dst:     dst,
		GetMode: ModeAny,
	})
}

// GetFile is a convenience wrapper for downloading a single file.
func GetFile(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	dst, src string,
	opts ...Option,
) (*GetResult, error) {
	return Download(ctx, NewClient(l, v, opts...), &Request{
		Src:     src,
		Dst:     dst,
		GetMode: ModeFile,
	})
}
