package util

import (
	"context"
	"sync"
)

// KeyLocks manages a map of locks, each associated with a string key.
//
// Each lock is a channel with one slot, so [KeyLocks.LockContext] can stop
// waiting when its context ends, and a goroutine waiting on one inside a
// testing/synctest bubble counts as durably blocked.
type KeyLocks struct {
	locks      map[string]chan struct{}
	masterLock sync.Mutex
}

// NewKeyLocks creates a new instance of KeyLocks.
func NewKeyLocks() *KeyLocks {
	return &KeyLocks{
		locks: make(map[string]chan struct{}),
	}
}

// Lock acquires the lock for the given key.
func (kl *KeyLocks) Lock(key string) {
	kl.getOrCreateLock(key) <- struct{}{}
}

// LockContext acquires the lock for the given key.
//
// Returns ctx's error, without acquiring the lock, when ctx has ended or ends
// before the lock is free.
func (kl *KeyLocks) LockContext(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case kl.getOrCreateLock(key) <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Unlock releases the lock for the given key. It does nothing for a key that
// has never been locked.
//
// Panics when the key has been locked before but is not locked now, as
// [sync.Mutex.Unlock] does.
func (kl *KeyLocks) Unlock(key string) {
	kl.masterLock.Lock()
	lock, ok := kl.locks[key]
	kl.masterLock.Unlock()

	if !ok {
		return
	}

	select {
	case <-lock:
	default:
		panic("util.KeyLocks: unlock of unlocked key " + key)
	}
}

// getOrCreateLock retrieves the lock for the given key, creating it if it doesn't exist.
func (kl *KeyLocks) getOrCreateLock(key string) chan struct{} {
	kl.masterLock.Lock()
	defer kl.masterLock.Unlock()

	lock, ok := kl.locks[key]
	if !ok {
		lock = make(chan struct{}, 1)
		kl.locks[key] = lock
	}

	return lock
}
