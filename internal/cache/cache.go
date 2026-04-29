// Package cache provides generic cache.
// It is used to store values by key and retrieve them later.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strings"
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

// RepoRootCache stores git repository roots and answers prefix-containment
// queries. Roots are kept sorted by descending length so Lookup yields the
// deepest matching root for paths inside nested repositories.
type RepoRootCache struct {
	name    string
	roots   []string
	mu      sync.RWMutex
	resolve sync.Mutex
}

// NewRepoRootCache constructs an empty RepoRootCache. The name is used as a
// prefix for telemetry counters.
func NewRepoRootCache(name string) *RepoRootCache {
	return &RepoRootCache{name: name}
}

// Lookup returns the deepest cached root that contains path. Containment is
// component-aware: `/foo` does not match `/foobar`.
func (c *RepoRootCache) Lookup(ctx context.Context, path string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	telemetry.TelemeterFromContext(ctx).Count(ctx, c.name+"_cache_get", 1)

	for _, root := range c.roots {
		if pathContainedIn(path, root) {
			telemetry.TelemeterFromContext(ctx).Count(ctx, c.name+"_cache_hit", 1)

			return root, true
		}
	}

	telemetry.TelemeterFromContext(ctx).Count(ctx, c.name+"_cache_miss", 1)

	return "", false
}

// Add records root as a known repository root. Inserts are ordered by
// descending length so Lookup's first match is the deepest. Duplicates are
// ignored.
func (c *RepoRootCache) Add(ctx context.Context, root string) {
	if root == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	telemetry.TelemeterFromContext(ctx).Count(ctx, c.name+"_cache_put", 1)

	insertAt := len(c.roots)

	for i, existing := range c.roots {
		if existing == root {
			return
		}

		if insertAt == len(c.roots) && len(root) > len(existing) {
			insertAt = i
		}
	}

	c.roots = slices.Insert(c.roots, insertAt, root)
}

// BeginResolve serializes callers that are about to perform the external
// lookup whose result they will Add. Callers must Lookup again after
// BeginResolve returns so a concurrent populate is observed before running
// the external resolver. EndResolve releases the lock.
//
// The lock is independent of the cache's read/write mutex; it is held across
// the external call so concurrent misses collapse to a single resolution.
func (c *RepoRootCache) BeginResolve() {
	c.resolve.Lock()
}

// EndResolve releases the lock taken by BeginResolve.
func (c *RepoRootCache) EndResolve() {
	c.resolve.Unlock()
}

// Len returns the number of cached roots.
func (c *RepoRootCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.roots)
}

// pathContainedIn reports whether path equals root or sits beneath it,
// requiring a separator after root so `/foo` does not match `/foobar`.
func pathContainedIn(path, root string) bool {
	if path == root {
		return true
	}

	if !strings.HasPrefix(path, root) {
		return false
	}

	rest := path[len(root):]

	return len(rest) > 0 && os.IsPathSeparator(rest[0])
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
