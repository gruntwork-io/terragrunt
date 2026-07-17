package getter_test

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"sync"
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

// TestCASGetterOCITagFetchIsDigestPinned pins the ABA-proof design: the
// fetch is bound to the digest resolved at download start, so a tag moving
// mid-fetch (even A to B and back to A) can never mis-key the cache.
func TestCASGetterOCITagFetchIsDigestPinned(t *testing.T) {
	t.Parallel()

	resolver := &movingTagResolver{tagDigest: "sha256:aaaa"}
	// The first download re-pushes the tag mid-fetch.
	fetcher := &countingModuleGetter{onFirstGet: func() { resolver.setTag("sha256:bbbb") }}

	g, client := newOCICASHarness(t, resolver, fetcher)
	_ = g

	tagSrc := "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0"

	dstOne := filepath.Join(t.TempDir(), "one")

	_, err := client.Get(t.Context(), &gogetter.Request{Src: tagSrc, Dst: dstOne, GetMode: gogetter.ModeDir})
	require.NoError(t, err)
	require.EqualValues(t, 1, fetcher.gets.Load())
	assert.Contains(t, fetcher.lastSrc(), "digest=sha256%3Aaaaa", "the download must be pinned to the resolved digest")
	assert.NotContains(t, fetcher.lastSrc(), "tag=", "the mutable tag must not reach the download")

	// The pinned fetch keyed A with A's content, so a digest request hits.
	dstTwo := filepath.Join(t.TempDir(), "two")

	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?digest=sha256:aaaa",
		Dst:     dstTwo,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, fetcher.gets.Load(), "digest A must hit the entry the pinned fetch stored")

	// The moved tag resolves to B now, so the tag misses and re-fetches by B.
	dstThree := filepath.Join(t.TempDir(), "three")

	_, err = client.Get(t.Context(), &gogetter.Request{Src: tagSrc, Dst: dstThree, GetMode: gogetter.ModeDir})
	require.NoError(t, err)
	assert.EqualValues(t, 2, fetcher.gets.Load(), "the moved tag must re-fetch")
	assert.Contains(t, fetcher.lastSrc(), "digest=sha256%3Abbbb", "the re-fetch must pin the moved digest")
}

// TestCASGetterOCIResolveFailureFallsBackToContentHash pins the pin-failure
// path: when resolution fails at download time, the earlier probe key must
// not be trusted.
func TestCASGetterOCIResolveFailureFallsBackToContentHash(t *testing.T) {
	t.Parallel()

	resolver := &movingTagResolver{tagDigest: "sha256:aaaa", failTagAfter: 1}
	fetcher := &countingModuleGetter{}

	_, client := newOCICASHarness(t, resolver, fetcher)

	dstOne := filepath.Join(t.TempDir(), "one")

	// Probe resolves (call 1), the pin resolution fails (call 2), so the
	// fetched bytes are stored under a content hash, not the probe key.
	_, err := client.Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dstOne,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, fetcher.gets.Load())

	// Key A must be unpopulated, so a digest pin for A has to fetch.
	dstTwo := filepath.Join(t.TempDir(), "two")

	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?digest=sha256:aaaa",
		Dst:     dstTwo,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 2, fetcher.gets.Load(), "a hit would mean the failed pin kept the stale probe key")
}

// TestCASGetterDoesNotClaimOCIWithoutFetcher pins that oci:// stays unclaimed
// when the oci fetcher is not registered.
func TestCASGetterDoesNotClaimOCIWithoutFetcher(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")

	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{})

	req := &gogetter.Request{
		Src: "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Pwd: t.TempDir(),
	}

	// The URL falls through to the file detector and fails there, which is
	// exactly what proves the oci scheme was not claimed.
	_, err = g.Detect(req)
	require.ErrorContains(t, err, "directory not found")
	assert.NotEqual(t, getter.SchemeOCI, req.Forced, "oci must not be claimed without a registered oci fetcher")
}

// newOCICASHarness builds a CASGetter over a fresh store with the fakes wired in.
func newOCICASHarness(
	t *testing.T, resolver getter.SourceResolver, fetcher gogetter.Getter,
) (*getter.CASGetter, *gogetter.Client) {
	t.Helper()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")

	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
		getter.WithGenericFetchers(map[string]gogetter.Getter{
			getter.SchemeOCI: fetcher,
		}),
		getter.WithGenericResolvers(map[string]getter.SourceResolver{
			getter.SchemeOCI: resolver,
		}),
	)

	return g, &gogetter.Client{Getters: []gogetter.Getter{g}}
}

// movingTagResolver models a mutable tag with real digest-pin semantics.
type movingTagResolver struct {
	tagDigest    string
	tagResolves  int
	failTagAfter int
	mu           sync.Mutex
}

func (r *movingTagResolver) Scheme() string { return getter.SchemeOCI }

func (r *movingTagResolver) Probe(ctx context.Context, rawURL string) (string, error) {
	digestValue, err := r.ResolveDigest(ctx, rawURL)
	if err != nil {
		return "", tgcas.ErrNoVersionMetadata
	}

	return tgcas.ContentKey("oci-manifest", digestValue), nil
}

func (r *movingTagResolver) ResolveDigest(_ context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	if pinned := u.Query().Get("digest"); pinned != "" {
		return pinned, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.tagResolves++
	if r.failTagAfter > 0 && r.tagResolves > r.failTagAfter {
		return "", errUnknownBlob
	}

	return r.tagDigest, nil
}

func (r *movingTagResolver) setTag(digestValue string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tagDigest = digestValue
}

// countingModuleGetter writes a one-file module, records the requested source,
// and runs onFirstGet during the first download to simulate a mid-fetch re-push.
type countingModuleGetter struct {
	onFirstGet func()
	src        string
	srcMu      sync.Mutex
	gets       atomic.Int32
}

func (f *countingModuleGetter) Get(_ context.Context, req *gogetter.Request) error {
	f.srcMu.Lock()
	f.src = req.Src
	f.srcMu.Unlock()

	if f.gets.Add(1) == 1 && f.onFirstGet != nil {
		f.onFirstGet()
	}

	return os.WriteFile(filepath.Join(req.Dst, "main.tf"), []byte(`output "fetched" {}`), 0o644)
}

func (f *countingModuleGetter) lastSrc() string {
	f.srcMu.Lock()
	defer f.srcMu.Unlock()

	return f.src
}

func (f *countingModuleGetter) GetFile(_ context.Context, _ *gogetter.Request) error {
	return getter.ErrOCIGetFileUnsupported
}

func (f *countingModuleGetter) Mode(_ context.Context, _ *url.URL) (gogetter.Mode, error) {
	return gogetter.ModeDir, nil
}

func (f *countingModuleGetter) Detect(_ *gogetter.Request) (bool, error) {
	return true, nil
}
