package cas_test

import (
	"context"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// newRecordedVenv returns an OS-backed bundle whose git invocations are
// counted by the returned recorder. A nil gate leaves ls-remote ungated.
func newRecordedVenv(gate chan struct{}) (*venv.Venv, *gitExecRecorder) {
	rec := newGitExecRecorder(vexec.NewOSExec(), gate)

	return venvtest.NewOSWithEmptyEnv().WithExec(rec), rec
}

// newStoreCAS constructs a CAS over storePath with opts, the way each unit
// of a `run --all` constructs its own. The persisted probe cache is on, as
// it is for a run with the offline-cas experiment enabled. It returns the
// construction error rather than asserting so goroutines can call it too.
func newStoreCAS(storePath string, opts ...cas.Option) (*cas.CAS, error) {
	base := make([]cas.Option, 0, len(opts)+2)
	base = append(base, cas.WithStorePath(storePath), cas.WithProbeCache())

	return cas.New(venvtest.NewWithOSFS(), append(base, opts...)...)
}

// cloneInto runs one Clone of branch into dst through a CAS built over
// storePath, the unit of work every goroutine in these tests performs.
func cloneInto(
	ctx context.Context,
	v *venv.Venv,
	storePath, repoURL, branch, dst string,
	opts ...cas.Option,
) error {
	c, err := newStoreCAS(storePath, opts...)
	if err != nil {
		return err
	}

	return c.Clone(ctx, logger.CreateLogger(), v, repoURL,
		cas.WithDir(dst),
		cas.WithBranch(branch),
		cas.WithDepth(-1))
}

// TestCASClone_ConcurrentInstancesShareProbeAndIngestWithRacing pins the
// `run --all` shape end to end: 32 CAS instances over one store cloning
// the same branch at once cost one ls-remote and one ingest between them.
// The first ls-remote is held until every goroutine has been started so
// the others join its flight rather than racing it, and the mutable-ref
// TTL covers any goroutine the scheduler still manages to hold back past
// the flight's completion, so the count is exact rather than probable.
func TestCASClone_ConcurrentInstancesShareProbeAndIngestWithRacing(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)
	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	gate := make(chan struct{})
	v, rec := newRecordedVenv(gate)

	const callers = 32

	errs := make([]error, callers)

	var started, wg sync.WaitGroup

	started.Add(callers)

	for i := range callers {
		wg.Go(func() {
			started.Done()

			errs[i] = cloneInto(t.Context(), v, storePath, repoURL, "main",
				filepath.Join(tempDir, "dst-"+strconv.Itoa(i)),
				cas.WithProbeTTL(time.Hour))
		})
	}

	started.Wait()
	close(gate)
	wg.Wait()

	for i := range callers {
		require.NoError(t, errs[i], "caller %d", i)
		assert.FileExists(t, filepath.Join(tempDir, "dst-"+strconv.Itoa(i), "main.tf"), "caller %d", i)
	}

	assert.Equal(t, 1, rec.count("ls-remote"), "every caller must share one probe")
	assert.Equal(t, 1, rec.count("fetch"), "every caller must share one fetch into the git store")
	assert.Equal(t, 1, rec.count("ls-tree"), "every caller must share one tree ingest")
}

// TestCASClone_LeaderCancellationFollowersStillSucceedWithRacing pins that
// the caller whose context started a probe flight can go away without
// taking the flight with it: the followers still get their clone. The
// first ls-remote is held open so the leader can be cancelled mid-flight.
func TestCASClone_LeaderCancellationFollowersStillSucceedWithRacing(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)
	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	gate := make(chan struct{})
	v, rec := newRecordedVenv(gate)

	leaderCtx, cancelLeader := context.WithCancel(t.Context())
	defer cancelLeader()

	leaderErr := make(chan error, 1)

	go func() {
		leaderErr <- cloneInto(leaderCtx, v, storePath, repoURL, "main", filepath.Join(tempDir, "leader"))
	}()

	<-rec.firstLsRemote

	const followers = 3

	errs := make([]error, followers)

	var wg sync.WaitGroup

	for i := range followers {
		wg.Go(func() {
			errs[i] = cloneInto(t.Context(), v, storePath, repoURL, "main",
				filepath.Join(tempDir, "follower-"+strconv.Itoa(i)))
		})
	}

	cancelLeader()
	require.ErrorIs(t, <-leaderErr, context.Canceled)

	close(gate)
	wg.Wait()

	for i := range followers {
		require.NoError(t, errs[i], "follower %d", i)
		assert.FileExists(t, filepath.Join(tempDir, "follower-"+strconv.Itoa(i), "main.tf"), "follower %d", i)
	}
}

// TestCASClone_SemverTagProbePersistsAcrossInstancesUntilTTL pins the
// persisted probe end to end: a second CAS over the same store clones a
// semver tag without ls-remote, until [cas.DefaultImmutableProbeTTL] has
// passed. The clones run inside a synctest bubble so the TTL is stepped
// over with no real wait, against the synthetic clock the probe cache
// reads. The server starts outside the bubble, and the clock only moves
// between clones: a goroutine waiting on a running git process is not
// durably blocked, so the bubble would not advance while one is in
// flight.
func TestCASClone_SemverTagProbePersistsAcrossInstancesUntilTTL(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile(t.Context(), "main.tf", []byte("# tagged"), "init"))
	require.NoError(t, srv.Tag(t.Context(), "v1.0.0"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")
	v, rec := newRecordedVenv(nil)

	synctest.Test(t, func(t *testing.T) {
		for i, dst := range []string{"first", "second"} {
			require.NoError(t, cloneInto(t.Context(), v, storePath, repoURL, "v1.0.0", filepath.Join(tempDir, dst)))
			assert.Equal(t, 1, rec.count("ls-remote"), "clone %d", i)
		}

		synctest.Sleep(cas.DefaultImmutableProbeTTL + time.Minute)

		require.NoError(t, cloneInto(t.Context(), v, storePath, repoURL, "v1.0.0", filepath.Join(tempDir, "third")))
		assert.Equal(t, 2, rec.count("ls-remote"), "an expired entry must be re-probed")
		assert.FileExists(t, filepath.Join(tempDir, "third", "main.tf"))
	})
}

// TestCASClone_OfflineServesCachedSourceWithoutNetwork pins the offline
// happy path: once a branch has been cloned, a later offline clone of it
// is answered entirely from the persisted probe and the tree store. The
// server is shut down first so any network attempt would fail loudly.
func TestCASClone_OfflineServesCachedSourceWithoutNetwork(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile(t.Context(), "main.tf", []byte("# offline"), "init"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	online, _ := newRecordedVenv(nil)
	require.NoError(t, cloneInto(t.Context(), online, storePath, repoURL, "main", filepath.Join(tempDir, "online")))

	require.NoError(t, srv.Close())

	v, rec := newRecordedVenv(nil)
	dst := filepath.Join(tempDir, "offline")

	require.NoError(t, cloneInto(t.Context(), v, storePath, repoURL, "main", dst, cas.WithOffline()))

	assert.FileExists(t, filepath.Join(dst, "main.tf"))
	assert.Equal(t, 0, rec.count("ls-remote"))
	assert.Equal(t, 0, rec.count("fetch"))
	assert.Equal(t, 0, rec.count("clone"))
}

// TestCASClone_OfflinePinnedSHAServedFromGitStore pins that a full commit
// SHA already fetched into the central git store resolves offline through
// the local rev-parse path rather than the probe cache.
func TestCASClone_OfflinePinnedSHAServedFromGitStore(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)
	headHash := resolveHead(t, repoURL)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	online, _ := newRecordedVenv(nil)
	require.NoError(t, cloneInto(t.Context(), online, storePath, repoURL, headHash, filepath.Join(tempDir, "online")))

	v, rec := newRecordedVenv(nil)
	dst := filepath.Join(tempDir, "offline")

	require.NoError(t, cloneInto(t.Context(), v, storePath, repoURL, headHash, dst, cas.WithOffline()))

	assert.FileExists(t, filepath.Join(dst, "main.tf"))
	assert.Equal(t, 0, rec.count("ls-remote"))
	assert.Equal(t, 0, rec.count("fetch"))
	assert.Equal(t, 0, rec.count("clone"))
}

// TestCASClone_OfflineAbbreviatedSHAServedFromGitStore pins the ref shape
// ls-remote can never name: an abbreviated commit SHA is never recorded in
// the probe cache, so offline it has only the local rev-parse to resolve
// it. A run that populated the store must therefore be enough to serve the
// same pin offline, as the flag documentation promises.
func TestCASClone_OfflineAbbreviatedSHAServedFromGitStore(t *testing.T) {
	t.Parallel()

	const abbrevLen = 12

	repoURL := startTestServer(t)
	abbrev := resolveHead(t, repoURL)[:abbrevLen]

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	online, _ := newRecordedVenv(nil)
	require.NoError(t, cloneInto(t.Context(), online, storePath, repoURL, abbrev, filepath.Join(tempDir, "online")))

	v, rec := newRecordedVenv(nil)
	dst := filepath.Join(tempDir, "offline")

	require.NoError(t, cloneInto(t.Context(), v, storePath, repoURL, abbrev, dst, cas.WithOffline()))

	assert.FileExists(t, filepath.Join(dst, "main.tf"))
	assert.Equal(t, 0, rec.count("ls-remote"))
	assert.Equal(t, 0, rec.count("fetch"))
	assert.Equal(t, 0, rec.count("clone"))
}

// TestCASClone_OfflineMissFailsWithoutFallbackClone pins the offline miss:
// a source the store has never seen fails with the typed error, and none
// of ls-remote, fetch, or the temporary-clone fallback runs.
func TestCASClone_OfflineMissFailsWithoutFallbackClone(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)
	tempDir := helpers.TmpDirWOSymlinks(t)

	v, rec := newRecordedVenv(nil)

	err := cloneInto(t.Context(), v, filepath.Join(tempDir, "store"), repoURL, "main",
		filepath.Join(tempDir, "dst"), cas.WithOffline())
	require.ErrorIs(t, err, cas.ErrCASOffline)

	var miss *cas.OfflineMissError

	require.ErrorAs(t, err, &miss)
	assert.Equal(t, repoURL, miss.Source)
	assert.Equal(t, "main", miss.Ref)

	assert.Equal(t, 0, rec.count("ls-remote"))
	assert.Equal(t, 0, rec.count("fetch"))
	assert.Equal(t, 0, rec.count("clone"))
}

// TestCASClone_OfflineProbeHitWithMissingTreeFails pins the fetch-side
// guard: a persisted probe can answer offline, but when the tree it names
// is gone from the store the clone fails with the typed error instead of
// fetching or falling back to a temporary clone.
func TestCASClone_OfflineProbeHitWithMissingTreeFails(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)
	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	online, _ := newRecordedVenv(nil)
	require.NoError(t, cloneInto(t.Context(), online, storePath, repoURL, "main", filepath.Join(tempDir, "online")))

	c, err := newStoreCAS(storePath)
	require.NoError(t, err)
	require.NoError(t, online.FS.RemoveAll(c.TreeStore().Path()))

	v, rec := newRecordedVenv(nil)

	err = cloneInto(t.Context(), v, storePath, repoURL, "main", filepath.Join(tempDir, "offline"), cas.WithOffline())
	require.ErrorIs(t, err, cas.ErrCASOffline)

	assert.Equal(t, 0, rec.count("ls-remote"))
	assert.Equal(t, 0, rec.count("fetch"))
	assert.Equal(t, 0, rec.count("clone"))
}

// TestCASClone_OfflineMissRedactsCredentials pins that neither offline
// miss, the probe-side one against an empty store nor the fetch-side one
// behind a persisted probe whose tree is gone, carries the token a source
// URL embeds anywhere in its error chain. The chain is rendered as a whole
// because that rendering is what the fallback warning logs.
func TestCASClone_OfflineMissRedactsCredentials(t *testing.T) {
	t.Parallel()

	const token = "secret-token"

	repoURL := startTestServer(t)

	u, err := url.Parse(repoURL)
	require.NoError(t, err)

	u.User = url.UserPassword("oauth2", token)
	credURL := u.String()

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	v, rec := newRecordedVenv(nil)

	probeMiss := cloneInto(t.Context(), v, storePath, credURL, "main",
		filepath.Join(tempDir, "probe-miss"), cas.WithOffline())
	require.ErrorIs(t, probeMiss, cas.ErrCASOffline)
	assert.NotContains(t, probeMiss.Error(), token, "the probe-side miss must not render the token")

	online, _ := newRecordedVenv(nil)
	require.NoError(t, cloneInto(t.Context(), online, storePath, credURL, "main", filepath.Join(tempDir, "online")))

	c, err := newStoreCAS(storePath)
	require.NoError(t, err)
	require.NoError(t, online.FS.RemoveAll(c.TreeStore().Path()))

	fetchMiss := cloneInto(t.Context(), v, storePath, credURL, "main",
		filepath.Join(tempDir, "fetch-miss"), cas.WithOffline())
	require.ErrorIs(t, fetchMiss, cas.ErrCASOffline)
	assert.NotContains(t, fetchMiss.Error(), token, "the fetch-side miss must not render the token")

	var miss *cas.OfflineMissError

	require.ErrorAs(t, fetchMiss, &miss)
	assert.Equal(t, repoURL, miss.Source)

	assert.Equal(t, 0, rec.count("ls-remote"))
	assert.Equal(t, 0, rec.count("fetch"))
	assert.Equal(t, 0, rec.count("clone"))
}
