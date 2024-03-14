package config

import (
	"crypto/sha256"
	"fmt"
	"sync"
)

// Cache - generic cache implementation
type Cache[V any] struct {
	Cache map[string]V
	Mutex *sync.Mutex
}

// NewCache - create new cache with generic type V
func NewCache[V any]() *Cache[V] {
	return &Cache[V]{
		Cache: make(map[string]V),
		Mutex: &sync.Mutex{},
	}
}

// Get - fetch value from cache by key
func (c *Cache[V]) Get(key string) (V, bool) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	keyHash := sha256.Sum256([]byte(key))
	cacheKey := fmt.Sprintf("%x", keyHash)
	value, found := c.Cache[cacheKey]
	return value, found
}

// Put - put value into cache by key
func (c *Cache[V]) Put(key string, value V) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	keyHash := sha256.Sum256([]byte(key))
	cacheKey := fmt.Sprintf("%x", keyHash)
	c.Cache[cacheKey] = value
}
