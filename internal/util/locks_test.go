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
// same key see a serialized critical section: 10 goroutines each increment
// a shared counter twice inside the lock; with correct serialization the
// final count must be exactly 20 (no lost updates).
func TestKeyLocksSharedKeySerializes(t *testing.T) {
	t.Parallel()

	kl := util.NewKeyLocks()

	var (
		counter int
		wg      sync.WaitGroup
	)

	for range 10 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			kl.Lock("test-key")
			defer kl.Unlock("test-key")

			counter++
			counter++
		}()
	}

	wg.Wait()

	require.Equal(t, 20, counter, "serialized increments must see exactly 20, any other value means a lost update")
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
