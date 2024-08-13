package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCacheCreation(t *testing.T) {
	t.Parallel()

	cache := NewCache[string]("test")

	require.NotNil(t, cache.Mutex)
	require.NotNil(t, cache.Cache)

	require.Empty(t, cache.Cache)
}

func TestStringCacheOperation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := NewCache[string]("test")

	value, found := cache.Get(ctx, "potato")

	require.False(t, found)
	require.Empty(t, value)

	cache.Put(ctx, "potato", "carrot")
	value, found = cache.Get(ctx, "potato")

	require.True(t, found)
	require.NotEmpty(t, value)
	require.Equal(t, "carrot", value)
}

func TestExpiringCacheCreation(t *testing.T) {
	t.Parallel()

	cache := NewExpiringCache[string]("test")

	require.NotNil(t, cache.Mutex)
	require.NotNil(t, cache.Cache)

	require.Empty(t, cache.Cache)
}

func TestExpiringCacheOperation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := NewExpiringCache[string]("test")

	value, found := cache.Get(ctx, "potato")

	require.False(t, found)
	require.Empty(t, value)

	cache.Put(ctx, "potato", "carrot", time.Now().Add(1*time.Second))
	value, found = cache.Get(ctx, "potato")

	require.True(t, found)
	require.NotEmpty(t, value)
	require.Equal(t, "carrot", value)
}

func TestExpiringCacheExpiration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := NewExpiringCache[string]("test")

	cache.Put(ctx, "potato", "carrot", time.Now().Add(-1*time.Second))
	value, found := cache.Get(ctx, "potato")

	require.False(t, found)
	require.NotEmpty(t, value)
	require.Equal(t, "carrot", value)
}
