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

// Package-level cache instances that persist across command invocations
// These are needed because each command creates a new context but we want caches to persist
var (
	persistentDependencyJSONOutputCache = cache.NewCache[[]byte](dependencyJSONOutputCacheName)
	persistentDependencyLocksCache      = cache.NewCache[*sync.Mutex](dependencyLocksCacheName)
)

// WithConfigValues add to context default values for configuration.
func WithConfigValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, HclCacheContextKey, cache.NewCache[*hclparse.File](hclCacheName))
	ctx = context.WithValue(ctx, TerragruntConfigCacheContextKey, cache.NewCache[*TerragruntConfig](configCacheName))
	ctx = context.WithValue(ctx, RunCmdCacheContextKey, cache.NewCache[string](runCmdCacheName))
	ctx = context.WithValue(ctx, DependencyOutputCacheContextKey, cache.NewCache[*dependencyOutputCache](dependencyOutputCacheName))

	// Use persistent caches for dependency outputs so they survive across command invocations
	ctx = context.WithValue(ctx, DependencyJSONOutputCacheContextKey, persistentDependencyJSONOutputCache)
	ctx = context.WithValue(ctx, DependencyLocksContextKey, persistentDependencyLocksCache)

	return ctx
}
