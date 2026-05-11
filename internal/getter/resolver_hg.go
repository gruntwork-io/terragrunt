package getter

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
)

// hgResolverTimeout caps `hg identify` so a slow remote can't stall CAS.
const hgResolverTimeout = 10 * time.Second

// HgResolver is a [cas.SourceResolver] for Mercurial sources.
type HgResolver struct {
	// Exec runs the hg binary. Required; [NewHgResolver] wires
	// [vexec.NewOSExec]. Tests substitute an in-memory backend.
	Exec vexec.Exec
	// HgBinary overrides the binary name resolved via [vexec.Exec.LookPath].
	// Empty means "hg".
	HgBinary string
}

// NewHgResolver returns a resolver bound to the real OS-backed exec
// and the ambient `hg` binary on PATH.
func NewHgResolver() *HgResolver { return &HgResolver{Exec: vexec.NewOSExec()} }

// Scheme returns "hg".
func (r *HgResolver) Scheme() string { return "hg" }

// Probe runs `hg identify --template '{node}\n'` against rawURL and
// returns the 40-char node hash as a content-addressed cache key. The
// ref comes from the URL's `rev` query parameter; absent or empty
// means "tip". Missing binary, timeout, or unreachable remote produce
// [cas.ErrNoVersionMetadata].
//
// `--template '{node}'` is used instead of `--id` because `--id`
// returns the abbreviated 12-char short hash, which is not
// collision-safe for cache keying.
func (r *HgResolver) Probe(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse hg URL %s: %w", rawURL, err)
	}

	rev := u.Query().Get("rev")

	cleaned := *u
	q := cleaned.Query()

	q.Del("rev")
	cleaned.RawQuery = q.Encode()

	bin := r.HgBinary
	if bin == "" {
		bin = "hg"
	}

	if _, err := r.Exec.LookPath(bin); err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	ctx, cancel := context.WithTimeout(ctx, hgResolverTimeout)
	defer cancel()

	// --rev=<v> and the -- terminator keep a `-`-prefixed value
	// from being reparsed by hg's option parser.
	args := []string{"identify", "--template", "{node}\n"}
	if rev != "" {
		args = append(args, "--rev="+rev)
	}

	args = append(args, "--", cleaned.String())

	out, err := r.Exec.Command(ctx, bin, args...).Output()
	if err != nil {
		return "", cas.ErrNoVersionMetadata
	}

	node := strings.TrimSpace(string(out))
	if node == "" {
		return "", cas.ErrNoVersionMetadata
	}

	return cas.ContentKey("hg-node", node), nil
}
