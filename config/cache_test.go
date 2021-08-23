package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCacheCreation(t *testing.T) {
	t.Parallel()

	cache := NewStringCache()

	assert.NotNil(t, cache.Mutex)
	assert.NotNil(t, cache.Cache)

	assert.Equal(t, 0, len(cache.Cache))
}

func TestCacheOperation(t *testing.T) {
	t.Parallel()

	cache := NewStringCache()

	value, found := cache.Get("potato")

	assert.False(t, found)
	assert.Empty(t, value)

	cache.Put("potato", "carrot")
	value, found = cache.Get("potato")

	assert.True(t, found)
	assert.NotEmpty(t, value)
	assert.Equal(t, "carrot", value)
}
