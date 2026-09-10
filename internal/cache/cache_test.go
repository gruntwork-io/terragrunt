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

	root := filepath.FromSlash("/repo")
	unit := filepath.FromSlash("/repo/live/vpc")

	// Empty cache misses.
	_, ok := c.Lookup(ctx, unit)
	assert.False(t, ok)

	// A walk records the whole chain it proved free of a `.git`.
	c.Add(ctx, root, filepath.FromSlash("/repo/live"), unit)
	assert.Equal(t, 2, c.Len())

	got, ok := c.Lookup(ctx, unit)
	assert.True(t, ok)
	assert.Equal(t, root, got)

	// An empty root records nothing, and an empty directory is skipped.
	c.Add(ctx, "", unit)
	c.Add(ctx, root, "")
	assert.Equal(t, 2, c.Len())

	// Entries are exact: a directory nobody walked is a miss even though a
	// cached root encloses it. That is what keeps a nested repository's units
	// from inheriting the outer root.
	_, ok = c.Lookup(ctx, filepath.FromSlash("/repo/vendor/mod/live/vpc"))
	assert.False(t, ok)

	// Re-recording a directory under a different root overwrites it, so a
	// resolver that learns a nested root is not stuck with a stale answer.
	nested := filepath.FromSlash("/repo/vendor/mod")
	c.Add(ctx, nested, unit)

	got, ok = c.Lookup(ctx, unit)
	assert.True(t, ok)
	assert.Equal(t, nested, got)
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
