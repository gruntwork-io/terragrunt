package cas_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const defaultStorePath = "/store"

func TestStore(t *testing.T) {
	t.Parallel()

	t.Run("custom path", func(t *testing.T) {
		t.Parallel()

		customPath := "/custom-store"

		store := cas.NewStore(customPath)
		assert.Equal(t, customPath, store.Path())
	})
}

func TestStore_NeedsWrite(t *testing.T) {
	t.Parallel()

	v := venvtest.New()
	storePath := defaultStorePath
	store := cas.NewStore(storePath)

	// Create a fake content file
	testHash := "abcdef123456"
	// Create partition directory
	partitionDir := filepath.Join(store.Path(), testHash[:2])
	err := v.FS.MkdirAll(partitionDir, 0755)
	require.NoError(t, err, "Failed to create partition directory")

	testPath := filepath.Join(partitionDir, testHash)
	err = vfs.WriteFile(v.FS, testPath, []byte("test"), 0644)
	require.NoError(t, err, "Failed to create test file")

	tests := []struct {
		name string
		hash string
		want bool
	}{
		{
			name: "existing content",
			hash: testHash,
			want: false,
		},
		{
			name: "non-existing content",
			hash: "nonexistent",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, store.NeedsWrite(v, tt.hash))
		})
	}
}

// TestStore_LockSerializesSameHash pins that a second holder of one hash
// waits for the first to release, and that a different hash does not.
func TestStore_LockSerializesSameHash(t *testing.T) {
	t.Parallel()

	store := cas.NewStore(filepath.Join(t.TempDir(), "store"))

	const (
		testHash  = "abcdef1234567890abcdef1234567890abcdef12"
		otherHash = "fedcba0987654321fedcba0987654321fedcba09"
	)

	unlock := store.Lock(testHash)

	otherUnlock := store.Lock(otherHash)
	otherUnlock()

	acquired := make(chan struct{})

	go func() {
		secondUnlock := store.Lock(testHash)

		close(acquired)
		secondUnlock()
	}()

	select {
	case <-acquired:
		t.Fatal("second holder acquired the hash while the first still held it")
	case <-time.After(50 * time.Millisecond):
	}

	unlock()

	select {
	case <-acquired:
	case <-time.After(5 * time.Second):
		t.Fatal("second holder never acquired the hash after release")
	}
}

// TestStore_LockSharedAcrossInstancesWithRacing pins that separate Store
// values rooted at one path share the lock, as separate CAS instances
// in one process do, so their writers of one hash never overlap.
func TestStore_LockSharedAcrossInstancesWithRacing(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(t.TempDir(), "store")

	const (
		testHash = "abcdef1234567890abcdef1234567890abcdef12"
		holders  = 8
		rounds   = 50
	)

	var (
		inside   atomic.Int32
		overlaps atomic.Int32
	)

	var g errgroup.Group

	for range holders {
		store := cas.NewStore(storePath)

		g.Go(func() error {
			for range rounds {
				unlock := store.Lock(testHash)

				if inside.Add(1) != 1 {
					overlaps.Add(1)
				}

				inside.Add(-1)
				unlock()
			}

			return nil
		})
	}

	require.NoError(t, g.Wait())
	assert.Zero(t, overlaps.Load(), "two holders were inside the critical section at once")
}

func TestStore_EnsureWithWait(t *testing.T) {
	t.Parallel()

	v := venvtest.New()
	storePath := defaultStorePath
	store := cas.NewStore(storePath)
	testHash := "abcdef1234567890abcdef1234567890abcdef12"

	t.Run("content already exists", func(t *testing.T) {
		t.Parallel()

		// Create the content manually
		partitionDir := filepath.Join(storePath, testHash[:2])
		err := v.FS.MkdirAll(partitionDir, 0755)
		require.NoError(t, err)

		contentPath := filepath.Join(partitionDir, testHash)
		err = vfs.WriteFile(v.FS, contentPath, []byte("existing content"), 0644)
		require.NoError(t, err)

		// EnsureWithWait should return false (no write needed)
		needsWrite, unlock := store.EnsureWithWait(v, testHash)
		unlock()
		assert.False(t, needsWrite)
	})

	t.Run("content doesn't exist, no contention", func(t *testing.T) {
		t.Parallel()

		testHashNew := "fedcba0987654321fedcba0987654321fedcba09"

		// EnsureWithWait should return true (write needed) and hold the lock
		needsWrite, unlock := store.EnsureWithWait(v, testHashNew)
		assert.True(t, needsWrite)

		unlock()
	})

	t.Run("written while waiting", func(t *testing.T) {
		t.Parallel()

		waitedHash := "0123456789abcdef0123456789abcdef01234567"

		writerUnlock := store.Lock(waitedHash)

		answers := make(chan bool, 1)
		started := make(chan struct{})

		go func() {
			close(started)

			needsWrite, unlock := store.EnsureWithWait(v, waitedHash)
			unlock()

			answers <- needsWrite
		}()

		<-started

		// Nothing is stored yet, so the waiter cannot have answered: it
		// saw the object missing and queued behind the lock held above.
		// An answer arriving here would mean two holders were inside.
		select {
		case <-answers:
			t.Fatal("waiter answered while another holder still held the hash")
		case <-time.After(50 * time.Millisecond):
		}

		// Publishing before the release is what forces the waiter's
		// re-check to find the content in place.
		partitionDir := filepath.Join(storePath, waitedHash[:2])
		require.NoError(t, v.FS.MkdirAll(partitionDir, 0755))
		require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(partitionDir, waitedHash), []byte("written"), 0644))

		writerUnlock()

		select {
		case needsWrite := <-answers:
			assert.False(t, needsWrite, "waiter must not be told to rewrite content published while it waited")
		case <-time.After(5 * time.Second):
			t.Fatal("waiter never returned")
		}
	})
}
