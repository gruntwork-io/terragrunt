package cas

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCAS_ConcurrencyControl(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logger := log.New()

	opts := Options{
		StorePath:           tmpDir,
		MaxConcurrentClones: 2, // Limit to 2 concurrent clones
		RetryMaxAttempts:    3,
		RetryBaseDelay:      10 * time.Millisecond,
		RetryMaxDelay:       100 * time.Millisecond,
	}

	cas, err := New(opts)
	require.NoError(t, err)

	// Test repository lock key generation
	t.Run("repo lock key generation", func(t *testing.T) {
		key1 := cas.getRepoLockKey("https://github.com/test/repo", "main")
		key2 := cas.getRepoLockKey("https://github.com/test/repo.git", "main")
		key3 := cas.getRepoLockKey("https://github.com/test/repo", "develop")

		// Same repo, different URL formats should generate same key
		assert.Equal(t, key1, key2)

		// Different branches should generate different keys
		assert.NotEqual(t, key1, key3)

		// Keys should be consistent
		assert.Equal(t, key1, cas.getRepoLockKey("https://github.com/test/repo", "main"))
	})

	t.Run("repository lock acquisition", func(t *testing.T) {
		ctx := context.Background()

		// Acquire first lock
		lock1, err := cas.acquireRepoLock(ctx, logger, "https://github.com/test/repo1", "main")
		require.NoError(t, err)
		require.NotNil(t, lock1)

		// Acquire second lock (should succeed as limit is 2)
		lock2, err := cas.acquireRepoLock(ctx, logger, "https://github.com/test/repo1", "main")
		require.NoError(t, err)
		require.NotNil(t, lock2)

		// Try to acquire third lock (should fail with timeout)
		ctx3, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()

		_, err = cas.acquireRepoLock(ctx3, logger, "https://github.com/test/repo1", "main")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "lock acquisition timeout")

		// Release first lock
		err = lock1.Unlock()
		require.NoError(t, err)

		// Now third lock should succeed
		lock3, err := cas.acquireRepoLock(ctx, logger, "https://github.com/test/repo1", "main")
		require.NoError(t, err)
		require.NotNil(t, lock3)

		// Clean up
		assert.NoError(t, lock2.Unlock())
		assert.NoError(t, lock3.Unlock())
	})

	t.Run("concurrent lock acquisition", func(t *testing.T) {
		ctx := context.Background()

		const numWorkers = 5
		const maxConcurrent = 2

		var wg sync.WaitGroup
		var mu sync.Mutex
		successCount := 0
		timeoutCount := 0

		// Create CAS with limited concurrency
		casLimited, err := New(Options{
			StorePath:           tmpDir + "/limited",
			MaxConcurrentClones: maxConcurrent,
			RetryMaxAttempts:    2,
			RetryBaseDelay:      10 * time.Millisecond,
			RetryMaxDelay:       50 * time.Millisecond,
		})
		require.NoError(t, err)

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				// Use a short timeout to test concurrency limits
				ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()

				lock, err := casLimited.acquireRepoLock(ctx, logger, "https://github.com/test/concurrent", "main")

				mu.Lock()
				if err != nil {
					timeoutCount++
				} else {
					successCount++
					// Hold the lock briefly to ensure contention
					time.Sleep(20 * time.Millisecond)
					lock.Unlock()
				}
				mu.Unlock()
			}(i)
		}

		wg.Wait()

		// Should have some successes and some timeouts due to concurrency limit
		assert.Greater(t, successCount, 0, "Should have some successful lock acquisitions")
		assert.Greater(t, timeoutCount, 0, "Should have some timeouts due to concurrency limit")
		assert.Equal(t, numWorkers, successCount+timeoutCount, "All workers should complete")

		t.Logf("Success: %d, Timeout: %d", successCount, timeoutCount)
	})
}

func TestCAS_BackoffCalculation(t *testing.T) {
	t.Parallel()

	opts := Options{
		RetryBaseDelay: 100 * time.Millisecond,
		RetryMaxDelay:  1 * time.Second,
	}

	cas, err := New(opts)
	require.NoError(t, err)

	// Test exponential backoff
	delay0 := cas.calculateBackoffDelay(0)
	delay1 := cas.calculateBackoffDelay(1)
	delay2 := cas.calculateBackoffDelay(2)
	delay3 := cas.calculateBackoffDelay(3)

	// Each delay should be roughly double the previous (with jitter)
	assert.True(t, delay0 >= 100*time.Millisecond)
	assert.True(t, delay1 >= 200*time.Millisecond)
	assert.True(t, delay2 >= 400*time.Millisecond)

	// Should be capped at maximum delay
	assert.True(t, delay3 <= 1500*time.Millisecond) // Max + jitter

	t.Logf("Backoff delays: %v, %v, %v, %v", delay0, delay1, delay2, delay3)
}
