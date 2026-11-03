package getter

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	getter "github.com/hashicorp/go-getter/v2"
)

// ErrHgUnavailable is returned when the venv's exec cannot find an hg binary.
var ErrHgUnavailable = errors.New("hg must be available and on the PATH")

// HgGetter is Terragrunt's Mercurial getter.
//
// It downloads the way go-getter's hg getter does. It honors the rev query
// parameter and pulls into a destination that already exists.
//
// It runs every hg command through the venv's exec and reaches the filesystem
// through the venv's FS.
type HgGetter struct {
	v        *venv.Venv
	detector *getter.HgGetter
}

// NewHgGetter returns an HgGetter that runs hg through v.
func NewHgGetter(v *venv.Venv) *HgGetter {
	v.RequireExec()
	v.RequireFS()
	v.RequireEnv()
	v.RequireTempDir()
	v.RequireGOOS()

	return &HgGetter{v: v, detector: new(getter.HgGetter)}
}

// Mode reports that an hg source is always a directory.
func (g *HgGetter) Mode(_ context.Context, _ *url.URL) (getter.Mode, error) {
	return getter.ModeDir, nil
}

// Detect reports whether req's source is an hg address, using go-getter's hg
// detection.
func (g *HgGetter) Detect(req *getter.Request) (bool, error) {
	return g.detector.Detect(req)
}

// Get clones the repository at req's URL into req.Dst unless it is already
// there, pulls, and updates the working copy to the requested revision.
func (g *HgGetter) Get(ctx context.Context, req *getter.Request) error {
	if _, err := g.v.Exec.LookPath("hg"); err != nil {
		return fmt.Errorf("%w: %w", ErrHgUnavailable, err)
	}

	src := *req.URL()

	if src.Scheme == "file" && src.Host == "" {
		src.Path = FileURL(src.Path).Path
	}

	q := src.Query()
	rev := q.Get("rev")
	q.Del("rev")
	src.RawQuery = q.Encode()

	return redactSourceError(g.get(ctx, src.String(), req.Dst, rev), src.String())
}

// GetFile downloads the repository that contains the file at req's URL, then
// copies that file to req.Dst.
func (g *HgGetter) GetFile(ctx context.Context, req *getter.Request) error {
	return getFileFromCheckout(ctx, g.v, req, g.Get)
}

func (g *HgGetter) get(ctx context.Context, src, dst, rev string) error {
	_, statErr := g.v.FS.Stat(dst)

	switch {
	case errors.Is(statErr, fs.ErrNotExist):
		if _, err := runDownloadCommand(ctx, g.v, "", "hg", "clone", "-U", "--", src, dst); err != nil {
			return err
		}
	case statErr != nil:
		return statErr
	}

	if _, err := runDownloadCommand(ctx, g.v, dst, "hg", "pull"); err != nil {
		return err
	}

	args := []string{"update"}
	if rev != "" {
		args = append(args, "--", rev)
	}

	_, err := runDownloadCommand(ctx, g.v, dst, "hg", args...)

	return err
}
