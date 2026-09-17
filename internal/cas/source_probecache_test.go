package cas_test

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const probeCacheTestURL = "https://example.com/mod.tgz"

// TestFetchSourceSharesOneProbeAcrossUnitsWithRacing pins that units
// resolving one source at the same time cost a single probe. The first
// probe is held
// until every goroutine has started so the others join its flight rather
// than racing it, and the TTL covers any goroutine the scheduler holds
// back past the flight's completion, so the count is exact rather than
// probable.
func TestFetchSourceSharesOneProbeAcrossUnitsWithRacing(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	gate := make(chan struct{})
	resolver := &fakeResolver{
		scheme: "http",
		key:    cas.OpaqueKey("http", probeCacheTestURL, "etag-abc"),
		gate:   gate,
	}

	const units = 32

	errs := make([]error, units)

	var started, wg sync.WaitGroup

	started.Add(units)

	for i := range units {
		wg.Go(func() {
			started.Done()

			errs[i] = fetchThroughNewCAS(t, storePath, v, l, resolver, []cas.Option{cas.WithProbeTTL(time.Hour)})
		})
	}

	started.Wait()
	close(gate)
	wg.Wait()

	for i := range units {
		require.NoError(t, errs[i], "unit %d", i)
	}

	assert.Equal(t, int32(1), resolver.calls.Load(), "every unit must share one probe")
}

// TestFetchSourceServesRecordedProbeWithinTTL pins that a later process
// reuses the recorded answer instead of probing again, so --cas-probe-ttl
// pays off for a source that is not a git repository.
func TestFetchSourceServesRecordedProbeWithinTTL(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	resolver := &fakeResolver{scheme: "http", key: cas.OpaqueKey("http", probeCacheTestURL, "etag-abc")}

	var fetchCalls atomic.Int32

	fetchOnce := func(c *cas.CAS) error {
		return c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: filepath.Join(t.TempDir(), "dst")},
			cas.SourceRequest{
				Scheme:   "http",
				URL:      probeCacheTestURL,
				Resolver: resolver,
				Fetch:    fakeFetcher(c, map[string]string{"main.tf": "# hello"}, &fetchCalls),
			})
	}

	first, err := cas.New(venvtest.NewWithOSFS(),
		cas.WithStorePath(storePath), cas.WithProbeCache(), cas.WithProbeTTL(time.Hour))
	require.NoError(t, err)
	require.NoError(t, fetchOnce(first))
	require.Equal(t, int32(1), resolver.calls.Load())

	second, err := cas.New(venvtest.NewWithOSFS(),
		cas.WithStorePath(storePath), cas.WithProbeCache(), cas.WithProbeTTL(time.Hour))
	require.NoError(t, err)
	require.NoError(t, fetchOnce(second))

	assert.Equal(t, int32(1), resolver.calls.Load(), "the recorded answer stood in for a second probe")
}

// TestFetchSourceReProbesWithoutTTL pins the default: a source that can
// change upstream is probed on every run, so a new object is picked up
// without the caller having to pass --cas-refresh.
func TestFetchSourceReProbesWithoutTTL(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	resolver := &fakeResolver{scheme: "http", key: cas.OpaqueKey("http", probeCacheTestURL, "etag-abc")}

	require.NoError(t, fetchThroughNewCAS(t, storePath, v, l, resolver, nil))
	require.NoError(t, fetchThroughNewCAS(t, storePath, v, l, resolver, nil))

	assert.Equal(t, int32(2), resolver.calls.Load())
}

// TestFetchSourcePinnedSourceOutlivesMutableTTL pins that a URL naming a
// version keeps its recorded answer for a day even though the caller set
// no mutable TTL, matching how a semver tag is treated on the git side.
func TestFetchSourcePinnedSourceOutlivesMutableTTL(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	resolver := &fakeResolver{
		scheme: "s3",
		key:    cas.OpaqueKey("s3", probeCacheTestURL, "checksum-abc"),
		pinned: true,
	}

	require.NoError(t, fetchThroughNewCAS(t, storePath, v, l, resolver, nil))
	require.NoError(t, fetchThroughNewCAS(t, storePath, v, l, resolver, nil))

	assert.Equal(t, int32(1), resolver.calls.Load())
}

// TestFetchSourceRefreshIgnoresRecordedProbe pins that --cas-refresh
// reaches the remote even for an answer that has not expired.
func TestFetchSourceRefreshIgnoresRecordedProbe(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	resolver := &fakeResolver{
		scheme: "s3",
		key:    cas.OpaqueKey("s3", probeCacheTestURL, "checksum-abc"),
		pinned: true,
	}

	require.NoError(t, fetchThroughNewCAS(t, storePath, v, l, resolver, nil))
	require.NoError(t, fetchThroughNewCAS(t, storePath, v, l, resolver, []cas.Option{cas.WithProbeRefresh()}))

	assert.Equal(t, int32(2), resolver.calls.Load())
}

// TestFetchSourceOfflineServesRecordedProbe pins that --cas-offline
// resolves a non-git source from what an earlier run recorded, however old
// that answer is, without reaching the remote.
func TestFetchSourceOfflineServesRecordedProbe(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	resolver := &fakeResolver{scheme: "http", key: cas.OpaqueKey("http", probeCacheTestURL, "etag-abc")}

	require.NoError(t, fetchThroughNewCAS(t, storePath, v, l, resolver, nil))
	require.NoError(t, fetchThroughNewCAS(t, storePath, v, l, resolver, []cas.Option{cas.WithOffline()}))

	assert.Equal(t, int32(1), resolver.calls.Load(), "offline never reached the resolver")
}

// TestFetchSourceOfflineMissFails pins that an offline run refuses a
// source nothing recorded, naming it, rather than quietly probing it.
func TestFetchSourceOfflineMissFails(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	resolver := &fakeResolver{scheme: "http", key: cas.OpaqueKey("http", probeCacheTestURL, "etag-abc")}

	err := fetchThroughNewCAS(t, storePath, v, l, resolver, []cas.Option{cas.WithOffline()})

	var miss *cas.OfflineMissError

	require.ErrorAs(t, err, &miss)
	require.ErrorIs(t, err, cas.ErrCASOffline)
	assert.Equal(t, probeCacheTestURL, miss.Source)
	assert.Equal(t, int32(0), resolver.calls.Load())
}

// TestFetchSourceRecordsNothingWithoutProbeCache pins the gate that keeps
// recording behind the offline-cas experiment: with no caller asking for
// the cache, a second process re-probes and the store holds nothing it
// could have read.
func TestFetchSourceRecordsNothingWithoutProbeCache(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	resolver := &fakeResolver{
		scheme: "s3",
		key:    cas.OpaqueKey("s3", probeCacheTestURL, "checksum-abc"),
		pinned: true,
	}

	fetch := func() error {
		c, err := cas.New(venvtest.NewWithOSFS(),
			cas.WithStorePath(storePath), cas.WithProbeTTL(time.Hour))
		require.NoError(t, err)

		var fetchCalls atomic.Int32

		return c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: filepath.Join(t.TempDir(), "dst")},
			cas.SourceRequest{
				Scheme:   resolver.scheme,
				URL:      probeCacheTestURL,
				Resolver: resolver,
				Fetch:    fakeFetcher(c, map[string]string{"main.tf": "# hello"}, &fetchCalls),
			})
	}

	require.NoError(t, fetch())
	require.NoError(t, fetch())

	assert.Equal(t, int32(2), resolver.calls.Load(), "neither a TTL nor a pin caches an answer")
	assert.NoDirExists(t, filepath.Join(storePath, "probes"))
}

// fetchThroughNewCAS runs one FetchSource against a CAS built over
// storePath, so each call stands for a separate Terragrunt process sharing
// one store. The persisted probe cache is on, as it is for a run with the
// offline-cas experiment enabled.
func fetchThroughNewCAS(
	t *testing.T,
	storePath string,
	v *venv.Venv,
	l log.Logger,
	resolver *fakeResolver,
	opts []cas.Option,
) error {
	t.Helper()

	base := make([]cas.Option, 0, len(opts)+2)
	base = append(base, cas.WithStorePath(storePath), cas.WithProbeCache())

	c, err := cas.New(venvtest.NewWithOSFS(), append(base, opts...)...)
	require.NoError(t, err)

	var fetchCalls atomic.Int32

	return c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: filepath.Join(t.TempDir(), "dst")},
		cas.SourceRequest{
			Scheme:   resolver.scheme,
			URL:      probeCacheTestURL,
			Resolver: resolver,
			Fetch:    fakeFetcher(c, map[string]string{"main.tf": "# hello"}, &fetchCalls),
		})
}

// TestFetchSourceOfflineRefusesRepair pins that a store missing an object
// behind a recorded probe fails an offline run by naming the object, not
// by re-ingesting from the remote the flag forbids and not as a miss for
// content that was in fact fetched.
func TestFetchSourceOfflineRefusesRepair(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	key := cas.OpaqueKey("http", probeCacheTestURL, "etag-abc")
	resolver := &fakeResolver{scheme: "http", key: key}

	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath), cas.WithProbeCache())
	require.NoError(t, err)

	var fetchCalls atomic.Int32

	files := map[string]string{"main.tf": "# hello"}
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: filepath.Join(t.TempDir(), "dst1")},
		cas.SourceRequest{
			Scheme:   "http",
			URL:      probeCacheTestURL,
			Resolver: resolver,
			Fetch:    fakeFetcher(c, files, &fetchCalls),
		}))
	require.NoError(t, v.FS.Remove(storedBlobPath(t, c, v, key, "main.tf")))

	offline, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath), cas.WithOffline())
	require.NoError(t, err)

	err = offline.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: filepath.Join(t.TempDir(), "dst2")},
		cas.SourceRequest{
			Scheme:   "http",
			URL:      probeCacheTestURL,
			Resolver: resolver,
			Fetch:    fakeFetcher(offline, files, &fetchCalls),
		})

	var refused *cas.OfflineRepairError

	require.ErrorAs(t, err, &refused)
	require.NotErrorIs(t, err, cas.ErrCASOffline)
	assert.Equal(t, probeCacheTestURL, refused.Source)

	var missing *cas.MissingObjectError

	require.ErrorAs(t, err, &missing)
	assert.Equal(t, int32(1), fetchCalls.Load(), "the offline run must not re-ingest the source")
}
