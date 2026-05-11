package getter_test

import (
	"context"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"

	tgcas "github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCASGetter_ForcedThreadedToInnerClient pins the wiring fix that
// would otherwise let the inner getter.Client's bare scheme-specific
// getter (Detect rejects URLs whose scheme doesn't match its
// validScheme) silently refuse to claim the request, surfacing as a
// generic "error downloading" multierror wrap.
//
// Both the bare go-getter v2 s3.Getter and gcs.Getter implement
// Detect as: forced != "" → validScheme(forced) → claim; URL.Scheme
// → validScheme(scheme) → claim; otherwise reject. For an `http://`
// or `gs://` URL with no forced field set, Detect rejects, no getter
// claims, and the client wraps with "error downloading". The fix is
// to propagate the scheme as `Forced` on the inner request.
func TestCASGetter_ForcedThreadedToInnerClient(t *testing.T) {
	t.Parallel()

	const scheme = "fakescheme"

	stub := &forcedRequiredGetter{scheme: scheme}

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
		getter.WithGenericFetchers(map[string]gogetter.Getter{scheme: stub}),
		// No resolver registered → forces the fetch path.
	)

	client := &gogetter.Client{Getters: []gogetter.Getter{g}}

	// The outer client.Get error is incidental to this test: the
	// stub's no-op Get returns nil but CAS.FetchSource then walks
	// the empty temp dir and ingests an empty tree, which may or
	// may not fail depending on the storeFetchedContent path. Log
	// whatever comes back so the assertion below is the contract.
	if _, err := client.Get(t.Context(), &gogetter.Request{
		Src:     scheme + "::http://example.com/anything.tar.gz",
		Dst:     filepath.Join(t.TempDir(), "out"),
		GetMode: gogetter.ModeAny,
	}); err != nil {
		t.Logf("client.Get returned %v (incidental for this test)", err)
	}

	// The stub records each Detect call; if Forced wasn't
	// propagated to the inner request, the stub's Detect returns
	// false because the URL scheme is `http` (not `fakescheme`) and
	// the inner client falls through to "error downloading". The
	// assertion below pins that Forced reached the stub.
	assert.Positive(t, stub.detectCalls.Load(),
		"inner getter.Client should have invoked stub.Detect")
	assert.Positive(t, stub.forcedCalls.Load(),
		"stub.Detect should have seen req.Forced == %q at least once "+
			"(if not, CASGetter is failing to thread the scheme through)", scheme)
}

// TestCASGetter_GetCanonicalizesForcedAlias pins that CASGetter.Get
// resolves an alias forced scheme (gs) to its registry key (gcs)
// before looking up the fetcher. Without canonicalization, a caller
// that reaches Get with req.Forced == "gs" passes the
// isGenericScheme gate (which uses lookupFetcher) and then trips a
// "no fetcher registered" failure inside getGeneric on the raw map
// lookup.
//
// Reaching the stub at all proves the alias resolved: the stub is
// only registered under "gcs", so an unresolved "gs" lookup would
// return the fetcher-not-registered error before any Detect call.
func TestCASGetter_GetCanonicalizesForcedAlias(t *testing.T) {
	t.Parallel()

	stub := &forcedRequiredGetter{scheme: getter.SchemeGCS}

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
		getter.WithGenericFetchers(map[string]gogetter.Getter{getter.SchemeGCS: stub}),
	)

	// Bypass the outer client so Detect does not normalize Forced;
	// Get must do the canonicalization itself.
	if err := g.Get(t.Context(), &gogetter.Request{
		Src:     "https://www.googleapis.com/storage/v1/bucket/mod.tgz",
		Dst:     filepath.Join(t.TempDir(), "out"),
		Forced:  "gs",
		GetMode: gogetter.ModeAny,
	}); err != nil {
		t.Logf("Get returned %v (incidental for this test)", err)
	}

	assert.Positive(t, stub.forcedCalls.Load(),
		"gcs fetcher stub should have been reached through the gs alias")
}

// forcedRequiredGetter is a bare-getter stub that only claims a
// request when req.Forced matches its scheme. Mirrors the bare s3 and
// gcs getters' Detect behavior.
type forcedRequiredGetter struct {
	scheme      string
	detectCalls atomic.Int32
	forcedCalls atomic.Int32
}

func (g *forcedRequiredGetter) Detect(req *gogetter.Request) (bool, error) {
	g.detectCalls.Add(1)

	if req.Forced == g.scheme {
		g.forcedCalls.Add(1)
		return true, nil
	}

	return false, nil
}

func (g *forcedRequiredGetter) Get(_ context.Context, _ *gogetter.Request) error {
	return nil
}

func (g *forcedRequiredGetter) GetFile(_ context.Context, _ *gogetter.Request) error {
	return nil
}

func (g *forcedRequiredGetter) Mode(_ context.Context, _ *url.URL) (gogetter.Mode, error) {
	return gogetter.ModeDir, nil
}
