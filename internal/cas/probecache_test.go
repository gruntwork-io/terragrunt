package cas_test

import (
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

const probeCacheRoot = "/store/probes"

// newCachingResolver returns a resolver over v whose persisted cache lives
// on v.FS. Freshness is measured against time.Now, which inside a synctest
// bubble is the bubble's synthetic clock.
func newCachingResolver(v *venv.Venv, branch string) *cas.GitResolver {
	return &cas.GitResolver{
		Venv:   v,
		Logger: logger.CreateLogger(),
		Cache:  cas.NewProbeCache(probeCacheRoot),
		Branch: branch,
	}
}

func TestGitResolver_SemverTagProbeServedFromCacheUntilTTL(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &lsRemoteStub{hash: stubHash}
		v := stub.venv()
		url := "https://example.com/probe-semver.git"
		r := newCachingResolver(v, "v1.2.3")

		got, err := r.Probe(t.Context(), url)
		require.NoError(t, err)
		assert.Equal(t, stubHash, got)
		require.Equal(t, int32(1), stub.calls.Load())

		got, err = r.Probe(t.Context(), url)
		require.NoError(t, err)
		assert.Equal(t, stubHash, got)
		assert.Equal(t, int32(1), stub.calls.Load(), "a fresh semver probe must be served from the cache")

		synctest.Sleep(cas.DefaultImmutableProbeTTL - time.Minute)

		_, err = r.Probe(t.Context(), url)
		require.NoError(t, err)
		assert.Equal(t, int32(1), stub.calls.Load(), "the entry is still inside its TTL")

		synctest.Sleep(2 * time.Minute)

		_, err = r.Probe(t.Context(), url)
		require.NoError(t, err)
		assert.Equal(t, int32(2), stub.calls.Load(), "an expired entry must be re-probed")
	})
}

func TestGitResolver_MutableRefProbeNotServedFromCacheByDefault(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &lsRemoteStub{hash: stubHash}
		v := stub.venv()
		url := "https://example.com/probe-mutable.git"
		r := newCachingResolver(v, "main")

		_, err := r.Probe(t.Context(), url)
		require.NoError(t, err)

		_, err = r.Probe(t.Context(), url)
		require.NoError(t, err)
		assert.Equal(t, int32(2), stub.calls.Load(), "a branch must be re-probed so a push is seen")

		_, ok := r.Cache.Lookup(v.FS, url, "main")
		assert.True(t, ok, "the answer is still recorded so offline mode can use it")
	})
}

func TestGitResolver_MutableRefProbeServedWithinConfiguredTTL(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &lsRemoteStub{hash: stubHash}
		v := stub.venv()
		url := "https://example.com/probe-mutable-ttl.git"
		r := newCachingResolver(v, "main")
		r.MutableTTL = time.Hour

		_, err := r.Probe(t.Context(), url)
		require.NoError(t, err)

		_, err = r.Probe(t.Context(), url)
		require.NoError(t, err)
		assert.Equal(t, int32(1), stub.calls.Load())

		synctest.Sleep(2 * time.Hour)

		_, err = r.Probe(t.Context(), url)
		require.NoError(t, err)
		assert.Equal(t, int32(2), stub.calls.Load())
	})
}

// TestGitResolver_BranchNamedLikeVersionIsNotServedAsSemver pins that
// the semver TTL follows what ls-remote resolved, not the requested
// name: a branch called v1.2.3 (which ls-remote lists ahead of a tag of
// the same name) is re-probed like any other branch.
func TestGitResolver_BranchNamedLikeVersionIsNotServedAsSemver(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &lsRemoteStub{hash: stubHash, resolvedRef: "refs/heads/v1.2.3"}
		v := stub.venv()
		url := "https://example.com/probe-version-branch.git"
		r := newCachingResolver(v, "v1.2.3")

		_, err := r.Probe(t.Context(), url)
		require.NoError(t, err)

		_, err = r.Probe(t.Context(), url)
		require.NoError(t, err)
		assert.Equal(t, int32(2), stub.calls.Load(), "a branch must be re-probed whatever it is called")

		entry, ok := r.Cache.Lookup(v.FS, url, "v1.2.3")
		require.True(t, ok)
		assert.False(t, entry.Immutable, "a branch is mutable however it is named")
	})
}

func TestGitResolver_RefreshModeBypassesCacheButStillRecords(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &lsRemoteStub{hash: stubHash}
		v := stub.venv()
		url := "https://example.com/probe-refresh.git"
		r := newCachingResolver(v, "v2.0.0")
		r.Mode = cas.ProbeModeRefresh

		_, err := r.Probe(t.Context(), url)
		require.NoError(t, err)

		_, err = r.Probe(t.Context(), url)
		require.NoError(t, err)
		assert.Equal(t, int32(2), stub.calls.Load(), "refresh must ignore the entry it just wrote")

		entry, ok := r.Cache.Lookup(v.FS, url, "v2.0.0")
		require.True(t, ok)
		assert.Equal(t, stubHash, entry.Key)
	})
}

func TestGitResolver_CorruptCacheEntryIsIgnoredAndRewritten(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &lsRemoteStub{hash: stubHash}
		v := stub.venv()
		url := "https://example.com/probe-corrupt.git"
		r := newCachingResolver(v, "v3.0.0")

		path := r.Cache.EntryPath(url, "v3.0.0")
		require.NoError(t, v.FS.MkdirAll(filepath.Dir(path), cas.DefaultDirPerms))
		require.NoError(t, vfs.WriteFile(v.FS, path, []byte("{not json"), cas.RegularFilePerms))

		got, err := r.Probe(t.Context(), url)
		require.NoError(t, err)
		assert.Equal(t, stubHash, got)
		assert.Equal(t, int32(1), stub.calls.Load(), "a damaged entry must fall through to ls-remote")

		entry, ok := r.Cache.Lookup(v.FS, url, "v3.0.0")
		require.True(t, ok, "the damaged entry must have been replaced")
		assert.Equal(t, stubHash, entry.Key)
	})
}

func TestGitResolver_OfflineServesAnyRecordedProbeAndFailsOnMiss(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &lsRemoteStub{hash: stubHash}
		v := stub.venv()
		url := "https://user:token@example.com/probe-offline.git"

		_, err := newCachingResolver(v, "main").Probe(t.Context(), url)
		require.NoError(t, err)
		require.Equal(t, int32(1), stub.calls.Load())

		// Well past any TTL a mutable ref could be given.
		synctest.Sleep(48 * time.Hour)

		offline := newCachingResolver(v, "main")
		offline.Mode = cas.ProbeModeOffline

		got, err := offline.Probe(t.Context(), url)
		require.NoError(t, err)
		assert.Equal(t, stubHash, got)
		assert.Equal(t, int32(1), stub.calls.Load(), "offline must never reach ls-remote")

		missing := newCachingResolver(v, "feature")
		missing.Mode = cas.ProbeModeOffline

		_, err = missing.Probe(t.Context(), url)
		require.ErrorIs(t, err, cas.ErrCASOffline)

		var miss *cas.OfflineMissError

		require.ErrorAs(t, err, &miss)
		assert.Equal(t, "https://example.com/probe-offline.git", miss.Source, "credentials must not reach the error")
		assert.Equal(t, "feature", miss.Ref)
		assert.Equal(t, int32(1), stub.calls.Load())
	})
}

// TestGitResolver_OfflineServesProbeRecordedUnderRotatedCredential is the
// resolver-level half of TestProbeCache_CredentialRotationSharesEntry: an
// offline run reaching a repository with a new token is answered from the
// probe the old one recorded, instead of failing on a source the store
// holds.
func TestGitResolver_OfflineServesProbeRecordedUnderRotatedCredential(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &lsRemoteStub{hash: stubHash}
		v := stub.venv()

		_, err := newCachingResolver(v, "main").
			Probe(t.Context(), "https://x:token-a@example.com/probe-rotated.git")
		require.NoError(t, err)
		require.Equal(t, int32(1), stub.calls.Load())

		offline := newCachingResolver(v, "main")
		offline.Mode = cas.ProbeModeOffline

		got, err := offline.Probe(t.Context(), "https://x:token-b@example.com/probe-rotated.git")
		require.NoError(t, err)
		assert.Equal(t, stubHash, got)
		assert.Equal(t, int32(1), stub.calls.Load(), "offline must never reach ls-remote")
	})
}

func TestProbeCache_StoreAndLookupRoundTrip(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	cache := cas.NewProbeCache(probeCacheRoot)
	probedAt := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	_, ok := cache.Lookup(fsys, "https://example.com/a.git", "")
	assert.False(t, ok)

	require.NoError(t, cache.Store(fsys, "https://example.com/a.git", "", &cas.ProbeEntry{
		ProbedAt: probedAt,
		Key:      stubHash,
	}))

	entry, ok := cache.Lookup(fsys, "https://example.com/a.git", "")
	require.True(t, ok)
	assert.Equal(t,
		cas.ProbeEntry{ProbedAt: probedAt, RefDigest: cas.RefDigest("HEAD"), Key: stubHash},
		entry)

	entry, ok = cache.Lookup(fsys, "https://example.com/a.git", "HEAD")
	require.True(t, ok, "an empty ref and an explicit HEAD share one entry")
	assert.Equal(t, stubHash, entry.Key)

	_, ok = cache.Lookup(fsys, "https://example.com/b.git", "")
	assert.False(t, ok, "entries are partitioned per URL")

	assert.NotEqual(t,
		cache.EntryPath("https://example.com/a.git", "main"),
		cache.EntryPath("https://example.com/a.git", "release/main"),
	)
}

// TestProbeCache_CredentialRotationSharesEntry pins that the token in a
// source URL does not partition the cache: a run whose credential was
// rotated since the store was filled must still find the answer recorded
// for the repository, or --cas-offline fails on content it already has.
func TestProbeCache_CredentialRotationSharesEntry(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	cache := cas.NewProbeCache(probeCacheRoot)

	require.NoError(t, cache.Store(fsys, "https://x:token-a@example.com/a.git", "main", &cas.ProbeEntry{
		ProbedAt: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC),
		Key:      stubHash,
	}))

	entry, ok := cache.Lookup(fsys, "https://x:token-b@example.com/a.git", "main")
	require.True(t, ok, "a rotated credential must find the answer recorded for the same repository")
	assert.Equal(t, stubHash, entry.Key)

	_, ok = cache.Lookup(fsys, "https://x:token-b@example.com/b.git", "main")
	assert.False(t, ok, "entries are still partitioned per repository")
}

func TestProbeCache_LookupRejectsEntryWithWrongRefOrHash(t *testing.T) {
	t.Parallel()

	cache := cas.NewProbeCache(probeCacheRoot)
	url := "https://example.com/a.git"

	tests := []struct {
		name string
		body string
	}{
		{
			name: "ref digest mismatch",
			body: `{"ref_digest":"` + cas.RefDigest("other") + `","key":"` + stubHash +
				`","probed_at":"2026-09-07T12:00:00Z"}`,
		},
		{
			name: "no key",
			body: `{"ref_digest":"` + cas.RefDigest("main") + `","probed_at":"2026-09-07T12:00:00Z"}`,
		},
		{name: "empty file", body: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The entry path is a pure function of (root, URL, ref), so the
			// cases would otherwise read each other's bodies and the one
			// that lost the race would assert against the wrong guard.
			fsys := vfs.NewMemMapFS()
			path := cache.EntryPath(url, "main")
			require.NoError(t, fsys.MkdirAll(filepath.Dir(path), cas.DefaultDirPerms))
			require.NoError(t, vfs.WriteFile(fsys, path, []byte(tt.body), cas.RegularFilePerms))

			_, ok := cache.Lookup(fsys, url, "main")
			assert.False(t, ok)
		})
	}
}

func TestIsSemverTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref  string
		want bool
	}{
		{ref: "refs/tags/v1.2.3", want: true},
		{ref: "refs/tags/1.2.3", want: true},
		{ref: "refs/tags/v1.2.3-rc.1", want: true},
		{ref: "refs/tags/v1.2.3+build.7", want: true},
		{ref: "refs/tags/v1.2", want: false},
		{ref: "refs/tags/v1", want: false},
		{ref: "refs/tags/latest", want: false},
		{ref: "refs/tags/release-1.2.3", want: false},
		{ref: "refs/tags/01.2.3", want: false},
		{ref: "refs/tags/1.2.3.4", want: false},
		{ref: "refs/heads/v1.2.3", want: false},
		{ref: "refs/heads/main", want: false},
		{ref: "v1.2.3", want: false},
		{ref: "main", want: false},
		{ref: "HEAD", want: false},
		{ref: "", want: false},
		{ref: stubHash, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, cas.IsSemverTag(tt.ref))
		})
	}
}

func TestProbeTTL(t *testing.T) {
	t.Parallel()

	immutable := cas.ProbeEntry{Immutable: true}
	mutable := cas.ProbeEntry{}

	assert.Equal(t, cas.DefaultImmutableProbeTTL, cas.ProbeTTL(&immutable, 0))
	assert.Equal(t, cas.DefaultImmutableProbeTTL, cas.ProbeTTL(&immutable, time.Hour),
		"the mutable TTL never applies to an immutable source")
	assert.Equal(t, time.Duration(0), cas.ProbeTTL(&mutable, 0))
	assert.Equal(t, time.Hour, cas.ProbeTTL(&mutable, time.Hour))
}

func TestOfflineMissError_UnwrapsToErrCASOffline(t *testing.T) {
	t.Parallel()

	err := &cas.OfflineMissError{Source: "https://example.com/a.git", Ref: "main"}
	assert.ErrorIs(t, err, cas.ErrCASOffline)
}
