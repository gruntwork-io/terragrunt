package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerragruntConfigCacheCreation(t *testing.T) {
	t.Parallel()

	cache := NewTerragruntConfigCache()

	assert.NotNil(t, cache.Mutex)
	assert.NotNil(t, cache.Cache)

	assert.Equal(t, 0, len(cache.Cache))
}

func TestTerragruntConfigCacheOperation(t *testing.T) {
	t.Parallel()

	testCacheKey := "super-safe-cache-key"

	cache := NewTerragruntConfigCache()

	actualResult, found := cache.Get(testCacheKey)

	assert.False(t, found)
	assert.Empty(t, actualResult)

	stubTerragruntConfig := TerragruntConfig{
		IsPartial: true, // Any random property will be sufficient
	}

	cache.Put(testCacheKey, stubTerragruntConfig)
	actualResult, found = cache.Get(testCacheKey)

	assert.True(t, found)
	assert.NotEmpty(t, actualResult)
	assert.Equal(t, stubTerragruntConfig, actualResult)
}
