package util

import (
	"sync"
)

type KeyLocks struct {
	locks sync.Map // Map to store locks by key
}

// NewKeyLocks - Create a new KeyLocks instance
func NewKeyLocks() *KeyLocks {
	return &KeyLocks{}
}

// Lock - Lock execution by key (blocking)
func (kl *KeyLocks) Lock(key string) {
	for {
		val, _ := kl.locks.LoadOrStore(key, &sync.Mutex{})
		mutex := val.(*sync.Mutex)

		// If we can successfully lock, break the loop
		if mutex.TryLock() {
			return
		}
	}
}

// Unlock - Unlock execution by key
func (kl *KeyLocks) Unlock(key string) {
	val, ok := kl.locks.Load(key)
	if ok {
		mutex := val.(*sync.Mutex)

		// Only unlock if the current goroutine holds the lock
		if mutex.TryLock() {
			mutex.Unlock()
			mutex.Unlock() // Unlock again since we locked it with TryLock
		}
	}
}
