package util

import "sync"

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

// Lock acquires the lock for the given key.
func (kl *KeyLocks) Lock(key string) {
	kl.ensureLock(key)
	kl.locks[key].Lock()
}

// Unlock releases the lock for the given key.
func (kl *KeyLocks) Unlock(key string) {
	if lock, ok := kl.locks[key]; ok {
		lock.Unlock()
	}
}

// ensureLock checks if a lock exists for the key, and if not, creates one.
func (kl *KeyLocks) ensureLock(key string) {
	kl.masterLock.Lock()
	defer kl.masterLock.Unlock()

	if _, ok := kl.locks[key]; !ok {
		kl.locks[key] = new(sync.Mutex)
	}
}
