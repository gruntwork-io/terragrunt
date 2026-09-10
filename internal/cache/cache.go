// Package cache provides generic cache.
// It is used to store values by key and retrieve them later.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
)

// Cache - generic cache implementation
type Cache[V any] struct {
	Cache map[string]V
	Mutex *sync.RWMutex
	Name  string
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
	cacheKey := hex.EncodeToString(keyHash[:])
	value, found := c.Cache[cacheKey]

	telemetry.TelemeterFromContext(ctx).Count(ctx, c.Name+"_cache_get", 1)

	if found {
		telemetry.TelemeterFromContext(ctx).Count(ctx, c.Name+"_cache_hit", 1)
	} else {
		telemetry.TelemeterFromContext(ctx).Count(ctx, c.Name+"_cache_miss", 1)
	}

	return value, found
}

// Put - put value into cache by key
func (c *Cache[V]) Put(ctx context.Context, key string, value V) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	telemetry.TelemeterFromContext(ctx).Count(ctx, c.Name+"_cache_put", 1)

	keyHash := sha256.Sum256([]byte(key))
	cacheKey := hex.EncodeToString(keyHash[:])
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
	Mutex *sync.RWMutex
	Name  string
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
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	item, found := c.Cache[key]
	telemetry.TelemeterFromContext(ctx).Count(ctx, c.Name+"_cache_get", 1)

	if !found {
		telemetry.TelemeterFromContext(ctx).Count(ctx, c.Name+"_cache_miss", 1)
		return item.Value, false
	}

	if time.Now().After(item.Expiration) {
		telemetry.TelemeterFromContext(ctx).Count(ctx, c.Name+"_cache_expiry", 1)
		delete(c.Cache, key)

		return item.Value, false
	}

	telemetry.TelemeterFromContext(ctx).Count(ctx, c.Name+"_cache_hit", 1)

	return item.Value, true
}

// Put - put value into cache by key
func (c *ExpiringCache[V]) Put(ctx context.Context, key string, value V, expiration time.Time) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	telemetry.TelemeterFromContext(ctx).Count(ctx, c.Name+"_cache_put", 1)
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

// RepoRootCache memoizes git repository roots by directory.
type RepoRootCache struct {
	roots map[string]string
	name  string
	mu    sync.RWMutex
}

// NewRepoRootCache constructs an empty RepoRootCache. The name is used as a
// prefix for telemetry counters.
func NewRepoRootCache(name string) *RepoRootCache {
	return &RepoRootCache{name: name, roots: make(map[string]string)}
}

// Lookup returns the repository root recorded for dir.
func (c *RepoRootCache) Lookup(ctx context.Context, dir string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	telemetry.TelemeterFromContext(ctx).Count(ctx, c.name+"_cache_get", 1)

	root, ok := c.roots[dir]
	if !ok {
		telemetry.TelemeterFromContext(ctx).Count(ctx, c.name+"_cache_miss", 1)

		return "", false
	}

	telemetry.TelemeterFromContext(ctx).Count(ctx, c.name+"_cache_hit", 1)

	return root, true
}

// Add records root as the repository root of every directory in dirs. The
// caller passes only directories it has established share that root. An empty
// root records nothing.
func (c *RepoRootCache) Add(ctx context.Context, root string, dirs ...string) {
	if root == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, dir := range dirs {
		if dir == "" {
			continue
		}

		telemetry.TelemeterFromContext(ctx).Count(ctx, c.name+"_cache_put", 1)

		c.roots[dir] = root
	}
}

// Len returns the number of memoized directories.
func (c *RepoRootCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.roots)
}

// ContextRepoRootCache returns the RepoRootCache stored on the context, or a
// fresh detached instance if none is present so callers do not need to
// nil-check.
func ContextRepoRootCache(ctx context.Context, key any) *RepoRootCache {
	if c, ok := ctx.Value(key).(*RepoRootCache); ok && c != nil {
		return c
	}

	return NewRepoRootCache(fmt.Sprintf("%v", key))
}
