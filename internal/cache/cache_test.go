package cache_test

import (
	"context"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheCreation(t *testing.T) {
	t.Parallel()

	cache := cache.NewCache[string]("test")

	assert.NotNil(t, cache.Mutex)
	assert.NotNil(t, cache.Cache)

	assert.Empty(t, cache.Cache)
}

func TestStringCacheOperation(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	cache := cache.NewCache[string]("test")

	value, found := cache.Get(ctx, "potato")

	assert.False(t, found)
	assert.Empty(t, value)

	cache.Put(ctx, "potato", "carrot")
	value, found = cache.Get(ctx, "potato")

	assert.True(t, found)
	assert.NotEmpty(t, value)
	assert.Equal(t, "carrot", value)
}

func TestExpiringCacheCreation(t *testing.T) {
	t.Parallel()

	cache := cache.NewExpiringCache[string]("test")

	assert.NotNil(t, cache.Mutex)
	assert.NotNil(t, cache.Cache)

	assert.Empty(t, cache.Cache)
}

func TestExpiringCacheOperation(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		cache := cache.NewExpiringCache[string]("test")

		value, found := cache.Get(ctx, "potato")

		assert.False(t, found)
		assert.Empty(t, value)

		cache.Put(ctx, "potato", "carrot", time.Now().Add(1*time.Second))
		value, found = cache.Get(ctx, "potato")

		assert.True(t, found)
		assert.NotEmpty(t, value)
		assert.Equal(t, "carrot", value)
	})
}

func TestExpiringCacheExpiration(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		cache := cache.NewExpiringCache[string]("test")

		cache.Put(ctx, "potato", "carrot", time.Now().Add(time.Second))

		// Move the bubble's virtual clock past the expiration so the Get below
		// observes the entry as expired without any real wallclock wait.
		time.Sleep(2 * time.Second)

		value, found := cache.Get(ctx, "potato")

		assert.False(t, found)
		assert.NotEmpty(t, value)
		assert.Equal(t, "carrot", value)
	})
}

func TestContextCache(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// Missing entry returns a fresh detached instance.
	c := cache.ContextCache[int](ctx, "not-installed")
	require.NotNil(t, c)
	c.Put(ctx, "k", 7)

	// Installed entry round-trips.
	installed := cache.NewCache[int]("installed")
	ctxWith := context.WithValue(ctx, cache.RunCmdCacheContextKey, installed)

	got := cache.ContextCache[int](ctxWith, cache.RunCmdCacheContextKey)
	assert.Same(t, installed, got)
}

func TestContextWithCacheInstallsBoth(t *testing.T) {
	t.Parallel()

	ctx := cache.ContextWithCache(t.Context())

	runCmd, ok := ctx.Value(cache.RunCmdCacheContextKey).(*cache.Cache[string])
	require.True(t, ok)
	require.NotNil(t, runCmd)

	repoRoots, ok := ctx.Value(cache.RepoRootCacheContextKey).(*cache.RepoRootCache)
	require.True(t, ok)
	require.NotNil(t, repoRoots)
	assert.Equal(t, 0, repoRoots.Len())
}

func TestRepoRootCacheLookupAndAdd(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := cache.NewRepoRootCache("repo")

	// Empty cache misses.
	_, ok := c.Lookup(ctx, filepath.FromSlash("/a/b"))
	assert.False(t, ok)

	outer := filepath.FromSlash("/repo")
	inner := filepath.FromSlash("/repo/sub/nested")

	// Add deepest-first, then a shallower root, to exercise insertion ordering.
	c.Add(ctx, outer)
	c.Add(ctx, inner)
	// Duplicate Add is a no-op.
	c.Add(ctx, outer)
	// Empty Add is a no-op.
	c.Add(ctx, "")
	assert.Equal(t, 2, c.Len())

	// Adding a shallower-still root exercises the "no insertAt found" branch
	// where the new root is shorter than every existing one and is appended.
	shallow := filepath.FromSlash("/r")
	c.Add(ctx, shallow)
	assert.Equal(t, 3, c.Len())

	// Exact-path hit returns the matching root.
	got, ok := c.Lookup(ctx, outer)
	assert.True(t, ok)
	assert.Equal(t, outer, got)

	// Descendant of the deeper root prefers it over the shallow one.
	got, ok = c.Lookup(ctx, filepath.Join(inner, "x"))
	assert.True(t, ok)
	assert.Equal(t, inner, got)

	// Sibling that prefix-matches lexically but not on a separator boundary
	// is not a hit (e.g. /repobar should not match /repo).
	_, ok = c.Lookup(ctx, filepath.FromSlash("/repobar/x"))
	assert.False(t, ok)

	// Path outside any cached root misses entirely.
	_, ok = c.Lookup(ctx, filepath.FromSlash("/elsewhere"))
	assert.False(t, ok)
}

func TestRepoRootCacheBeginEndResolve(t *testing.T) {
	t.Parallel()

	c := cache.NewRepoRootCache("repo")

	// Round-trip the lock twice to confirm BeginResolve/EndResolve pair up
	// (a missing Unlock would deadlock the second BeginResolve). The lock's
	// mutual-exclusion semantics are the stdlib's responsibility, not this
	// test's.
	c.BeginResolve()
	c.EndResolve()

	c.BeginResolve()
	c.EndResolve()
}

func TestContextRepoRootCache(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// Missing key returns a fresh detached instance, never nil.
	c := cache.ContextRepoRootCache(ctx, "missing")
	require.NotNil(t, c)
	assert.Equal(t, 0, c.Len())

	installed := cache.NewRepoRootCache("installed")
	ctxWith := context.WithValue(ctx, cache.RepoRootCacheContextKey, installed)

	got := cache.ContextRepoRootCache(ctxWith, cache.RepoRootCacheContextKey)
	assert.Same(t, installed, got)
}
