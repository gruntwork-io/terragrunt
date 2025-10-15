package cas_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore(t *testing.T) {
	t.Parallel()

	t.Run("custom path", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		customPath := filepath.Join(tempDir, "custom-store")

		store := cas.NewStore(customPath)
		assert.Equal(t, customPath, store.Path())
	})
}

func TestStore_NeedsWrite(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	store := cas.NewStore(tempDir)

	// Create a fake content file
	testHash := "abcdef123456"
	// Create partition directory
	partitionDir := filepath.Join(store.Path(), testHash[:2])
	err := os.MkdirAll(partitionDir, 0755)
	require.NoError(t, err, "Failed to create partition directory")

	testPath := filepath.Join(partitionDir, testHash)
	err = os.WriteFile(testPath, []byte("test"), 0644)
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
			assert.Equal(t, tt.want, store.NeedsWrite(tt.hash))
		})
	}
}

func TestStore_AcquireLock(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	store := cas.NewStore(tempDir)
	testHash := "abcdef1234567890abcdef1234567890abcdef12"

	// Test successful lock acquisition
	lock, err := store.AcquireLock(testHash)
	require.NoError(t, err)
	assert.NotNil(t, lock)

	// Verify lock file exists
	lockPath := filepath.Join(tempDir, testHash[:2], testHash+".lock")
	assert.FileExists(t, lockPath)

	// Clean up
	err = lock.Unlock()
	require.NoError(t, err)
}

func TestStore_TryAcquireLock(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	store := cas.NewStore(tempDir)
	testHash := "abcdef1234567890abcdef1234567890abcdef12"

	// Test successful lock acquisition
	lock1, acquired, err := store.TryAcquireLock(testHash)
	require.NoError(t, err)
	assert.True(t, acquired)
	assert.NotNil(t, lock1)

	// Test lock contention - should fail to acquire
	lock2, acquired, err := store.TryAcquireLock(testHash)
	require.NoError(t, err)
	assert.False(t, acquired)
	assert.Nil(t, lock2)

	// Clean up first lock
	err = lock1.Unlock()
	require.NoError(t, err)

	// Now should be able to acquire again
	lock3, acquired, err := store.TryAcquireLock(testHash)
	require.NoError(t, err)
	assert.True(t, acquired)
	assert.NotNil(t, lock3)

	// Clean up
	err = lock3.Unlock()
	assert.NoError(t, err)
}

func TestStore_LockConcurrency(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	store := cas.NewStore(tempDir)
	testHash := "abcdef1234567890abcdef1234567890abcdef12"

	// Test that multiple goroutines can't acquire the same lock
	done := make(chan bool, 2)
	acquired := make(chan bool, 2)

	// First goroutine acquires lock and holds it briefly
	go func() {
		lock, err := store.AcquireLock(testHash)
		assert.NoError(t, err)

		acquired <- true

		time.Sleep(100 * time.Millisecond) // Hold lock briefly

		err = lock.Unlock()
		assert.NoError(t, err)

		done <- true
	}()

	// Second goroutine tries to acquire the same lock
	go func() {
		<-acquired // Wait for first goroutine to acquire lock

		// Should block until first lock is released
		start := time.Now()
		lock, err := store.AcquireLock(testHash)
		elapsed := time.Since(start)

		assert.NoError(t, err)
		assert.Greater(t, elapsed, 50*time.Millisecond, "Second lock should have been blocked")

		err = lock.Unlock()
		assert.NoError(t, err)

		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done
}

func TestStore_EnsureWithWait(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	store := cas.NewStore(tempDir)
	testHash := "abcdef1234567890abcdef1234567890abcdef12"

	t.Run("content already exists", func(t *testing.T) {
		t.Parallel()

		// Create the content manually
		partitionDir := filepath.Join(tempDir, testHash[:2])
		err := os.MkdirAll(partitionDir, 0755)
		require.NoError(t, err)

		contentPath := filepath.Join(partitionDir, testHash)
		err = os.WriteFile(contentPath, []byte("existing content"), 0644)
		require.NoError(t, err)

		// EnsureWithWait should return false (no write needed)
		needsWrite, lock, err := store.EnsureWithWait(testHash)
		require.NoError(t, err)
		assert.False(t, needsWrite)
		assert.Nil(t, lock)
	})

	t.Run("content doesn't exist, no contention", func(t *testing.T) {
		t.Parallel()

		testHashNew := "fedcba0987654321fedcba0987654321fedcba09"

		// EnsureWithWait should return true (write needed) and provide lock
		needsWrite, lock, err := store.EnsureWithWait(testHashNew)
		require.NoError(t, err)
		assert.True(t, needsWrite)
		assert.NotNil(t, lock)

		// Clean up
		err = lock.Unlock()
		assert.NoError(t, err)
	})
}
