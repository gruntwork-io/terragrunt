package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/telemetry"
)

// Cache - generic cache implementation
type Cache[V any] struct {
	Name  string
	Cache map[string]V
	Mutex *sync.RWMutex
}

// NewCache - create new cache with generic type V
func NewCache[V any](name string) *Cache[V] {
	return &Cache[V]{
		Name:  name,
		Cache: make(map[string]V),
		Mutex: &sync.RWMutex{},
	}
}

// Get - fetch value from cache by key
func (c *Cache[V]) Get(ctx context.Context, key string) (V, bool) {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()
	keyHash := sha256.Sum256([]byte(key))
	cacheKey := fmt.Sprintf("%x", keyHash)
	value, found := c.Cache[cacheKey]
	telemetry.Count(ctx, fmt.Sprintf("%s_cache_get", c.Name), 1)
	if found {
		telemetry.Count(ctx, fmt.Sprintf("%s_cache_hit", c.Name), 1)
	} else {
		telemetry.Count(ctx, fmt.Sprintf("%s_cache_miss", c.Name), 1)
	}
	return value, found
}

// Put - put value into cache by key
func (c *Cache[V]) Put(ctx context.Context, key string, value V) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	telemetry.Count(ctx, fmt.Sprintf("%s_cache_put", c.Name), 1)
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
	Name  string
	Cache map[string]ExpiringItem[V]
	Mutex *sync.RWMutex
}

// NewExpiringCache - create new cache with generic type V
func NewExpiringCache[V any](name string) *ExpiringCache[V] {
	return &ExpiringCache[V]{
		Name:  name,
		Cache: make(map[string]ExpiringItem[V]),
		Mutex: &sync.RWMutex{},
	}
}

// Get - fetch value from cache by key
func (c *ExpiringCache[V]) Get(ctx context.Context, key string) (V, bool) {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()
	item, found := c.Cache[key]
	telemetry.Count(ctx, fmt.Sprintf("%s_cache_get", c.Name), 1)
	if !found {
		telemetry.Count(ctx, fmt.Sprintf("%s_cache_miss", c.Name), 1)
		return item.Value, false
	}
	if time.Now().After(item.Expiration) {
		telemetry.Count(ctx, fmt.Sprintf("%s_cache_expiry", c.Name), 1)
		delete(c.Cache, key)
		return item.Value, false
	}
	telemetry.Count(ctx, fmt.Sprintf("%s_cache_hit", c.Name), 1)
	return item.Value, true
}

// Put - put value into cache by key
func (c *ExpiringCache[V]) Put(ctx context.Context, key string, value V, expiration time.Time) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	telemetry.Count(ctx, fmt.Sprintf("%s_cache_put", c.Name), 1)
	c.Cache[key] = ExpiringItem[V]{Value: value, Expiration: expiration}
}

// ContextCache returns cache from the context. If the cache is nil, it creates a new instance.
func ContextCache[T any](ctx context.Context, key any) *Cache[T] {
	cacheInstance, ok := ctx.Value(key).(*Cache[T])
	if !ok || cacheInstance == nil {
		cacheInstance = NewCache[T](fmt.Sprintf("%v", key))
	}
	return cacheInstance
}
