// This is a white-box test file: it needs access to the unexported
// ensureWorkingDir helper and generateMutex, and to the exported sentinel
// errors they return. The lock-path / vfs.FS tests from earlier revisions
// were removed when the filesystem lock was replaced by an in-process
// sync.Mutex (see the generateMutex doc-comment for rationale).
//
//nolint:testpackage // white-box testing of unexported helpers
package generate

import (
	"os"
	"path/filepath"
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

// TestGenerateMutexSerializes asserts that the package-level generateMutex
// serializes two concurrent holders. A second Lock() must block while the
// first holds, and unblock after Unlock().
//
// Cannot use t.Parallel because it acquires the package-level mutex; a
// concurrent test that also runs GenerateStacks (or grabs the mutex for
// any other reason) would deadlock with this test.
//
//nolint:paralleltest // acquires package-level generateMutex
func TestGenerateMutexSerializes(t *testing.T) {
	generateMutex.Lock()

	locked := make(chan struct{})

	go func() {
		generateMutex.Lock()
		defer generateMutex.Unlock()

		close(locked)
	}()

	// Prove the second Lock() is blocked: 50ms is generous for goroutine
	// scheduling, far short enough that the test is fast.
	select {
	case <-locked:
		t.Fatal("second Lock() returned while the first was still holding")
	case <-time.After(50 * time.Millisecond):
	}

	generateMutex.Unlock()

	// Now the second Lock() must succeed promptly.
	select {
	case <-locked:
	case <-time.After(2 * time.Second):
		t.Fatal("second Lock() did not unblock after the first Unlock()")
	}
}

// TestGenerateMutexConcurrent asserts that many goroutines contending on
// the mutex all eventually acquire+release without error and observe a
// serialized critical section (no concurrent holders at any instant).
//
//nolint:paralleltest // acquires package-level generateMutex
func TestGenerateMutexConcurrent(t *testing.T) {
	const numGoroutines = 16

	var (
		active    int
		maxActive int
	)

	g, _ := errgroup.WithContext(t.Context())

	for range numGoroutines {
		g.Go(func() error {
			generateMutex.Lock()
			active++

			if active > maxActive {
				maxActive = active
			}

			// Short critical section.
			time.Sleep(5 * time.Millisecond)

			active--
			generateMutex.Unlock()

			return nil
		})
	}

	require.NoError(t, g.Wait())
	require.Equal(t, 1, maxActive, "mutex must enforce exactly one holder at a time")
}
