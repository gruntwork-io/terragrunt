package getter

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	getter "github.com/hashicorp/go-getter/v2"
)

// CASProtocolGetter resolves cas::<algorithm>:<hash> references by
// materializing the referenced tree from the CAS store.
type CASProtocolGetter struct {
	CAS    *cas.CAS
	Logger log.Logger
}

// NewCASProtocolGetter creates a new CASProtocolGetter.
func NewCASProtocolGetter(l log.Logger, c *cas.CAS) *CASProtocolGetter {
	return &CASProtocolGetter{
		CAS:    c,
		Logger: l,
	}
}

// Get materializes the tree referenced by req.Src into req.Dst.
func (g *CASProtocolGetter) Get(ctx context.Context, req *getter.Request) error {
	src := strings.TrimPrefix(req.Src, cas.CASProtocolPrefix)

	hash, err := cas.ParseCASRef(src)
	if err != nil {
		return fmt.Errorf("failed to parse CAS reference %q: %w", req.Src, err)
	}

	return g.CAS.MaterializeTree(ctx, g.Logger, hash, req.Dst)
}

// GetFile is not supported for the CAS protocol getter.
func (g *CASProtocolGetter) GetFile(_ context.Context, _ *getter.Request) error {
	return cas.ErrGetFileNotSupported
}

// Mode reports directory mode for all CAS protocol sources.
func (g *CASProtocolGetter) Mode(_ context.Context, _ *url.URL) (getter.Mode, error) {
	return getter.ModeDir, nil
}

// Detect recognizes cas:: prefixed sources and forces the cas protocol.
func (g *CASProtocolGetter) Detect(req *getter.Request) (bool, error) {
	src := req.Src

	if req.Forced == "cas" {
		if _, err := cas.ParseCASRef(src); err == nil {
			return true, nil
		}

		return false, nil
	}

	if strings.HasPrefix(src, cas.CASProtocolPrefix) {
		// Direct calls to Detect (tests) or sources that did not go through the wrapper.
		req.Forced = "cas"

		return true, nil
	}

	return false, nil
}
