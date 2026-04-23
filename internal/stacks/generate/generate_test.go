// This is a white-box test file: it needs access to the unexported
// validateWorkingDir helper and generateMutex, and to the exported sentinel
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

// TestValidateWorkingDirMissing asserts that a nonexistent --working-dir
// fails with the typed ErrWorkingDirNotFound sentinel instead of being
// silently created. Regression guard for a prior behavior where MkdirAll
// on the lock-dir's parent would auto-create a missing working directory
// and discovery would then report "No stack files found" as a successful
// no-op, masking user typos.
func TestValidateWorkingDirMissing(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "does-not-exist")

	err := validateWorkingDir(missing)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrWorkingDirNotFound)

	// And verify the missing dir was NOT created as a side effect.
	_, statErr := os.Stat(missing)
	require.True(t, os.IsNotExist(statErr), "working dir must not be auto-created on failure")
}

// TestValidateWorkingDirIsFile asserts that passing a file (not a
// directory) as working-dir is rejected with the typed sentinel
// ErrWorkingDirNotDirectory.
func TestValidateWorkingDirIsFile(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(filePath, []byte("hello"), 0o600))

	err := validateWorkingDir(filePath)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrWorkingDirNotDirectory)
}

// TestValidateWorkingDirValid asserts that a real existing directory
// passes validation cleanly.
func TestValidateWorkingDirValid(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateWorkingDir(t.TempDir()))
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
