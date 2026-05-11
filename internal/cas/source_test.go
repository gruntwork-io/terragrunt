package cas_test

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestFetchSource_ProbeHitSkipsDownload(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	resolver := &fakeResolver{
		scheme: "http",
		key:    cas.OpaqueKey("http", "https://example.com/mod.tgz", "etag-abc"),
	}

	var fetchCalls atomic.Int32

	fetch := fakeFetcher(c, map[string]string{
		"main.tf":  `# hello`,
		"README":   "readme",
		"sub/x.tf": `variable "x" {}`,
	}, &fetchCalls)

	dst1 := filepath.Join(t.TempDir(), "dst1")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dst1}, cas.SourceRequest{
		Scheme:   "http",
		URL:      "https://example.com/mod.tgz",
		Resolver: resolver,
		Fetch:    fetch,
	}))

	require.Equal(t, int32(1), fetchCalls.Load())
	require.FileExists(t, filepath.Join(dst1, "main.tf"))

	dst2 := filepath.Join(t.TempDir(), "dst2")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dst2}, cas.SourceRequest{
		Scheme:   "http",
		URL:      "https://example.com/mod.tgz",
		Resolver: resolver,
		Fetch:    fetch,
	}))

	assert.Equal(t, int32(1), fetchCalls.Load(), "second call must hit the cache and not re-fetch")
	assert.FileExists(t, filepath.Join(dst2, "main.tf"))
	assert.FileExists(t, filepath.Join(dst2, "sub", "x.tf"))
}

func TestFetchSource_NoMetadataFallsBackToContentHash(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	resolver := &fakeResolver{
		scheme: "http",
		err:    cas.ErrNoVersionMetadata,
	}

	var fetchCalls atomic.Int32

	fetch := fakeFetcher(c, map[string]string{
		"main.tf": "content",
	}, &fetchCalls)

	dst1 := filepath.Join(t.TempDir(), "dst1")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dst1}, cas.SourceRequest{
		Scheme:   "http",
		URL:      "https://example.com/mod.tgz",
		Resolver: resolver,
		Fetch:    fetch,
	}))

	dst2 := filepath.Join(t.TempDir(), "dst2")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dst2}, cas.SourceRequest{
		Scheme:   "http",
		URL:      "https://example.com/mod.tgz",
		Resolver: resolver,
		Fetch:    fetch,
	}))

	assert.Equal(t, int32(2), fetchCalls.Load(), "no probe means we re-download every time")
	assert.Equal(t, int32(2), resolver.calls.Load())
	assert.FileExists(t, filepath.Join(dst2, "main.tf"))
}

func TestFetchSource_NilResolverContentHashes(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	var fetchCalls atomic.Int32

	fetch := fakeFetcher(c, map[string]string{
		"a.tf": "1",
	}, &fetchCalls)

	dst := filepath.Join(t.TempDir(), "dst")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dst}, cas.SourceRequest{
		Scheme: "s3",
		URL:    "s3://bucket/key.tgz",
		Fetch:  fetch,
	}))

	assert.Equal(t, int32(1), fetchCalls.Load())
	assert.FileExists(t, filepath.Join(dst, "a.tf"))
}

func TestFetchSource_ContentAddressedDedupesAcrossURLs(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	// Both resolvers report the same content-addressed cache key. The
	// URL drops out of the derivation so identical bytes at two URLs
	// share a tree-store entry.
	contentKey := cas.ContentKey("sha256", "deadbeef")

	files := map[string]string{
		"main.tf": "ok",
	}

	var fetchCalls atomic.Int32

	resolverA := &fakeResolver{scheme: "s3", key: contentKey}
	resolverB := &fakeResolver{scheme: "gcs", key: contentKey}

	dstA := filepath.Join(t.TempDir(), "dstA")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dstA}, cas.SourceRequest{
		Scheme:   "s3",
		URL:      "s3://bucketA/mod.tgz",
		Resolver: resolverA,
		Fetch:    fakeFetcher(c, files, &fetchCalls),
	}))

	dstB := filepath.Join(t.TempDir(), "dstB")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dstB}, cas.SourceRequest{
		Scheme:   "gcs",
		URL:      "gs://bucketB/different/path.tgz",
		Resolver: resolverB,
		Fetch:    fakeFetcher(c, files, &fetchCalls),
	}))

	assert.Equal(t, int32(1), fetchCalls.Load(),
		"content-addressed probe must dedupe identical bytes across two URLs")
	assert.FileExists(t, filepath.Join(dstB, "main.tf"))
}

func TestFetchSource_OpaqueProbeURLScoped(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	files := map[string]string{"main.tf": "hello"}

	var fetchCalls atomic.Int32

	resolverA := &fakeResolver{
		scheme: "http",
		key:    cas.OpaqueKey("http", "https://a.example.com/mod.tgz", "same-etag"),
	}
	resolverB := &fakeResolver{
		scheme: "http",
		key:    cas.OpaqueKey("http", "https://b.example.com/mod.tgz", "same-etag"),
	}

	dstA := filepath.Join(t.TempDir(), "dstA")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dstA}, cas.SourceRequest{
		Scheme:   "http",
		URL:      "https://a.example.com/mod.tgz",
		Resolver: resolverA,
		Fetch:    fakeFetcher(c, files, &fetchCalls),
	}))

	dstB := filepath.Join(t.TempDir(), "dstB")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dstB}, cas.SourceRequest{
		Scheme:   "http",
		URL:      "https://b.example.com/mod.tgz",
		Resolver: resolverB,
		Fetch:    fakeFetcher(c, files, &fetchCalls),
	}))

	assert.Equal(t, int32(2), fetchCalls.Load(),
		"opaque probe must not dedupe across distinct URLs even when the token matches")
}

func TestFetchSource_ConcurrentFetchesOfSameKeyWithRacing(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	resolver := &fakeResolver{
		scheme: "http",
		key:    cas.OpaqueKey("http", "https://example.com/race.tgz", "etag-race"),
	}

	var fetchCalls atomic.Int32

	files := map[string]string{"main.tf": "race"}

	const n = 8

	// Pre-allocate destination directories from the test goroutine.
	// t.TempDir() invoked from worker goroutines races with t.Cleanup
	// registration when many workers fire at once on macOS.
	dsts := make([]string, n)
	for i := range n {
		dsts[i] = filepath.Join(t.TempDir(), "dst")
	}

	var g errgroup.Group

	for i := range n {
		dst := dsts[i]

		g.Go(func() error {
			return c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dst}, cas.SourceRequest{
				Scheme:   "http",
				URL:      "https://example.com/race.tgz",
				Resolver: resolver,
				Fetch:    fakeFetcher(c, files, &fetchCalls),
			})
		})
	}

	require.NoError(t, g.Wait())

	// At least one fetch occurred; concurrent racers may legitimately
	// fetch into separate temp dirs and only the first to acquire the
	// tree-store lock wins.
	assert.GreaterOrEqual(t, fetchCalls.Load(), int32(1))
}

func TestFetchSource_RequiresFetchClosure(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	err := c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: t.TempDir()}, cas.SourceRequest{
		Scheme: "http",
		URL:    "https://example.com",
	})
	require.ErrorIs(t, err, cas.ErrFetchClosureRequired)
}

func TestContentKey_URLIndependent(t *testing.T) {
	t.Parallel()

	k1 := cas.ContentKey("sha256", "abc")
	k2 := cas.ContentKey("sha256", "abc")
	assert.Equal(t, k1, k2, "same alg+token must produce the same key")

	// Algorithm tag matters: different alg, same token, different key.
	assert.NotEqual(t, k1, cas.ContentKey("md5", "abc"))
	// Token matters.
	assert.NotEqual(t, k1, cas.ContentKey("sha256", "abd")) //codespell:ignore abd
}

func TestOpaqueKey_URLScoped(t *testing.T) {
	t.Parallel()

	k1 := cas.OpaqueKey("http", "https://a.example/x", "etag")
	k2 := cas.OpaqueKey("http", "https://b.example/x", "etag")
	assert.NotEqual(t, k1, k2, "different URLs must produce different keys")

	// Scheme matters.
	assert.NotEqual(t, k1, cas.OpaqueKey("https", "https://a.example/x", "etag"))
	// Token matters.
	assert.NotEqual(t, k1, cas.OpaqueKey("http", "https://a.example/x", "other"))
}

func TestContentKey_DoesNotCollideWithOpaqueKey(t *testing.T) {
	t.Parallel()

	// Both derivations would otherwise hash the same token "abc" but
	// the namespace prefix ("content" vs "source") keeps them apart.
	content := cas.ContentKey("sha256", "abc")
	opaque := cas.OpaqueKey("sha256", "abc", "abc")
	assert.NotEqual(t, content, opaque)
}

// fakeResolver returns a canned cache key on Probe.
type fakeResolver struct {
	err    error
	scheme string
	key    string
	calls  atomic.Int32
}

func (f *fakeResolver) Scheme() string { return f.scheme }

func (f *fakeResolver) Probe(_ context.Context, _ string) (string, error) {
	f.calls.Add(1)

	if f.err != nil {
		return "", f.err
	}

	return f.key, nil
}

// fakeFetcher writes a deterministic fixture into a fresh temp dir,
// ingests it into CAS via IngestDirectory, and returns the resulting
// tree key. It counts invocations for assertions.
func fakeFetcher(c *cas.CAS, files map[string]string, calls *atomic.Int32) cas.SourceFetcher {
	return func(_ context.Context, l log.Logger, v cas.Venv, suggestedKey string) (string, error) {
		calls.Add(1)

		tempDir, cleanup, err := c.MakeFetchTempDir(l, v)
		if err != nil {
			return "", err
		}

		defer cleanup()

		for rel, body := range files {
			full := filepath.Join(tempDir, rel)
			if err := vfs.WriteFile(v.FS, full, []byte(body), 0o644); err != nil {
				return "", err
			}
		}

		return c.IngestDirectory(l, v, tempDir, suggestedKey)
	}
}
