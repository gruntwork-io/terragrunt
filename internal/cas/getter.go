package cas

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-getter/v2"
)

// Assert that CASGetter implements the Getter interface
var _ getter.Getter = &CASGetter{}

// CASGetter is a go-getter Getter implementation.
type CASGetter struct {
	CAS       *CAS
	Logger    log.Logger
	Opts      *CloneOptions
	Detectors []getter.Detector
}

func NewCASGetter(l log.Logger, cas *CAS, opts *CloneOptions) *CASGetter {
	return &CASGetter{
		Detectors: []getter.Detector{
			new(getter.GitHubDetector),
			new(getter.GitDetector),
			new(getter.BitBucketDetector),
			new(getter.GitLabDetector),
			new(getter.FileDetector),
		},
		CAS:    cas,
		Logger: l,
		Opts:   opts,
	}
}

func (g *CASGetter) Get(ctx context.Context, req *getter.Request) error {
	if req.Copy {
		// Handle local directory by persisting to CAS and linking
		return g.CAS.StoreLocalDirectory(ctx, g.Logger, req.Src, req.Dst)
	}

	ref := ""

	url := req.URL()

	q := url.Query()
	if len(q) > 0 {
		ref = q.Get("ref")
		q.Del("ref")

		url.RawQuery = q.Encode()
	}

	opts := g.Opts
	opts.Branch = ref
	opts.Dir = req.Dst

	urlStr := url.String()
	urlStr = strings.TrimPrefix(urlStr, "git::")

	// We have to switch back to the original URL scheme to clone the repository
	// go-getter sets the URL like this:
	// git::ssh://git@github.com/gruntwork-io/terragrunt.git
	// We need to switch to a valid Git URL to clone the repository
	// Like this:
	// git@github.com:gruntwork-io/terragrunt.git
	if after, ok := strings.CutPrefix(urlStr, "ssh://"); ok {
		urlStr = after
		// Replace the first slash with a colon
		urlStr = strings.Replace(urlStr, "/", ":", 1)
	}

	return g.CAS.Clone(ctx, g.Logger, opts, urlStr)
}

func (g *CASGetter) GetFile(_ context.Context, req *getter.Request) error {
	return errors.New("GetFile not implemented")
}

func (g *CASGetter) Mode(_ context.Context, url *url.URL) (getter.Mode, error) {
	return getter.ModeDir, nil
}

func (g *CASGetter) Detect(req *getter.Request) (bool, error) {
	if req.Forced == "git" {
		return true, nil
	}

	for _, detector := range g.Detectors {
		src, ok, err := detector.Detect(req.Src, req.Pwd)
		if err != nil {
			return ok, err
		}

		if ok {
			// Check if this is a FileDetector using type assertion
			if _, isFileDetector := detector.(*getter.FileDetector); isFileDetector {
				info, statErr := os.Stat(src)
				if statErr != nil {
					return false, fmt.Errorf("%w: %s", ErrDirectoryNotFound, src)
				}

				if !info.IsDir() {
					return false, fmt.Errorf("%w: %s", ErrNotADirectory, src)
				}

				// We use this as a simple way to indicate that we're working with a local directory.
				req.Copy = true
			}

			req.Src = src

			return ok, nil
		}
	}

	return false, nil
}

var (
	ErrDirectoryNotFound = errors.New("directory not found")
	ErrNotADirectory     = errors.New("not a directory")
)
