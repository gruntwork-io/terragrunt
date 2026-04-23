// This is a white-box test file: it needs access to the unexported
// acquireGenerateLock, stackGenerateLockDir, and stackGenerateLockFile
// symbols to verify the locking implementation deterministically.
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

// TestAcquireGenerateLockSerializes is the deterministic correctness guard
// for the stack-generate lockfile. It proves the lock does what it claims
// without relying on timing-based filesystem races.
func TestAcquireGenerateLockSerializes(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	l := log.New()
	ctx := t.Context()

	// First acquire — should succeed immediately.
	lockPath1, unlocker1, err := acquireGenerateLock(ctx, l, workingDir)
	require.NoError(t, err)
	require.NotNil(t, unlocker1)
	require.Equal(t, filepath.Join(workingDir, stackGenerateLockDir, stackGenerateLockFile), lockPath1)

	// Second acquire — must NOT succeed while the first holds the lock.
	// Use vfs.TryLock directly (same primitive acquireGenerateLock polls)
	// to verify without blocking.
	_, acquired, err := vfs.TryLock(vfs.NewOSFS(), lockPath1)
	require.NoError(t, err)
	require.False(t, acquired, "lock must be held exclusively while first caller has it")

	// Release the first lock.
	require.NoError(t, unlocker1.Unlock())

	// Third acquire — should succeed now that the first is released.
	lockPath2, unlocker2, err := acquireGenerateLock(ctx, l, workingDir)
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

	workingDir := t.TempDir()
	l := log.New()

	// Goroutine A holds the lock.
	heldCtx := context.Background()
	_, heldUnlocker, err := acquireGenerateLock(heldCtx, l, workingDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = heldUnlocker.Unlock() })

	// Goroutine B tries to acquire with a context that we cancel mid-wait.
	waitCtx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)

	go func() {
		_, _, err := acquireGenerateLock(waitCtx, l, workingDir)
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
// and `terragrunt stack clean` — placing the lock there is load-bearing
// for the H2 fix from the code review.
func TestAcquireGenerateLockCreatesDir(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	l := log.New()

	lockPath, unlocker, err := acquireGenerateLock(t.Context(), l, workingDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unlocker.Unlock() })

	require.DirExists(t, filepath.Join(workingDir, stackGenerateLockDir))
	require.FileExists(t, lockPath)
}

// TestAcquireGenerateLockConcurrent asserts that many goroutines racing on
// the same lock all eventually acquire+release without error. With the
// working-dir lock in place, the total runtime should be near
// numGoroutines * 0 (lock acquisition is cheap when no one else holds it
// long enough to contend), but we mainly care that correctness holds.
func TestAcquireGenerateLockConcurrent(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	l := log.New()

	const numGoroutines = 8

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errs := make(chan error, numGoroutines)

	for range numGoroutines {
		go func() {
			defer wg.Done()

			_, unlocker, err := acquireGenerateLock(t.Context(), l, workingDir)
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
