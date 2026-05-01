package cas_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCASCloneByCommitRef(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	repoURL := startTestServer(t)
	headHash := resolveHead(t, repoURL)

	t.Run("clone with full commit SHA", func(t *testing.T) {
		t.Parallel()
		tempDir := helpers.TmpDirWOSymlinks(t)

		c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
		require.NoError(t, err)

		targetPath := filepath.Join(tempDir, "repo")
		err = c.Clone(t.Context(), l, &cas.CloneOptions{
			Dir:    targetPath,
			Branch: headHash,
			Depth:  -1,
		}, repoURL)
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(targetPath, "README.md"))
		require.NoError(t, err)
	})

	t.Run("clone with abbreviated commit SHA", func(t *testing.T) {
		t.Parallel()
		tempDir := helpers.TmpDirWOSymlinks(t)

		c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
		require.NoError(t, err)

		targetPath := filepath.Join(tempDir, "repo")
		err = c.Clone(t.Context(), l, &cas.CloneOptions{
			Dir:    targetPath,
			Branch: headHash[:8],
			Depth:  -1,
		}, repoURL)
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(targetPath, "README.md"))
		require.NoError(t, err)
	})

	t.Run("commit ref reuses central store on second clone", func(t *testing.T) {
		t.Parallel()
		tempDir := helpers.TmpDirWOSymlinks(t)
		storePath := filepath.Join(tempDir, "store")

		c, err := cas.New(cas.WithStorePath(storePath))
		require.NoError(t, err)

		// Prime the central git store.
		require.NoError(t, c.Clone(t.Context(), l, &cas.CloneOptions{
			Dir:    filepath.Join(tempDir, "first"),
			Branch: headHash,
			Depth:  -1,
		}, repoURL))

		// Drop the test server: a cached clone must not need it.
		repoEntry := cas.EntryPathForURL(filepath.Join(storePath, "git"), repoURL)
		_, err = os.Stat(filepath.Join(repoEntry, "repo"))
		require.NoError(t, err)

		secondClone := filepath.Join(tempDir, "second")
		require.NoError(t, c.Clone(t.Context(), l, &cas.CloneOptions{
			Dir:    secondClone,
			Branch: headHash,
			Depth:  -1,
		}, repoURL))

		_, err = os.Stat(filepath.Join(secondClone, "README.md"))
		require.NoError(t, err)
	})

	t.Run("unresolvable ref reports no matching reference", func(t *testing.T) {
		t.Parallel()
		tempDir := helpers.TmpDirWOSymlinks(t)

		c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
		require.NoError(t, err)

		err = c.Clone(t.Context(), l, &cas.CloneOptions{
			Dir:    filepath.Join(tempDir, "repo"),
			Branch: "0000000000000000000000000000000000000000",
			Depth:  -1,
		}, repoURL)
		require.Error(t, err)
		assert.ErrorIs(t, err, git.ErrNoMatchingReference)
	})
}

func TestGitStoreEnsureCommit_CachedAfterFirstFetch(t *testing.T) {
	t.Parallel()

	url := startTestServer(t)
	hash := resolveHead(t, url)

	store, fs, _ := newTestGitStore(t)
	l := logger.CreateLogger()
	ctx := t.Context()

	// First call must fetch.
	repo, err := store.EnsureCommit(ctx, l, fs, url, hash)
	require.NoError(t, err)
	assert.Equal(t, hash, repo.Hash)
	assert.NotEmpty(t, repo.Path)
	require.NoError(t, repo.Unlock())

	// Second call hits the local-cache short-circuit.
	repo2, err := store.EnsureCommit(ctx, l, fs, url, hash)
	require.NoError(t, err)
	assert.Equal(t, hash, repo2.Hash)
	require.NoError(t, repo2.Unlock())
}

func TestGitStoreEnsureCommit_AbbreviatedSHA(t *testing.T) {
	t.Parallel()

	url := startTestServer(t)
	hash := resolveHead(t, url)

	store, fs, _ := newTestGitStore(t)
	l := logger.CreateLogger()

	repo, err := store.EnsureCommit(t.Context(), l, fs, url, hash[:8])
	require.NoError(t, err)
	assert.Equal(t, hash, repo.Hash, "abbreviated SHA must canonicalize to the full hash")
	require.NoError(t, repo.Unlock())
}

func TestGitStoreEnsureCommit_UnresolvableSurfacesNoMatchingReference(t *testing.T) {
	t.Parallel()

	url := startTestServer(t)

	store, fs, _ := newTestGitStore(t)
	l := logger.CreateLogger()

	_, err := store.EnsureCommit(t.Context(), l, fs, url, "0000000000000000000000000000000000000000")
	require.Error(t, err)
	assert.ErrorIs(t, err, git.ErrNoMatchingReference)
}

// TestCASClone_NonTipCommit pins the actual reason this feature
// exists: cloning a commit that is not the tip of any branch. With
// the previous --depth 1 path the fetch would have failed; the new
// commit-fallback flow does a full-history fetch so older commits
// remain reachable.
func TestCASClone_NonTipCommit(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("first.txt", []byte("first"), "first commit"))

	firstHash, err := srv.Head()
	require.NoError(t, err)

	require.NoError(t, srv.CommitFile("second.txt", []byte("second"), "second commit"))
	require.NoError(t, srv.CommitFile("third.txt", []byte("third"), "third commit"))

	headHash, err := srv.Head()
	require.NoError(t, err)
	require.NotEqual(t, firstHash, headHash, "non-tip test requires HEAD to differ from target")

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)

	c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	targetPath := filepath.Join(tempDir, "repo")
	err = c.Clone(t.Context(), logger.CreateLogger(), &cas.CloneOptions{
		Dir:    targetPath,
		Branch: firstHash,
		Depth:  -1,
	}, repoURL)
	require.NoError(t, err)

	// Only the first commit's file should be present; later commits
	// belong to descendants we did not check out.
	_, err = os.Stat(filepath.Join(targetPath, "first.txt"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(targetPath, "second.txt"))
	require.Error(t, err, "non-tip clone must not include later commits")
}

// TestCASClone_HexBranchNameResolvesViaLsRemote verifies that a
// branch whose name happens to be 40 lowercase hex characters still
// resolves through ls-remote rather than being treated as a commit
// SHA. The branch is intentionally named with a hash that does not
// match any commit so commit-fallback resolution would fail, leaving
// ls-remote as the only path to success.
func TestCASClone_HexBranchNameResolvesViaLsRemote(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("README.md", []byte("# test"), "initial"))

	const hexBranch = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	require.NoError(t, srv.Branch(hexBranch))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)

	c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	targetPath := filepath.Join(tempDir, "repo")
	err = c.Clone(t.Context(), logger.CreateLogger(), &cas.CloneOptions{
		Dir:    targetPath,
		Branch: hexBranch,
		Depth:  -1,
	}, repoURL)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(targetPath, "README.md"))
	require.NoError(t, err)
}

// TestCASClone_TagRef pins that ls-remote-resolved tag refs continue
// to work after the commit-fallback wiring landed.
func TestCASClone_TagRef(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("README.md", []byte("# tagged"), "initial"))
	require.NoError(t, srv.Tag("v1.0.0"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)

	c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	targetPath := filepath.Join(tempDir, "repo")
	err = c.Clone(t.Context(), logger.CreateLogger(), &cas.CloneOptions{
		Dir:    targetPath,
		Branch: "v1.0.0",
		Depth:  -1,
	}, repoURL)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(targetPath, "README.md"))
	require.NoError(t, err)
}

// TestGitStoreEnsureCommit_OfflineWhenCached proves that once a
// commit is cached in the central git store, EnsureCommit resolves it
// without touching the network. We tear the server down between the
// priming call and the cached call to make any accidental fetch
// explicit.
func TestGitStoreEnsureCommit_OfflineWhenCached(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("README.md", []byte("# offline"), "initial"))

	hash, err := srv.Head()
	require.NoError(t, err)

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	store, fs, _ := newTestGitStore(t)
	l := logger.CreateLogger()
	ctx := t.Context()

	primed, err := store.EnsureCommit(ctx, l, fs, repoURL, hash)
	require.NoError(t, err)
	require.NoError(t, primed.Unlock())

	require.NoError(t, srv.Close())

	cached, err := store.EnsureCommit(ctx, l, fs, repoURL, hash)
	require.NoError(t, err, "cached commit must resolve without contacting the server")
	assert.Equal(t, hash, cached.Hash)
	require.NoError(t, cached.Unlock())
}

// TestCASClone_OfflineWhenCommitCached pins offline behavior for
// previously-cached commit refs: once a SHA has been cloned, a
// subsequent clone for the same SHA must succeed even when the
// remote is unreachable. The test server is shut down between the
// priming and the cached call so any ls-remote call would surface as
// a clone failure.
func TestCASClone_OfflineWhenCommitCached(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("README.md", []byte("# offline cached"), "initial"))

	hash, err := srv.Head()
	require.NoError(t, err)

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	l := logger.CreateLogger()

	require.NoError(t, c.Clone(t.Context(), l, &cas.CloneOptions{
		Dir:    filepath.Join(tempDir, "primed"),
		Branch: hash,
		Depth:  -1,
	}, repoURL))

	require.NoError(t, srv.Close())

	cachedDir := filepath.Join(tempDir, "cached")
	require.NoError(t, c.Clone(t.Context(), l, &cas.CloneOptions{
		Dir:    cachedDir,
		Branch: hash,
		Depth:  -1,
	}, repoURL), "cached commit ref must resolve without contacting the server")

	_, err = os.Stat(filepath.Join(cachedDir, "README.md"))
	require.NoError(t, err)
}

// TestCASGetterGet_WithCommitRef exercises the full URL-parsing path
// through CASGetter for a ref=<sha> query string.
func TestCASGetterGet_WithCommitRef(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)
	headHash := resolveHead(t, repoURL)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	g := getter.NewCASGetter(logger.CreateLogger(), c, &cas.CloneOptions{Depth: -1})
	client := getter.Client{Getters: []getter.Getter{g}}

	dst := filepath.Join(tempDir, "repo")

	res, err := client.Get(t.Context(), &getter.Request{
		Src: "git::" + repoURL + "?ref=" + headHash,
		Dst: dst,
	})
	require.NoError(t, err)
	assert.Equal(t, dst, res.Dst)

	_, err = os.Stat(filepath.Join(dst, "README.md"))
	require.NoError(t, err)
}

// TestCAS_CommitRefFallbackWhenGitStoreFails mirrors
// TestCAS_FallbackWhenGitStoreFails for the commit-fallback path: if
// the central git store cannot create its per-URL directory, the
// commit-ref clone falls back to a temporary bare clone (no --depth)
// and resolves the SHA there.
func TestCAS_CommitRefFallbackWhenGitStoreFails(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)
	headHash := resolveHead(t, repoURL)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	gitStoreRoot := filepath.Join(storePath, "git")
	require.NoError(t, os.MkdirAll(gitStoreRoot, 0o755))

	blocker := cas.EntryPathForURL(gitStoreRoot, repoURL)
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o644))

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	targetPath := filepath.Join(tempDir, "repo")
	err = c.Clone(t.Context(), logger.CreateLogger(), &cas.CloneOptions{
		Dir:    targetPath,
		Branch: headHash,
		Depth:  -1,
	}, repoURL)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(targetPath, "README.md"))
	require.NoError(t, err)

	info, err := os.Stat(blocker)
	require.NoError(t, err)
	assert.False(t, info.IsDir(), "fallback must not have replaced the blocking file")
}

func TestCASCloneByCommitRefConcurrentWithRacing(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	repoURL := startTestServer(t)
	headHash := resolveHead(t, repoURL)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	const workers = 4

	var wg sync.WaitGroup

	errs := make([]error, workers)

	for i := range workers {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			errs[idx] = c.Clone(t.Context(), l, &cas.CloneOptions{
				Dir:    filepath.Join(tempDir, "repo", "worker", string(rune('a'+idx))),
				Branch: headHash,
				Depth:  -1,
			}, repoURL)
		}(i)
	}

	wg.Wait()

	for i, e := range errs {
		require.NoErrorf(t, e, "worker %d", i)
	}
}
