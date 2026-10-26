package util_test

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

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

// TestKeyLocksSharedKeySerializes asserts concurrent holders of the same key serialise without lost updates.
func TestKeyLocksSharedKeySerializes(t *testing.T) {
	t.Parallel()

	kl := util.NewKeyLocks()

	var (
		counter int
		wg      sync.WaitGroup
	)

	for range 10 {
		wg.Go(func() {
			kl.Lock("test-key")
			defer kl.Unlock("test-key")

			counter++
			counter++
		})
	}

	wg.Wait()

	require.Equal(
		t,
		20,
		counter,
		"serialized increments must total 20 (other totals indicate a lost update)",
	)
}

// TestKeyLocksIndependentKeysDoNotBlock asserts that distinct keys do not block each other.
func TestKeyLocksIndependentKeysDoNotBlock(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		kl := util.NewKeyLocks()
		kl.Lock("a")
		defer kl.Unlock("a")

		done := make(chan struct{})

		go func() {
			kl.Lock("b")
			kl.Unlock("b")
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			require.FailNow(t, `locking independent key "b" was blocked by holder of "a"`)
		}
	})
}

// TestKeyLocksUnlockWithoutLock checks for safe behavior when unlocking without locking.
func TestKeyLocksUnlockWithoutLock(t *testing.T) {
	t.Parallel()

	kl := util.NewKeyLocks()

	require.NotPanics(t, func() {
		kl.Unlock("nonexistent_key")
	}, "Unlocking without locking should not panic")
}

// TestKeyLocksLockContextStopsWaitingWhenContextEnds pins that a caller
// waiting on a held key returns its context's error once the context ends,
// and that the key stays with its holder.
func TestKeyLocksLockContextStopsWaitingWhenContextEnds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		kl := util.NewKeyLocks()
		kl.Lock("key1")

		ctx, cancel := context.WithCancel(t.Context())

		errCh := make(chan error, 1)

		go func() {
			errCh <- kl.LockContext(ctx, "key1")
		}()

		synctest.Wait()
		cancel()

		require.ErrorIs(t, <-errCh, context.Canceled)
		require.NotPanics(t, func() { kl.Unlock("key1") })
	})
}

// TestKeyLocksLockContextEndedContext pins that an ended context returns its
// error even when the key is free.
func TestKeyLocksLockContextEndedContext(t *testing.T) {
	t.Parallel()

	kl := util.NewKeyLocks()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorIs(t, kl.LockContext(ctx, "key1"), context.Canceled)
	require.NoError(t, kl.LockContext(t.Context(), "key1"))
}

// TestKeyLocksUnlockOfUnlockedKeyPanics pins that unlocking a key twice
// panics, as unlocking a sync.Mutex twice does.
func TestKeyLocksUnlockOfUnlockedKeyPanics(t *testing.T) {
	t.Parallel()

	kl := util.NewKeyLocks()

	kl.Lock("key1")
	kl.Unlock("key1")

	require.Panics(t, func() {
		kl.Unlock("key1")
	})
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
		wg.Go(func() {
			kl.Lock("shared_key")
			defer kl.Unlock("shared_key")

			for range numOperations {
				counter++
				counter++
			}
		})
	}

	wg.Wait()

	require.Equal(
		t,
		numGoroutines*numOperations*2,
		counter,
		"All lock/unlock cycles should be completed",
	)
}
