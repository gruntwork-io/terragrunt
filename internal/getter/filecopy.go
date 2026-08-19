package getter

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"

	"errors"

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

// ErrSourceNotAFile is returned by FileCopyGetter.GetFile when the source path
// resolves to a directory rather than a file. Exported so callers can match on
// it via errors.Is.
var ErrSourceNotAFile = errors.New("source path must be a file")

// dstDirPerms is the mode given to parent directories created on the way to a
// single-file destination.
const dstDirPerms = 0o700

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
		return fmt.Errorf("source path error: %w", err)
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

	return util.CopyFolderContents(g.Logger, g.FS, path, req.Dst, SourceManifestName, copyOpts...)
}

// GetFile copies the single file referenced by req into req.Dst. v2's
// FileGetter would do the same, but it reaches for os directly, so a
// virtual filesystem would never see the write.
func (g *FileCopyGetter) GetFile(_ context.Context, req *getter.Request) error {
	u := req.URL()

	path := u.Path
	if u.RawPath != "" {
		path = u.RawPath
	}

	fi, err := g.FS.Stat(path)
	if err != nil {
		return fmt.Errorf("source path error: %w", err)
	}

	if fi.IsDir() {
		return ErrSourceNotAFile
	}

	if err := g.FS.MkdirAll(filepath.Dir(req.Dst), dstDirPerms); err != nil {
		return err
	}

	if err := vfs.CopyFile(g.FS, path, req.Dst); err != nil {
		return fmt.Errorf("failed to copy file to %s: %w", req.Dst, err)
	}

	return nil
}

// Mode reports whether the source path is a directory or a single file. v2's
// FileGetter probes the same way, but through os rather than the filesystem
// this getter was built with.
func (g *FileCopyGetter) Mode(_ context.Context, u *url.URL) (getter.Mode, error) {
	path := u.Path
	if u.RawPath != "" {
		path = u.RawPath
	}

	fi, err := g.FS.Stat(path)
	if err != nil {
		return 0, err
	}

	if fi.IsDir() {
		return getter.ModeDir, nil
	}

	return getter.ModeFile, nil
}

// Detect delegates to v2's FileGetter so the URL canonicalization matches the
// stock implementation exactly.
func (g *FileCopyGetter) Detect(req *getter.Request) (bool, error) {
	return (&getter.FileGetter{}).Detect(req)
}

// NewFileCopyGetter returns a FileCopyGetter backed by the supplied
// filesystem. Use the With* methods to customize other behavior.
func NewFileCopyGetter(fsys vfs.FS) *FileCopyGetter {
	return &FileCopyGetter{FS: fsys}
}

// WithLogger sets the logger used by [util.CopyFolderContents] during a copy.
func (g *FileCopyGetter) WithLogger(l log.Logger) *FileCopyGetter {
	g.Logger = l
	return g
}

// WithFS sets the filesystem every read and write of the copy runs through.
func (g *FileCopyGetter) WithFS(fsys vfs.FS) *FileCopyGetter {
	g.FS = fsys
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
