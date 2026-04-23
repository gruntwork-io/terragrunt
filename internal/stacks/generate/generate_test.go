// This is a white-box test file: it needs access to the unexported
// ensureWorkingDir helper and StackGenerator, and to the exported sentinel
// errors they return. The lock-path / vfs.FS tests from earlier revisions
// were removed when the filesystem lock was replaced by an in-process
// manager.
//
//nolint:testpackage // white-box testing of unexported helpers
package generate

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// TestEnsureWorkingDirCreatesMissing asserts that ensureWorkingDir creates
// a missing --working-dir (mkdir -p semantics) instead of returning an
// error. Lets fresh CI environments run `terragrunt stack generate` without
// pre-creating the directory.
func TestEnsureWorkingDirCreatesMissing(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "nested", "does-not-exist")

	require.NoError(t, ensureWorkingDir(missing))

	info, err := os.Stat(missing)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

// TestEnsureWorkingDirRejectsFile asserts that passing a regular file (not
// a directory) as --working-dir is rejected with the typed sentinel
// ErrWorkingDirNotDirectory. We create the missing case silently but a
// misuse where workingDir names an existing file is still an error.
func TestEnsureWorkingDirRejectsFile(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(filePath, []byte("hello"), 0o600))

	err := ensureWorkingDir(filePath)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrWorkingDirNotDirectory)
}

// TestEnsureWorkingDirExisting asserts that ensureWorkingDir is a no-op on
// a directory that already exists.
func TestEnsureWorkingDirExisting(t *testing.T) {
	t.Parallel()

	require.NoError(t, ensureWorkingDir(t.TempDir()))
}

// TestGenerateMutexSerializes asserts that the package-level
// generateLockManager serializes two concurrent holders on the same key. A
// second Lock() must block while the first holds, and unblock after the
// first Unlock() is called.
func TestGenerateMutexSerializes(t *testing.T) {
	t.Parallel()

	g := NewStackGenerator()
	key := t.TempDir()
	g.lockManager.Lock(key)

	var once sync.Once

	unlock := func() {
		once.Do(func() { g.lockManager.Unlock(key) })
	}

	t.Cleanup(unlock)

	locked := make(chan struct{})

	go func() {
		g.lockManager.Lock(key)
		defer g.lockManager.Unlock(key)

		close(locked)
	}()

	// Prove the second Lock() is blocked: 50ms is generous for goroutine
	// scheduling, far short enough that the test is fast.
	select {
	case <-locked:
		t.Fatal("second Lock() returned while the first was still holding")
	case <-time.After(50 * time.Millisecond):
	}

	unlock()

	// Now the second Lock() must succeed promptly.
	select {
	case <-locked:
	case <-time.After(2 * time.Second):
		t.Fatal("second Lock() did not unblock after the first Unlock()")
	}
}

// TestGenerateMutexConcurrent asserts that many goroutines contending on
// the same key all eventually acquire+release without error and observe a
// serialized critical section (no concurrent holders at any instant).
func TestGenerateMutexConcurrent(t *testing.T) {
	t.Parallel()

	const numGoroutines = 16

	var (
		active    int
		maxActive int
	)

	g := NewStackGenerator()
	key := t.TempDir()
	eg, _ := errgroup.WithContext(t.Context())

	for range numGoroutines {
		eg.Go(func() error {
			g.lockManager.Lock(key)
			active++

			if active > maxActive {
				maxActive = active
			}

			// Short critical section.
			time.Sleep(5 * time.Millisecond)

			active--
			g.lockManager.Unlock(key)

			return nil
		})
	}

	require.NoError(t, eg.Wait())
	require.Equal(t, 1, maxActive, "mutex must enforce exactly one holder at a time")
}

// TestGenerateMutexIndependentKeys asserts that two different keys can
// be held simultaneously without blocking.
func TestGenerateMutexIndependentKeys(t *testing.T) {
	t.Parallel()

	g := NewStackGenerator()
	key1 := filepath.Join(t.TempDir(), "1")
	key2 := filepath.Join(t.TempDir(), "2")

	g.lockManager.Lock(key1)
	defer g.lockManager.Unlock(key1)

	locked2 := make(chan struct{})

	go func() {
		g.lockManager.Lock(key2)
		defer g.lockManager.Unlock(key2)

		close(locked2)
	}()

	// Second key must succeed promptly even while the first is held.
	select {
	case <-locked2:
	case <-time.After(2 * time.Second):
		t.Fatal("second key blocked behind unrelated first key")
	}
}
