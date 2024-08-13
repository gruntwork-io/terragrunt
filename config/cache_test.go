package config

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/stretchr/testify/require"
)

const testCacheName = "TerragruntConfig"

func TestTerragruntConfigCacheCreation(t *testing.T) {
	t.Parallel()

	cache := cache.NewCache[TerragruntConfig](testCacheName)

	require.NotNil(t, cache.Mutex)
	require.NotNil(t, cache.Cache)

	require.Empty(t, cache.Cache)
}

func TestTerragruntConfigCacheOperation(t *testing.T) {
	t.Parallel()

	testCacheKey := "super-safe-cache-key"

	ctx := context.Background()
	cache := cache.NewCache[TerragruntConfig](testCacheName)

	actualResult, found := cache.Get(ctx, testCacheKey)

	require.False(t, found)
	require.Empty(t, actualResult)

	stubTerragruntConfig := TerragruntConfig{
		IsPartial: true, // Any random property will be sufficient
	}

	cache.Put(ctx, testCacheKey, stubTerragruntConfig)
	actualResult, found = cache.Get(ctx, testCacheKey)

	require.True(t, found)
	require.NotEmpty(t, actualResult)
	require.Equal(t, stubTerragruntConfig, actualResult)
}
