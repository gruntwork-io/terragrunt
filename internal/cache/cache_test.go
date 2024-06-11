package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheCreation(t *testing.T) {
	t.Parallel()

	cache := NewCache[string]()

	assert.NotNil(t, cache.Mutex)
	assert.NotNil(t, cache.Cache)

	assert.Equal(t, 0, len(cache.Cache))
}

func TestStringCacheOperation(t *testing.T) {
	t.Parallel()

	cache := NewCache[string]()

	value, found := cache.Get("potato")

	assert.False(t, found)
	assert.Empty(t, value)

	cache.Put("potato", "carrot")
	value, found = cache.Get("potato")

	assert.True(t, found)
	assert.NotEmpty(t, value)
	assert.Equal(t, "carrot", value)
}

func TestExpiringCacheCreation(t *testing.T) {
	t.Parallel()

	cache := NewExpiringCache[string]()

	assert.NotNil(t, cache.Mutex)
	assert.NotNil(t, cache.Cache)

	assert.Equal(t, 0, len(cache.Cache))
}

func TestExpiringCacheOperation(t *testing.T) {
	t.Parallel()

	cache := NewExpiringCache[string]()

	value, found := cache.Get("potato")

	assert.False(t, found)
	assert.Empty(t, value)

	cache.Put("potato", "carrot", time.Now().Add(1*time.Second))
	value, found = cache.Get("potato")

	assert.True(t, found)
	assert.NotEmpty(t, value)
	assert.Equal(t, "carrot", value)
}

func TestExpiringCacheExpiration(t *testing.T) {
	t.Parallel()

	cache := NewExpiringCache[string]()

	cache.Put("potato", "carrot", time.Now().Add(-1*time.Second))
	value, found := cache.Get("potato")

	assert.False(t, found)
	assert.NotEmpty(t, value)
	assert.Equal(t, "carrot", value)
}
