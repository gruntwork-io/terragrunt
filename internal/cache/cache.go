package cache

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
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

// ExpiringItem - item with expiration time
type ExpiringItem[V any] struct {
	Value      V
	Expiration time.Time
}

// ExpiringCache - cache with items with expiration time
type ExpiringCache[V any] struct {
	Cache map[string]ExpiringItem[V]
	Mutex *sync.Mutex
}

// NewExpiringCache - create new cache with generic type V
func NewExpiringCache[V any]() *ExpiringCache[V] {
	return &ExpiringCache[V]{
		Cache: make(map[string]ExpiringItem[V]),
		Mutex: &sync.Mutex{},
	}
}

// Get - fetch value from cache by key
func (c *ExpiringCache[V]) Get(key string) (V, bool) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	item, found := c.Cache[key]
	if !found {
		return item.Value, false
	}
	if time.Now().After(item.Expiration) {
		delete(c.Cache, key)
		return item.Value, false
	}
	return item.Value, true
}

// Put - put value into cache by key
func (c *ExpiringCache[V]) Put(key string, value V, expiration time.Time) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	c.Cache[key] = ExpiringItem[V]{Value: value, Expiration: expiration}
}
