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
	kl.masterLock.Lock()
	lock, ok := kl.locks[key]
	if !ok {
		lock = &sync.Mutex{}
		kl.locks[key] = lock
	}
	kl.masterLock.Unlock()
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
