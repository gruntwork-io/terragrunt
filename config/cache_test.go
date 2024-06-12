package config

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/stretchr/testify/assert"
)

func TestTerragruntConfigCacheCreation(t *testing.T) {
	t.Parallel()

	cache := cache.NewCache[TerragruntConfig]()

	assert.NotNil(t, cache.Mutex)
	assert.NotNil(t, cache.Cache)

	assert.Equal(t, 0, len(cache.Cache))
}

func TestTerragruntConfigCacheOperation(t *testing.T) {
	t.Parallel()

	testCacheKey := "super-safe-cache-key"

	cache := cache.NewCache[TerragruntConfig]()

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
