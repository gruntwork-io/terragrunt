package config

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/stretchr/testify/assert"
)

func TestTerragruntConfigCacheCreation(t *testing.T) {
	t.Parallel()

	cache := cache.NewCache[TerragruntConfig]("TerragruntConfig")

	assert.NotNil(t, cache.Mutex)
	assert.NotNil(t, cache.Cache)

	assert.Equal(t, 0, len(cache.Cache))
}

func TestTerragruntConfigCacheOperation(t *testing.T) {
	t.Parallel()

	testCacheKey := "super-safe-cache-key"

	ctx := context.Background()
	cache := cache.NewCache[TerragruntConfig]("TerragruntConfigCacheName")

	actualResult, found := cache.Get(ctx, testCacheKey)

	assert.False(t, found)
	assert.Empty(t, actualResult)

	stubTerragruntConfig := TerragruntConfig{
		IsPartial: true, // Any random property will be sufficient
	}

	cache.Put(ctx, testCacheKey, stubTerragruntConfig)
	actualResult, found = cache.Get(ctx, testCacheKey)

	assert.True(t, found)
	assert.NotEmpty(t, actualResult)
	assert.Equal(t, stubTerragruntConfig, actualResult)
}
