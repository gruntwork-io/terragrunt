package config

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/cache"
)

type configKey byte

const (
	HclCacheContextKey                  configKey = iota
	TerragruntConfigCacheContextKey     configKey = iota
	RunCmdCacheContextKey               configKey = iota
	DependencyOutputCacheContextKey     configKey = iota
	DependencyJSONOutputCacheContextKey configKey = iota
	DependencyLocksContextKey           configKey = iota

	hclCacheName                  = "hclCache"
	configCacheName               = "configCache"
	runCmdCacheName               = "runCmdCache"
	dependencyOutputCacheName     = "dependencyOutputCache"
	dependencyJSONOutputCacheName = "dependencyJSONOutputCache"
	dependencyLocksCacheName      = "dependencyLocksCache"
)

// Global cache references for testing purposes
// These allow ClearOutputCache to work without needing the context
var (
	globalDependencyJSONOutputCache *cache.Cache[[]byte]
	globalDependencyLocksCache      *cache.Cache[*sync.Mutex]
	globalCacheMutex                sync.RWMutex
)

// WithConfigValues add to context default values for configuration.
func WithConfigValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, HclCacheContextKey, cache.NewCache[*hclparse.File](hclCacheName))
	ctx = context.WithValue(ctx, TerragruntConfigCacheContextKey, cache.NewCache[*TerragruntConfig](configCacheName))
	ctx = context.WithValue(ctx, RunCmdCacheContextKey, cache.NewCache[string](runCmdCacheName))
	ctx = context.WithValue(ctx, DependencyOutputCacheContextKey, cache.NewCache[*dependencyOutputCache](dependencyOutputCacheName))

	// Create and store caches that need to be clearable from tests
	jsonCache := cache.NewCache[[]byte](dependencyJSONOutputCacheName)
	locksCache := cache.NewCache[*sync.Mutex](dependencyLocksCacheName)

	ctx = context.WithValue(ctx, DependencyJSONOutputCacheContextKey, jsonCache)
	ctx = context.WithValue(ctx, DependencyLocksContextKey, locksCache)

	// Store global references for testing
	globalCacheMutex.Lock()

	globalDependencyJSONOutputCache = jsonCache
	globalDependencyLocksCache = locksCache

	globalCacheMutex.Unlock()

	return ctx
}
