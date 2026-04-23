package util

import (
	"sync"
)

// KeyLocks manages a map of locks, each associated with a string key.
// Lock entries are reference-counted: a key's entry is removed from the
// internal map when the last holder releases it, so long-running callers
// that lock many distinct keys do not leak memory.
type KeyLocks struct {
	locks      map[string]*keyLockEntry
	masterLock sync.Mutex
}

// keyLockEntry is one reference-counted lock inside KeyLocks. refs counts
// both current holders (while Lock is active) and goroutines waiting to
// acquire; the entry is removed from the map only when refs drops to zero
// on Unlock.
type keyLockEntry struct {
	mu   sync.Mutex
	refs int
}

// NewKeyLocks creates a new instance of KeyLocks.
func NewKeyLocks() *KeyLocks {
	return &KeyLocks{
		locks: make(map[string]*keyLockEntry),
	}
}

// acquireEntry returns the lock entry for key, creating it if needed, and
// bumps its refcount so Unlock knows whether the entry can be deleted.
func (kl *KeyLocks) acquireEntry(key string) *keyLockEntry {
	kl.masterLock.Lock()
	defer kl.masterLock.Unlock()

	entry, ok := kl.locks[key]
	if !ok {
		entry = &keyLockEntry{}
		kl.locks[key] = entry
	}

	entry.refs++

	return entry
}

// Lock acquires the lock for the given key, blocking indefinitely if
// another goroutine holds it.
//
// NOTE: Lock does not consult any context.Context. Callers whose ctx
// cancellation must abort a wait should serialize access themselves, or
// use a ctx-aware lock primitive. Adding a LockCtx(ctx, key) variant is
// the recommended follow-up when a caller surfaces that need.
func (kl *KeyLocks) Lock(key string) {
	entry := kl.acquireEntry(key)
	entry.mu.Lock()
}

// Unlock releases the lock for the given key and decrements its reference
// count. When the count reaches zero, the entry is removed from the
// internal map so keys that are never seen again do not accumulate.
func (kl *KeyLocks) Unlock(key string) {
	kl.masterLock.Lock()

	entry, ok := kl.locks[key]
	if !ok {
		kl.masterLock.Unlock()
		return
	}

	entry.refs--
	if entry.refs == 0 {
		delete(kl.locks, key)
	}

	kl.masterLock.Unlock()

	entry.mu.Unlock()
}

// Len returns the number of lock entries currently tracked. Zero after all
// callers have Unlock'd all keys. Useful for tests that verify the
// reference-counted cleanup actually runs.
func (kl *KeyLocks) Len() int {
	kl.masterLock.Lock()
	defer kl.masterLock.Unlock()

	return len(kl.locks)
}
