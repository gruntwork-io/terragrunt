package config

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/options"
)

type configKey byte

const (
	HclCacheContextKey                  configKey = iota
	TerragruntConfigCacheContextKey     configKey = iota
	RunCmdCacheContextKey               configKey = iota
	DependencyOutputCacheContextKey     configKey = iota
	DependencyJSONOutputCacheContextKey configKey = iota
	DependencyLocksContextKey           configKey = iota
	SopsCacheContextKey                 configKey = iota
	IAMRoleCacheContextKey              configKey = iota

	hclCacheName                  = "hclCache"
	configCacheName               = "configCache"
	runCmdCacheName               = "runCmdCache"
	dependencyOutputCacheName     = "dependencyOutputCache"
	dependencyJSONOutputCacheName = "dependencyJSONOutputCache"
	dependencyLocksCacheName      = "dependencyLocksCache"
	sopsCacheName                 = "sopsCache"
	iamRoleCacheName              = "iamRoleCache"
)

// GetSopsCache returns the SOPS cache instance from context
func GetSopsCache(ctx context.Context) *cache.Cache[string] {
	return cache.ContextCache[string](ctx, SopsCacheContextKey)
}

// GetIAMRoleCache returns the IAM role cache instance from context
func GetIAMRoleCache(ctx context.Context) *cache.Cache[options.IAMRoleOptions] {
	return cache.ContextCache[options.IAMRoleOptions](ctx, IAMRoleCacheContextKey)
}

// GetHclCache returns the HCL file cache instance from context
func GetHclCache(ctx context.Context) *cache.Cache[*hclparse.File] {
	return cache.ContextCache[*hclparse.File](ctx, HclCacheContextKey)
}

// GetTerragruntConfigCache returns the Terragrunt config cache instance from context
func GetTerragruntConfigCache(ctx context.Context) *cache.Cache[*TerragruntConfig] {
	return cache.ContextCache[*TerragruntConfig](ctx, TerragruntConfigCacheContextKey)
}

// GetRunCmdCache returns the run command cache instance from context
func GetRunCmdCache(ctx context.Context) *cache.Cache[string] {
	return cache.ContextCache[string](ctx, RunCmdCacheContextKey)
}

// GetDependencyOutputCache returns the dependency output cache instance from context
func GetDependencyOutputCache(ctx context.Context) *cache.Cache[*dependencyOutputCache] {
	return cache.ContextCache[*dependencyOutputCache](ctx, DependencyOutputCacheContextKey)
}

// GetDependencyJSONOutputCache returns the dependency JSON output cache instance from context
func GetDependencyJSONOutputCache(ctx context.Context) *cache.Cache[[]byte] {
	return cache.ContextCache[[]byte](ctx, DependencyJSONOutputCacheContextKey)
}

// GetDependencyLocksCache returns the dependency locks cache instance from context
func GetDependencyLocksCache(ctx context.Context) *cache.Cache[*sync.Mutex] {
	return cache.ContextCache[*sync.Mutex](ctx, DependencyLocksContextKey)
}

// WithConfigValues add to context default values for configuration.
// If caches already exist in the context, they are reused. Otherwise, new ones are created.
func WithConfigValues(ctx context.Context) context.Context {
	// Reuse existing caches if they exist, otherwise create new ones
	if _, ok := ctx.Value(HclCacheContextKey).(*cache.Cache[*hclparse.File]); !ok {
		ctx = context.WithValue(ctx, HclCacheContextKey, cache.NewCache[*hclparse.File](hclCacheName))
	}
	if _, ok := ctx.Value(TerragruntConfigCacheContextKey).(*cache.Cache[*TerragruntConfig]); !ok {
		ctx = context.WithValue(ctx, TerragruntConfigCacheContextKey, cache.NewCache[*TerragruntConfig](configCacheName))
	}
	if _, ok := ctx.Value(RunCmdCacheContextKey).(*cache.Cache[string]); !ok {
		ctx = context.WithValue(ctx, RunCmdCacheContextKey, cache.NewCache[string](runCmdCacheName))
	}
	if _, ok := ctx.Value(DependencyOutputCacheContextKey).(*cache.Cache[*dependencyOutputCache]); !ok {
		ctx = context.WithValue(ctx, DependencyOutputCacheContextKey, cache.NewCache[*dependencyOutputCache](dependencyOutputCacheName))
	}
	if _, ok := ctx.Value(DependencyJSONOutputCacheContextKey).(*cache.Cache[[]byte]); !ok {
		ctx = context.WithValue(ctx, DependencyJSONOutputCacheContextKey, cache.NewCache[[]byte](dependencyJSONOutputCacheName))
	}
	if _, ok := ctx.Value(DependencyLocksContextKey).(*cache.Cache[*sync.Mutex]); !ok {
		ctx = context.WithValue(ctx, DependencyLocksContextKey, cache.NewCache[*sync.Mutex](dependencyLocksCacheName))
	}
	if _, ok := ctx.Value(SopsCacheContextKey).(*cache.Cache[string]); !ok {
		ctx = context.WithValue(ctx, SopsCacheContextKey, cache.NewCache[string](sopsCacheName))
	}
	if _, ok := ctx.Value(IAMRoleCacheContextKey).(*cache.Cache[options.IAMRoleOptions]); !ok {
		ctx = context.WithValue(ctx, IAMRoleCacheContextKey, cache.NewCache[options.IAMRoleOptions](iamRoleCacheName))
	}

	return ctx
}
