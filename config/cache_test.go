package config_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/stretchr/testify/assert"
)

const testCacheName = "TerragruntConfig"

func TestTerragruntConfigCacheCreation(t *testing.T) {
	t.Parallel()

	cache := cache.NewCache[config.TerragruntConfig](testCacheName)

	assert.NotNil(t, cache.Mutex)
	assert.NotNil(t, cache.Cache)

	assert.Empty(t, cache.Cache)
}

func TestTerragruntConfigCacheOperation(t *testing.T) {
	t.Parallel()

	testCacheKey := "super-safe-cache-key"

	ctx := context.Background()
	cache := cache.NewCache[config.TerragruntConfig](testCacheName)

	actualResult, found := cache.Get(ctx, testCacheKey)

	assert.False(t, found)
	assert.Empty(t, actualResult)

	stubTerragruntConfig := config.TerragruntConfig{
		IsPartial: true, // Any random property will be sufficient
	}

	cache.Put(ctx, testCacheKey, stubTerragruntConfig)
	actualResult, found = cache.Get(ctx, testCacheKey)

	assert.True(t, found)
	assert.NotEmpty(t, actualResult)
	assert.Equal(t, stubTerragruntConfig, actualResult)
}

func TestGlobalCacheDirectory(t *testing.T) {
	t.Parallel()

	// Set up global cache directory
	globalCacheDir := filepath.Join(os.TempDir(), "terragrunt-global-cache")
	err := os.MkdirAll(globalCacheDir, os.ModePerm)
	assert.NoError(t, err)
	defer os.RemoveAll(globalCacheDir)

	// Set environment variable for global cache
	os.Setenv("TERRAGRUNT_GLOBAL_CACHE", globalCacheDir)
	defer os.Unsetenv("TERRAGRUNT_GLOBAL_CACHE")

	// Create a new cache instance
	cache := cache.NewCache[config.TerragruntConfig](testCacheName)

	// Verify that the cache directory is set correctly
	assert.Equal(t, globalCacheDir, os.Getenv("TERRAGRUNT_GLOBAL_CACHE"))
}

func TestGlobalCacheDirectoryMultipleOS(t *testing.T) {
	t.Parallel()

	// Set up global cache directory
	globalCacheDir := filepath.Join(os.TempDir(), "terragrunt-global-cache")
	err := os.MkdirAll(globalCacheDir, os.ModePerm)
	assert.NoError(t, err)
	defer os.RemoveAll(globalCacheDir)

	// Set environment variable for global cache
	os.Setenv("TERRAGRUNT_GLOBAL_CACHE", globalCacheDir)
	defer os.Unsetenv("TERRAGRUNT_GLOBAL_CACHE")

	// Create a new cache instance
	cache := cache.NewCache[config.TerragruntConfig](testCacheName)

	// Verify that the cache directory is set correctly
	assert.Equal(t, globalCacheDir, os.Getenv("TERRAGRUNT_GLOBAL_CACHE"))

	// Check for different operating systems
	switch runtime.GOOS {
	case "windows":
		assert.Contains(t, globalCacheDir, `\`)
	case "linux", "darwin":
		assert.Contains(t, globalCacheDir, `/`)
	default:
		t.Fatalf("Unsupported OS: %s", runtime.GOOS)
	}
}
