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

// TestCASGetterOCITagFetchIsDigestPinned pins the ABA-proof design with
// digest-specific content: the fetch is bound to the digest resolved at
// download start, so a tag moving A to B and back to A can neither mis-key
// the cache nor materialize the wrong manifest's bytes.
func TestCASGetterOCITagFetchIsDigestPinned(t *testing.T) {
	t.Parallel()

	resolver := &movingTagResolver{tagDigest: "sha256:aaaa"}
	fetcher := &digestModuleGetter{}
	// First download: the tag moves A to B mid-fetch. Second download: it
	// moves back to A, completing the A to B to A sequence.
	fetcher.onGet = func(call int32) {
		if call == 1 {
			resolver.setTag("sha256:bbbb")
		}

		if call == 2 {
			resolver.setTag("sha256:aaaa")
		}
	}

	_, client := newOCICASHarness(t, resolver, fetcher)

	tagSrc := "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0"

	// Fetch 1 pins digest A before the mid-fetch move, so A's content lands
	// under A's key.
	dstOne := filepath.Join(t.TempDir(), "one")

	_, err := client.Get(t.Context(), &gogetter.Request{Src: tagSrc, Dst: dstOne, GetMode: gogetter.ModeDir})
	require.NoError(t, err)
	require.EqualValues(t, 1, fetcher.gets.Load())
	assert.Contains(t, fetcher.lastSrc(), "digest=sha256%3Aaaaa", "the download must be pinned to the resolved digest")
	assert.NotContains(t, fetcher.lastSrc(), "tag=", "the mutable tag must not reach the download")
	assertModuleContent(t, dstOne, "sha256:aaaa")

	// The tag points at B now: fetch 2 pins digest B and moves the tag back
	// to A mid-fetch, so B's content lands under B's key.
	dstTwo := filepath.Join(t.TempDir(), "two")

	_, err = client.Get(t.Context(), &gogetter.Request{Src: tagSrc, Dst: dstTwo, GetMode: gogetter.ModeDir})
	require.NoError(t, err)
	require.EqualValues(t, 2, fetcher.gets.Load(), "the moved tag must re-fetch")
	assert.Contains(t, fetcher.lastSrc(), "digest=sha256%3Abbbb", "the re-fetch must pin the moved digest")
	assertModuleContent(t, dstTwo, "sha256:bbbb")

	// After A to B to A, digest requests hit their own entries with their
	// own bytes and no further fetches.
	dstA := filepath.Join(t.TempDir(), "digest-a")

	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?digest=sha256:aaaa",
		Dst:     dstA,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	assertModuleContent(t, dstA, "sha256:aaaa")

	dstB := filepath.Join(t.TempDir(), "digest-b")

	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?digest=sha256:bbbb",
		Dst:     dstB,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	assertModuleContent(t, dstB, "sha256:bbbb")

	// The tag is back on A: a tag request hits A's cached entry.
	dstThree := filepath.Join(t.TempDir(), "three")

	_, err = client.Get(t.Context(), &gogetter.Request{Src: tagSrc, Dst: dstThree, GetMode: gogetter.ModeDir})
	require.NoError(t, err)
	assert.EqualValues(t, 2, fetcher.gets.Load(), "every request after the two fetches must be a cache hit")
	assertModuleContent(t, dstThree, "sha256:aaaa")
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

// TestCASGetterOCISubdirSelectionSharesOneEntry proves root and subdir
// requests share one full-root cache entry in either order: the client strips
// //subdir before dispatch, so even a subdir-first miss ingests the complete
// root, and the client applies the selector after materialization.
func TestCASGetterOCISubdirSelectionSharesOneEntry(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		subdirFirst bool
	}{
		{name: "root then subdir", subdirFirst: false},
		{name: "subdir then root", subdirFirst: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resolver := &movingTagResolver{tagDigest: "sha256:aaaa"}
			fetcher := &treeModuleGetter{}

			_, client := newOCICASHarness(t, resolver, fetcher)

			rootSrc := "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0"
			subSrc := "oci://127.0.0.1:5000/terraform-modules/vpc//subdir?tag=1.0.0"

			first, second := rootSrc, subSrc
			if tc.subdirFirst {
				first, second = subSrc, rootSrc
			}

			dstFirst := filepath.Join(t.TempDir(), "first")

			_, err := client.Get(t.Context(), &gogetter.Request{Src: first, Dst: dstFirst, GetMode: gogetter.ModeDir})
			require.NoError(t, err)

			dstSecond := filepath.Join(t.TempDir(), "second")

			_, err = client.Get(t.Context(), &gogetter.Request{Src: second, Dst: dstSecond, GetMode: gogetter.ModeDir})
			require.NoError(t, err)
			require.EqualValues(t, 1, fetcher.gets.Load(), "both orders must share one full-root cache entry")

			dstRoot, dstSub := dstFirst, dstSecond
			if tc.subdirFirst {
				dstRoot, dstSub = dstSecond, dstFirst
			}

			assert.FileExists(t, filepath.Join(dstRoot, "main.tf"))
			assert.FileExists(t, filepath.Join(dstRoot, "subdir", "sub.tf"))
			assert.FileExists(t, filepath.Join(dstSub, "sub.tf"))
			assert.NoFileExists(t, filepath.Join(dstSub, "main.tf"), "the root tree must not leak into a subdir request")
			assert.NoFileExists(t, filepath.Join(dstSub, "subdir"), "the selector must be applied, not the full tree")
		})
	}
}

// TestCASGetterOCIProbeOnlyResolverNeverKeysMutableTags pins the fallback for
// custom resolvers without the digest contract: the fetched bytes must be
// content-hashed, never stored under the mutable probe key.
func TestCASGetterOCIProbeOnlyResolverNeverKeysMutableTags(t *testing.T) {
	t.Parallel()

	keyA := tgcas.ContentKey("oci-manifest", "sha256:aaaa")

	resolver := &probeOnlyResolver{key: keyA}
	fetcher := &countingModuleGetter{}

	_, client := newOCICASHarness(t, resolver, fetcher)

	dstOne := filepath.Join(t.TempDir(), "one")

	_, err := client.Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dstOne,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, fetcher.gets.Load())
	assert.Contains(t, fetcher.lastSrc(), "tag=1.0.0", "a probe-only resolver cannot pin the fetch")

	// Key A must be unpopulated, so a request probing the same key re-fetches.
	dstTwo := filepath.Join(t.TempDir(), "two")

	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dstTwo,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 2, fetcher.gets.Load(), "a hit would mean mutable-tag bytes were stored under the probe key")
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

// probeOnlyResolver implements only the base resolver contract, no digest pinning.
type probeOnlyResolver struct {
	key string
}

func (r *probeOnlyResolver) Scheme() string { return getter.SchemeOCI }

func (r *probeOnlyResolver) Probe(context.Context, string) (string, error) {
	return r.key, nil
}

// treeModuleGetter writes a two-level module tree and counts downloads.
type treeModuleGetter struct {
	gets atomic.Int32
}

func (f *treeModuleGetter) Get(_ context.Context, req *gogetter.Request) error {
	f.gets.Add(1)

	if err := os.MkdirAll(filepath.Join(req.Dst, "subdir"), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(req.Dst, "main.tf"), []byte(`output "root" {}`), 0o644); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(req.Dst, "subdir", "sub.tf"), []byte(`output "sub" {}`), 0o644)
}

func (f *treeModuleGetter) GetFile(_ context.Context, _ *gogetter.Request) error {
	return getter.ErrOCIGetFileUnsupported
}

func (f *treeModuleGetter) Mode(_ context.Context, _ *url.URL) (gogetter.Mode, error) {
	return gogetter.ModeDir, nil
}

func (f *treeModuleGetter) Detect(_ *gogetter.Request) (bool, error) {
	return true, nil
}

// assertModuleContent asserts the materialized module carries the content of
// the given digest.
func assertModuleContent(t *testing.T, dst, digestValue string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(dst, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(data), digestValue, "the materialized tree must match the pinned digest")
}

// digestModuleGetter writes digest-specific content, records the requested
// source, and runs onGet with the download count for mid-fetch tag moves.
type digestModuleGetter struct {
	onGet func(call int32)
	src   string
	srcMu sync.Mutex
	gets  atomic.Int32
}

func (f *digestModuleGetter) Get(_ context.Context, req *gogetter.Request) error {
	f.srcMu.Lock()
	f.src = req.Src
	f.srcMu.Unlock()

	call := f.gets.Add(1)
	if f.onGet != nil {
		f.onGet(call)
	}

	pinned := "UNPINNED"
	if u, err := url.Parse(req.Src); err == nil && u.Query().Get("digest") != "" {
		pinned = u.Query().Get("digest")
	}

	content := `output "manifest" { value = "` + pinned + `" }`

	return os.WriteFile(filepath.Join(req.Dst, "main.tf"), []byte(content), 0o644)
}

func (f *digestModuleGetter) lastSrc() string {
	f.srcMu.Lock()
	defer f.srcMu.Unlock()

	return f.src
}

func (f *digestModuleGetter) GetFile(_ context.Context, _ *gogetter.Request) error {
	return getter.ErrOCIGetFileUnsupported
}

func (f *digestModuleGetter) Mode(_ context.Context, _ *url.URL) (gogetter.Mode, error) {
	return gogetter.ModeDir, nil
}

func (f *digestModuleGetter) Detect(_ *gogetter.Request) (bool, error) {
	return true, nil
}
