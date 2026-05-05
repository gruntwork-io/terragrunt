package getter

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	getter "github.com/hashicorp/go-getter/v2"
)

// ErrDirectoryNotFound is returned when CASGetter cannot stat a local source.
var ErrDirectoryNotFound = errors.New("directory not found")

// CASGetter is the go-getter implementation that routes git/file sources
// through Terragrunt's content-addressable store.
type CASGetter struct {
	CAS       *cas.CAS
	Logger    log.Logger
	Opts      *cas.CloneOptions
	Detectors []Detector
}

// NewCASGetter constructs a CASGetter wired with the standard detector chain.
func NewCASGetter(l log.Logger, c *cas.CAS, opts *cas.CloneOptions) *CASGetter {
	return &CASGetter{
		Detectors: []Detector{
			new(GitHubDetector),
			new(GitDetector),
			new(BitBucketDetector),
			new(GitLabDetector),
			new(FileDetector),
		},
		CAS:    c,
		Logger: l,
		Opts:   opts,
	}
}

// Get clones (or copies, for local sources) the source into the CAS store and
// links it into req.Dst.
func (g *CASGetter) Get(ctx context.Context, req *getter.Request) error {
	if req.Copy {
		// Local directory: persist to CAS and link.
		return g.CAS.StoreLocalDirectory(ctx, g.Logger, req.Src, req.Dst)
	}

	ref := ""

	u := req.URL()

	q := u.Query()
	if len(q) > 0 {
		ref = q.Get("ref")
		q.Del("ref")

		u.RawQuery = q.Encode()
	}

	// Copy so concurrent Get calls against the same getter don't race on
	// Branch/Dir mutation.
	opts := *g.Opts
	opts.Branch = ref
	opts.Dir = req.Dst

	return g.CAS.Clone(ctx, g.Logger, &opts, GitCloneURL(u.String()))
}

// GitCloneURL turns a v2-detected URL string into a clone target the
// underlying git client accepts.
//
// Two normalizations are needed:
//
//  1. Strip a leading "git::". The v2 outer client only splits the forced
//     prefix into req.Forced when the source carried it on entry; when
//     CASGetter.Detect runs its own detector chain (e.g. for github
//     shorthand or git@host:path SCP), the v2 GitDetector reattaches
//     "git::" to its result, and req.URL().String() preserves it. Passing
//     it through to git makes git look up the missing "git-remote-git"
//     helper.
//  2. Convert "ssh://git@host/path" into the SCP-style "git@host:path"
//     git expects for SSH cloning.
func GitCloneURL(urlStr string) string {
	urlStr = strings.TrimPrefix(urlStr, "git::")

	if after, ok := strings.CutPrefix(urlStr, "ssh://"); ok {
		return strings.Replace(after, "/", ":", 1)
	}

	return urlStr
}

// GetFile is not supported for the CAS getter.
func (g *CASGetter) GetFile(_ context.Context, _ *getter.Request) error {
	return cas.ErrGetFileNotSupported
}

// Mode reports directory mode for all CAS sources.
func (g *CASGetter) Mode(_ context.Context, _ *url.URL) (getter.Mode, error) {
	return getter.ModeDir, nil
}

// Detect canonicalizes the source via the detector chain. For local sources
// it sets req.Copy=true so Get takes the StoreLocalDirectory path.
func (g *CASGetter) Detect(req *getter.Request) (bool, error) {
	if req.Forced == "git" {
		return true, nil
	}

	if after, ok := strings.CutPrefix(req.Src, "git::"); ok {
		req.Src = after
		req.Forced = "git"

		return true, nil
	}

	for _, detector := range g.Detectors {
		src, ok, err := detector.Detect(req.Src, req.Pwd)
		if err != nil {
			return false, err
		}

		if !ok {
			continue
		}

		if _, isFileDetector := detector.(*getter.FileDetector); isFileDetector {
			info, statErr := g.CAS.FS().Stat(src)
			if statErr != nil {
				return false, fmt.Errorf("%w: %s", ErrDirectoryNotFound, src)
			}

			if !info.IsDir() {
				return false, fmt.Errorf("%w: %s", cas.ErrNotADirectory, src)
			}

			// Indicates a local directory to Get.
			req.Copy = true
		}

		req.Src = src

		return true, nil
	}

	return false, nil
}
