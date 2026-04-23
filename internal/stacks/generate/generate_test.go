// This is a white-box test file: it needs access to the unexported
// acquireGenerateLock, stackGenerateLockDir, and stackGenerateLockFile
// symbols to verify the locking implementation deterministically.
//
// Tests use vfs.NewMemMapFS() (in-memory filesystem) rather than the real
// filesystem, so they exercise the same code path as production without
// touching disk. The vfs abstraction makes this a straight swap — locks,
// Stat, MkdirAll all work against the in-memory backing.
//
//nolint:testpackage // white-box testing of unexported lock helpers
package generate

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/require"
)

// newTestFS returns an in-memory filesystem with workingDir pre-created as
// a directory. Used by tests that want to start from a valid workingDir and
// exercise only the locking logic.
func newTestFS(t *testing.T, workingDir string) vfs.FS {
	t.Helper()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, fsys.MkdirAll(workingDir, 0o755))

	return fsys
}

// TestAcquireGenerateLockSerializes is the deterministic correctness guard
// for the stack-generate lockfile. It proves the lock does what it claims
// without relying on timing-based filesystem races.
func TestAcquireGenerateLockSerializes(t *testing.T) {
	t.Parallel()

	const workingDir = "/wd"

	fsys := newTestFS(t, workingDir)
	l := log.New()
	ctx := t.Context()

	// First acquire — should succeed immediately.
	lockPath1, unlocker1, err := acquireGenerateLock(ctx, l, fsys, workingDir)
	require.NoError(t, err)
	require.NotNil(t, unlocker1)
	require.Equal(t, filepath.Join(workingDir, stackGenerateLockDir, stackGenerateLockFile), lockPath1)

	// Second acquire — must NOT succeed while the first holds the lock.
	// Use vfs.TryLock directly (same primitive acquireGenerateLock polls)
	// to verify without blocking.
	_, acquired, err := vfs.TryLock(fsys, lockPath1)
	require.NoError(t, err)
	require.False(t, acquired, "lock must be held exclusively while first caller has it")

	// Release the first lock.
	require.NoError(t, unlocker1.Unlock())

	// Third acquire — should succeed now that the first is released.
	lockPath2, unlocker2, err := acquireGenerateLock(ctx, l, fsys, workingDir)
	require.NoError(t, err)
	require.NotNil(t, unlocker2)
	require.Equal(t, lockPath1, lockPath2)
	require.NoError(t, unlocker2.Unlock())
}

// TestAcquireGenerateLockContextCancel asserts that cancelling the context
// aborts a wait for the lock. Regression guard for H1 (the previous
// implementation used a plain blocking flock that ignored cancellation).
func TestAcquireGenerateLockContextCancel(t *testing.T) {
	t.Parallel()

	const workingDir = "/wd"

	fsys := newTestFS(t, workingDir)
	l := log.New()

	// Goroutine A holds the lock.
	_, heldUnlocker, err := acquireGenerateLock(context.Background(), l, fsys, workingDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = heldUnlocker.Unlock() })

	// Goroutine B tries to acquire with a context that we cancel mid-wait.
	waitCtx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)

	go func() {
		_, _, err := acquireGenerateLock(waitCtx, l, fsys, workingDir)
		done <- err
	}()

	// Let the wait goroutine enter its poll loop, then cancel.
	time.Sleep(250 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("acquireGenerateLock did not return after ctx cancel")
	}
}

// TestAcquireGenerateLockCreatesDir asserts that the helper creates the
// .terragrunt-stack/ subdirectory if it doesn't exist yet. The lockfile
// lives inside this dir so it's covered by the user's existing .gitignore
// and `terragrunt stack clean`.
//
// Only the directory is asserted — whether the lockfile itself exists as a
// filesystem artifact is backend-specific (flock on OS creates an empty
// file; the in-memory MemMapFS tracks locks in a mutex map without a file).
// We care about directory creation here; the lock primitive has its own
// serialization guarantees tested in TestAcquireGenerateLockSerializes.
func TestAcquireGenerateLockCreatesDir(t *testing.T) {
	t.Parallel()

	const workingDir = "/wd"

	fsys := newTestFS(t, workingDir)
	l := log.New()

	_, unlocker, err := acquireGenerateLock(t.Context(), l, fsys, workingDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unlocker.Unlock() })

	stackDirInfo, err := fsys.Stat(filepath.Join(workingDir, stackGenerateLockDir))
	require.NoError(t, err)
	require.True(t, stackDirInfo.IsDir())
}

// TestAcquireGenerateLockMissingWorkingDir asserts that a nonexistent
// --working-dir fails loudly instead of being silently created. Regression
// guard for a prior behavior where MkdirAll on the lock-dir's parent would
// auto-create a missing working directory and discovery would then report
// "No stack files found" as a successful no-op — masking user typos.
func TestAcquireGenerateLockMissingWorkingDir(t *testing.T) {
	t.Parallel()

	// Empty in-memory filesystem, so /wd does not exist.
	fsys := vfs.NewMemMapFS()
	l := log.New()

	const missing = "/wd-does-not-exist"

	_, _, err := acquireGenerateLock(t.Context(), l, fsys, missing)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not exist")

	// And verify the missing dir was NOT created as a side effect.
	_, statErr := fsys.Stat(missing)
	require.Error(t, statErr, "working dir must not be auto-created on failure")
}

// TestAcquireGenerateLockWorkingDirIsFile asserts that passing a file (not a
// directory) as working-dir is rejected with a clear error.
func TestAcquireGenerateLockWorkingDirIsFile(t *testing.T) {
	t.Parallel()

	const filePath = "/not-a-dir"

	fsys := vfs.NewMemMapFS()

	// Create a regular file (not a dir) at the path.
	f, err := fsys.Create(filePath)
	require.NoError(t, err)
	_, err = f.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	l := log.New()

	_, _, err = acquireGenerateLock(t.Context(), l, fsys, filePath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a directory")
}

// TestAcquireGenerateLockConcurrent asserts that many goroutines racing on
// the same lock all eventually acquire+release without error.
func TestAcquireGenerateLockConcurrent(t *testing.T) {
	t.Parallel()

	const workingDir = "/wd"

	fsys := newTestFS(t, workingDir)
	l := log.New()

	const numGoroutines = 8

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errs := make(chan error, numGoroutines)

	for range numGoroutines {
		go func() {
			defer wg.Done()

			_, unlocker, err := acquireGenerateLock(t.Context(), l, fsys, workingDir)
			if err != nil {
				errs <- err

				return
			}

			// Simulate critical-section work.
			time.Sleep(10 * time.Millisecond)

			errs <- unlocker.Unlock()
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
}
