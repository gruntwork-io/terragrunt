package cas

import (
	"context"
	"errors"
	"net/url"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-getter/v2"
)

// Assert that CASGetter implements the Getter interface
var _ getter.Getter = &CASGetter{}

// CASGetter is a go-getter Getter implementation.
type CASGetter struct {
	Detectors []getter.Detector
	CAS       *CAS
	Logger    *log.Logger
	Opts      CloneOptions
}

func NewCASGetter(l *log.Logger, cas *CAS, opts CloneOptions) *CASGetter {
	return &CASGetter{
		Detectors: []getter.Detector{
			new(getter.GitHubDetector),
			new(getter.GitDetector),
			new(getter.BitBucketDetector),
			new(getter.GitLabDetector),
		},
		CAS:    cas,
		Logger: l,
		Opts:   opts,
	}
}

func (g *CASGetter) Get(ctx context.Context, req *getter.Request) error {
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

	return g.CAS.Clone(ctx, g.Logger, opts, url.String())
}

func (g *CASGetter) GetFile(_ context.Context, req *getter.Request) error {
	return errors.New("GetFile not implemented")
}

func (g *CASGetter) Mode(_ context.Context, url *url.URL) (getter.Mode, error) {
	return getter.ModeDir, nil
}

func (g *CASGetter) Detect(req *getter.Request) (bool, error) {
	for _, detector := range g.Detectors {
		if _, ok, err := detector.Detect(req.Src, ""); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}

		src, ok, err := detector.Detect(req.Src, req.Pwd)
		if err != nil {
			return ok, err
		}

		if ok {
			req.Src = src
			return ok, nil
		}
	}

	return false, nil
}
