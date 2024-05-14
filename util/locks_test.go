package util

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewKeyLocks(t *testing.T) {
	t.Parallel()
	kl := NewKeyLocks()
	require.NotNil(t, kl, "NewKeyLocks() should not return nil")
	require.Empty(t, kl.locks, "NewKeyLocks() should create an empty map")
}

func TestLockUnlock(t *testing.T) {
	t.Parallel()
	kl := NewKeyLocks()
	key := "testkey"

	kl.Lock(key)
	require.Contains(t, kl.locks, key, "Lock should create a lock for key: %s", key)

	kl.Unlock(key)

	kl.Lock(key)
	kl.Unlock(key)
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()
	kl := NewKeyLocks()
	key := "concurrentKey"
	wg := sync.WaitGroup{}
	sharedResource := 0

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			kl.Lock(key)
			defer kl.Unlock(key)
			time.Sleep(10 * time.Millisecond)
			sharedResource++
		}()
	}

	wg.Wait()

	require.Equal(t, 100, sharedResource, "Concurrent access to shared resource managed incorrectly")
}

func TestMultipleKeys(t *testing.T) {
	t.Parallel()
	kl := NewKeyLocks()
	lockKey := "/tmp/project1"
	keys := []string{"key1", "key2", "key3", "key4", "key5"}
	wg := sync.WaitGroup{}
	lockState := make(map[string]bool)

	for _, key := range keys {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			kl.Lock(lockKey)
			defer kl.Unlock(lockKey)
			lockState[k] = true
		}(key)
	}

	wg.Wait()
	require.Len(t, lockState, len(keys), "Locks for multiple keys did not function independently")
}
