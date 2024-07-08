package util

import (
	"sync"
	"testing"
)

// TestKeyLocksBasic verifies basic locking and unlocking behavior.
func TestKeyLocksBasic(t *testing.T) {
	t.Parallel()
	kl := NewKeyLocks()

	kl.Lock("key1")
	kl.Unlock("key1")
}

// TestKeyLocksConcurrentAccess ensures thread-safe access for multiple keys.
func TestKeyLocksConcurrentAccess(t *testing.T) {
	t.Parallel()
	kl := NewKeyLocks()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			kl.Lock(key)
			defer kl.Unlock(key)
		}(string(rune('a' + i))) // Use different keys for each goroutine
	}

	wg.Wait()
}

// TestKeyLocksStress tests the KeyLocks under high concurrency.
func TestKeyLocksStress(t *testing.T) {
	t.Parallel()
	kl := NewKeyLocks()
	const numGoroutines = 1000
	const numOperations = 100
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := string(rune('a' + (id+j)%26)) // Cycle through keys
				kl.Lock(key)
				kl.Unlock(key)
			}
		}(i)
	}

	wg.Wait()
}

// TestKeyLocksUnlockWithoutLock checks for safe behavior when unlocking without locking.
func TestKeyLocksUnlockWithoutLock(t *testing.T) {
	t.Parallel()
	kl := NewKeyLocks()

	// Directly calling Unlock should not cause issues
	kl.Unlock("nonexistent_key")
}

// TestKeyLocksLockUnlockStressWithSharedKey tests a shared key under high concurrent load.
func TestKeyLocksLockUnlockStressWithSharedKey(t *testing.T) {
	t.Parallel()
	kl := NewKeyLocks()
	const numGoroutines = 100
	const numOperations = 1000
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				kl.Lock("shared_key")
				kl.Unlock("shared_key")
			}
		}()
	}

	wg.Wait()

}
