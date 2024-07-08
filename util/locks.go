package util

import (
	"sync"
)

// KeyLocks manages a map of locks, each associated with a string key.
type KeyLocks struct {
	masterLock sync.Mutex
	locks      map[string]*sync.Mutex
}

// NewKeyLocks creates a new instance of KeyLocks.
func NewKeyLocks() *KeyLocks {
	return &KeyLocks{
		locks: make(map[string]*sync.Mutex),
	}
}

// getOrCreateLock retrieves the lock for the given key, creating it if it doesn't exist.
func (kl *KeyLocks) getOrCreateLock(key string) *sync.Mutex {
	kl.masterLock.Lock()
	defer kl.masterLock.Unlock()

	lock, ok := kl.locks[key]
	if !ok {
		lock = &sync.Mutex{}
		kl.locks[key] = lock
	}
	return lock
}

// Lock acquires the lock for the given key.
func (kl *KeyLocks) Lock(key string) {
	lock := kl.getOrCreateLock(key)
	lock.Lock()
}

// Unlock releases the lock for the given key.
func (kl *KeyLocks) Unlock(key string) {
	kl.masterLock.Lock()
	defer kl.masterLock.Unlock()

	if lock, ok := kl.locks[key]; ok {
		lock.Unlock()
	}
}
