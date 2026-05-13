package getter

import (
	"context"
	"net/url"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	getter "github.com/hashicorp/go-getter/v2"
)

// SourceManifestName is the manifest written when a local source directory is
// copied via FileCopyGetter. Tracks files that should be cleaned up by a
// later run.
const SourceManifestName = ".terragrunt-source-manifest"

// ErrSourceNotADirectory is returned by FileCopyGetter.Get when the source
// path resolves to a file rather than a directory. Exported so callers can
// match on it via errors.Is.
var ErrSourceNotADirectory = errors.New("source path must be a directory")

// FileCopyGetter is the file-protocol Getter Terragrunt uses in place of
// go-getter's default FileGetter. The default FileGetter creates symlinks
// (faster, less disk), but symlinks misbehave on Windows and cause infinite
// loops when source dirs nest, so we copy instead.
//
// FileCopyGetter implements the v2 [getter.Getter] interface. Construct via
// newFileCopyGetter so FS is populated.
type FileCopyGetter struct {
	Logger          log.Logger
	FS              vfs.FS
	IncludeInCopy   []string
	ExcludeFromCopy []string
	FastCopy        bool
}

// Get copies the source directory referenced by req into req.Dst.
func (g *FileCopyGetter) Get(_ context.Context, req *getter.Request) error {
	u := req.URL()

	path := u.Path
	if u.RawPath != "" {
		path = u.RawPath
	}

	fi, err := g.FS.Stat(path)
	if err != nil {
		return errors.Errorf("source path error: %s", err)
	}

	if !fi.IsDir() {
		return ErrSourceNotADirectory
	}

	copyOpts := []util.CopyOption{
		util.WithIncludeInCopy(g.IncludeInCopy...),
		util.WithExcludeFromCopy(g.ExcludeFromCopy...),
	}
	if g.FastCopy {
		copyOpts = append(copyOpts, util.WithFastCopy())
	}

	return util.CopyFolderContents(g.Logger, path, req.Dst, SourceManifestName, copyOpts...)
}

// GetFile copies a single file. We delegate to v2's FileGetter with Copy=true
// so we don't have to reimplement the file-copy details.
func (g *FileCopyGetter) GetFile(ctx context.Context, req *getter.Request) error {
	clone := *req
	clone.Copy = true

	if err := (&getter.FileGetter{}).GetFile(ctx, &clone); err != nil {
		return errors.Errorf("failed to copy file to %s: %w", req.Dst, err)
	}

	return nil
}

// Mode delegates to v2's FileGetter so the directory/file probe matches the
// stock implementation exactly.
func (g *FileCopyGetter) Mode(ctx context.Context, u *url.URL) (getter.Mode, error) {
	return (&getter.FileGetter{}).Mode(ctx, u)
}

// Detect delegates to v2's FileGetter so the URL canonicalization matches the
// stock implementation exactly.
func (g *FileCopyGetter) Detect(req *getter.Request) (bool, error) {
	return (&getter.FileGetter{}).Detect(req)
}

// NewFileCopyGetter returns a FileCopyGetter backed by the supplied
// filesystem. Use the With* methods to customize other behavior.
func NewFileCopyGetter(fs vfs.FS) *FileCopyGetter {
	return &FileCopyGetter{FS: fs}
}

// WithLogger sets the logger used by [util.CopyFolderContents] during a copy.
func (g *FileCopyGetter) WithLogger(l log.Logger) *FileCopyGetter {
	g.Logger = l
	return g
}

// WithIncludeInCopy sets the glob patterns that should be included in the
// copy even when [util.CopyFolderContents] would skip them by default
// (e.g. hidden folders).
func (g *FileCopyGetter) WithIncludeInCopy(patterns ...string) *FileCopyGetter {
	g.IncludeInCopy = patterns
	return g
}

// WithExcludeFromCopy sets the glob patterns to exclude from the copy.
func (g *FileCopyGetter) WithExcludeFromCopy(patterns ...string) *FileCopyGetter {
	g.ExcludeFromCopy = patterns
	return g
}

// WithFastCopy routes [util.CopyFolderContents] through its fast-copy path,
// driven by the `fast-copy` strict control.
func (g *FileCopyGetter) WithFastCopy(enabled bool) *FileCopyGetter {
	g.FastCopy = enabled
	return g
}
