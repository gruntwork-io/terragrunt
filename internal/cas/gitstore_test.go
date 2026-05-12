package cas_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitStoreEnsureRef_InitsAndFetches(t *testing.T) {
	t.Parallel()

	url := startTestServer(t)
	hash := resolveHead(t, url)

	store, fs, root := newTestGitStore(t)
	l := logger.CreateLogger()
	ctx := t.Context()

	repoPath, unlock, err := store.EnsureRef(ctx, l, fs, url, "main", hash, 0)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(repoPath, root), "repo path %q should be under store root %q", repoPath, root)

	_, err = fs.Stat(filepath.Join(repoPath, "HEAD"))
	require.NoError(t, err)

	require.NoError(t, unlock.Unlock())

	// Second call hits the cache-warm path: object already present, no fetch.
	_, unlock2, err := store.EnsureRef(ctx, l, fs, url, "main", hash, 0)
	require.NoError(t, err)
	require.NoError(t, unlock2.Unlock())
}

func TestGitStoreEnsureRef_PartitionsByURL(t *testing.T) {
	t.Parallel()

	url1 := startTestServer(t)
	url2 := startTestServer(t)

	store, fs, root := newTestGitStore(t)
	require.NotEmpty(t, root)

	l := logger.CreateLogger()
	ctx := t.Context()

	hash1 := resolveHead(t, url1)
	hash2 := resolveHead(t, url2)

	r1, u1, err := store.EnsureRef(ctx, l, fs, url1, "main", hash1, 0)
	require.NoError(t, err)
	require.NoError(t, u1.Unlock())

	r2, u2, err := store.EnsureRef(ctx, l, fs, url2, "main", hash2, 0)
	require.NoError(t, err)
	require.NoError(t, u2.Unlock())

	assert.NotEqual(t, r1, r2, "different URLs must map to different bare repos")
}

func TestGitStoreEnsureRefConcurrentSameURLWithRacing(t *testing.T) {
	t.Parallel()

	url := startTestServer(t)
	hash := resolveHead(t, url)

	store, fs, root := newTestGitStore(t)
	require.NotEmpty(t, root)

	l := logger.CreateLogger()

	const workers = 4

	var wg sync.WaitGroup

	errs := make([]error, workers)

	for i := range workers {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			_, unlock, err := store.EnsureRef(t.Context(), l, fs, url, "main", hash, 0)
			if err != nil {
				errs[idx] = err
				return
			}

			errs[idx] = unlock.Unlock()
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "worker %d", i)
	}
}

func TestGitStoreEnsureRef_LockHeldRespectsContextCancellation(t *testing.T) {
	t.Parallel()

	url := startTestServer(t)
	hash := resolveHead(t, url)

	store, fs, root := newTestGitStore(t)
	require.NotEmpty(t, root)

	l := logger.CreateLogger()

	// First caller takes the per-URL lock and holds it.
	repoPath, unlock, err := store.EnsureRef(t.Context(), l, fs, url, "main", hash, 0)
	require.NoError(t, err)
	require.NotEmpty(t, repoPath)
	t.Cleanup(func() { _ = unlock.Unlock() })

	// Second caller arrives with a short deadline. With the lock held it
	// must return a context error rather than block.
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()

	_, _, err = store.EnsureRef(ctx, l, fs, url, "main", hash, 0)
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second, "EnsureRef should not block past the context deadline")
	assert.True(
		t,
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"expected context error, got %v", err,
	)
}

func TestGitStoreEnsureRefLockReleaseAllowsWaiterToProceedWithRacing(t *testing.T) {
	t.Parallel()

	url := startTestServer(t)
	hash := resolveHead(t, url)

	store, fs, root := newTestGitStore(t)
	require.NotEmpty(t, root)

	l := logger.CreateLogger()

	_, unlock, err := store.EnsureRef(t.Context(), l, fs, url, "main", hash, 0)
	require.NoError(t, err)

	// Release the holder after a short delay so the waiter sees the lock open.
	go func() {
		time.Sleep(50 * time.Millisecond)

		_ = unlock.Unlock()
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	_, unlock2, err := store.EnsureRef(ctx, l, fs, url, "main", hash, 0)
	require.NoError(t, err)
	require.NoError(t, unlock2.Unlock())
}

func TestGitStoreEnsureRef_FetchFailureSurfacesError(t *testing.T) {
	t.Parallel()

	store, fs, root := newTestGitStore(t)
	require.NotEmpty(t, root)

	l := logger.CreateLogger()

	_, _, err := store.EnsureRef(t.Context(), l, fs, "file:///does/not/exist", "main", "deadbeef", 0)
	require.Error(t, err)
}

func TestGitStoreRejectsNonOSFilesystem(t *testing.T) {
	t.Parallel()

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	root := filepath.Join(helpers.TmpDirWOSymlinks(t), "gitstore")

	_, err = cas.NewGitStore(vfs.NewMemMapFS(), runner, root)
	require.ErrorIs(t, err, cas.ErrGitStoreFSNotOS)

	store, err := cas.NewGitStore(vfs.NewOSFS(), runner, root)
	require.NoError(t, err)

	_, _, err = store.EnsureRef(
		t.Context(), logger.CreateLogger(), vfs.NewMemMapFS(),
		"file:///does/not/exist", "main", "deadbeef", 0,
	)
	require.ErrorIs(t, err, cas.ErrGitStoreFSNotOS)
}

func newTestGitStore(t *testing.T) (*cas.GitStore, vfs.FS, string) {
	t.Helper()

	root := filepath.Join(helpers.TmpDirWOSymlinks(t), "gitstore")
	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	fs := vfs.NewOSFS()

	store, err := cas.NewGitStore(fs, runner, root)
	require.NoError(t, err)

	return store, fs, root
}

func resolveHead(t *testing.T, url string) string {
	t.Helper()

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	results, err := runner.LsRemote(t.Context(), url, "HEAD")
	require.NoError(t, err)
	require.NotEmpty(t, results)

	return results[0].Hash
}
