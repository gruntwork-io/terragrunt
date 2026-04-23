package util_test

import (
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/stretchr/testify/require"
)

// TestKeyLocksBasic verifies basic locking and unlocking behavior.
func TestKeyLocksBasic(t *testing.T) {
	t.Parallel()

	kl := util.NewKeyLocks()

	var counter int // Counter to track lock/unlock cycles

	kl.Lock("key1")

	counter++

	kl.Unlock("key1")

	counter++

	require.Equal(t, 2, counter, "Lock/unlock cycle should be completed")
}

// TestKeyLocksSharedKeySerializes asserts that concurrent holders of the
// same key are serialized one-at-a-time, not that distinct keys run in
// parallel. Multi-key parallelism is covered by
// TestGenerateMutexIndependentKeys in internal/stacks/generate.
func TestKeyLocksSharedKeySerializes(t *testing.T) {
	t.Parallel()

	kl := util.NewKeyLocks()

	var (
		counters [10]int
		wg       sync.WaitGroup
	)

	for i := range 10 {
		wg.Add(1)

		go func(key string, idx int) {
			defer wg.Done()

			kl.Lock(key)
			defer kl.Unlock(key)

			counters[idx]++
			counters[idx]++
		}("test-key", i)
	}

	wg.Wait()

	for i := range 10 {
		require.Equal(t, 2, counters[i], "Lock/unlock cycle for each goroutine should be completed")
	}
}

// TestKeyLocksUnlockWithoutLock checks for safe behavior when unlocking without locking.
func TestKeyLocksUnlockWithoutLock(t *testing.T) {
	t.Parallel()

	kl := util.NewKeyLocks()

	require.NotPanics(t, func() {
		kl.Unlock("nonexistent_key")
	}, "Unlocking without locking should not panic")
}

// TestKeyLocksEntriesCleanedUp asserts that Unlock removes the underlying
// entry when the last holder releases, so callers that Lock/Unlock many
// distinct keys do not leak map entries.
func TestKeyLocksEntriesCleanedUp(t *testing.T) {
	t.Parallel()

	kl := util.NewKeyLocks()
	require.Equal(t, 0, kl.Len())

	kl.Lock("a")
	kl.Lock("b")
	require.Equal(t, 2, kl.Len())

	kl.Unlock("a")
	require.Equal(t, 1, kl.Len())

	kl.Unlock("b")
	require.Equal(t, 0, kl.Len(), "all entries should be cleaned up after the last holder releases")
}

// TestKeyLocksEntryRetainedWhileWaiter asserts that a waiter's bumped
// refcount keeps the entry alive until the waiter, too, releases. Proves
// the cleanup is safe under contention, not just sequential use.
func TestKeyLocksEntryRetainedWhileWaiter(t *testing.T) {
	t.Parallel()

	kl := util.NewKeyLocks()
	kl.Lock("k")

	waiterDone := make(chan struct{})

	go func() {
		kl.Lock("k")
		kl.Unlock("k")
		close(waiterDone)
	}()

	// Main goroutine releases; waiter then acquires+releases in sequence.
	// After both are done, the entry must be cleaned up (refcount went
	// main=1 -> both=2 -> waiter=1 -> 0).
	kl.Unlock("k")
	<-waiterDone

	require.Equal(t, 0, kl.Len(), "entry must be deleted after the last holder (originally the waiter) releases")
}

// TestKeyLocksLockUnlockStressWithSharedKey tests a shared key under high concurrent load.
func TestKeyLocksLockUnlockStressWithSharedKey(t *testing.T) {
	t.Parallel()

	kl := util.NewKeyLocks()

	const (
		numGoroutines = 100
		numOperations = 1000
	)

	var (
		wg      sync.WaitGroup
		counter int
	)

	for range numGoroutines {
		wg.Add(1)

		go func() {
			defer wg.Done()

			kl.Lock("shared_key")
			defer kl.Unlock("shared_key")

			for range numOperations {
				counter++
				counter++
			}
		}()
	}

	wg.Wait()

	require.Equal(t, numGoroutines*numOperations*2, counter, "All lock/unlock cycles should be completed")
}
