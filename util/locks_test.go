package util_test

import (
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
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

// TestKeyLocksConcurrentAccess ensures thread-safe access for multiple keys.
func TestKeyLocksConcurrentAccess(t *testing.T) {
	t.Parallel()
	kl := util.NewKeyLocks()
	var counters [10]int
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
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

	for i := 0; i < 10; i++ {
		require.Equal(t, 2, counters[i], "Lock/unlock cycle for each key should be completed")
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

// TestKeyLocksLockUnlockStressWithSharedKey tests a shared key under high concurrent load.
func TestKeyLocksLockUnlockStressWithSharedKey(t *testing.T) {
	t.Parallel()
	kl := util.NewKeyLocks()
	const numGoroutines = 100
	const numOperations = 1000
	var wg sync.WaitGroup
	var counter int

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			kl.Lock("shared_key")
			defer kl.Unlock("shared_key")
			for j := 0; j < numOperations; j++ {
				counter++
				counter++
			}
		}()
	}

	wg.Wait()

	require.Equal(t, numGoroutines*numOperations*2, counter, "All lock/unlock cycles should be completed")
}
