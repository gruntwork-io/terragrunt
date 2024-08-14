package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/stretchr/testify/assert"
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

	ctx := context.Background()
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

	ctx := context.Background()
	cache := cache.NewExpiringCache[string]("test")

	value, found := cache.Get(ctx, "potato")

	assert.False(t, found)
	assert.Empty(t, value)

	cache.Put(ctx, "potato", "carrot", time.Now().Add(1*time.Second))
	value, found = cache.Get(ctx, "potato")

	assert.True(t, found)
	assert.NotEmpty(t, value)
	assert.Equal(t, "carrot", value)
}

func TestExpiringCacheExpiration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := cache.NewExpiringCache[string]("test")

	cache.Put(ctx, "potato", "carrot", time.Now().Add(-1*time.Second))
	value, found := cache.Get(ctx, "potato")

	assert.False(t, found)
	assert.NotEmpty(t, value)
	assert.Equal(t, "carrot", value)
}
