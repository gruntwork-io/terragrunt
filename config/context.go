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

// Package-level cache instances that persist across command invocations
var (
	persistentDependencyJSONOutputCache = cache.NewCache[[]byte](dependencyJSONOutputCacheName)
	persistentDependencyLocksCache      = cache.NewCache[*sync.Mutex](dependencyLocksCacheName)
	persistentSopsCache                 = cache.NewCache[string](sopsCacheName)
	persistentIAMRoleCache              = cache.NewCache[options.IAMRoleOptions](iamRoleCacheName)
)

// GetSopsCache returns the SOPS cache instance from context
func GetSopsCache(ctx context.Context) *cache.Cache[string] {
	return cache.ContextCache[string](ctx, SopsCacheContextKey)
}

// GetIAMRoleCache returns the IAM role cache instance from context
func GetIAMRoleCache(ctx context.Context) *cache.Cache[options.IAMRoleOptions] {
	return cache.ContextCache[options.IAMRoleOptions](ctx, IAMRoleCacheContextKey)
}

// WithConfigValues add to context default values for configuration.
func WithConfigValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, HclCacheContextKey, cache.NewCache[*hclparse.File](hclCacheName))
	ctx = context.WithValue(ctx, TerragruntConfigCacheContextKey, cache.NewCache[*TerragruntConfig](configCacheName))
	ctx = context.WithValue(ctx, RunCmdCacheContextKey, cache.NewCache[string](runCmdCacheName))
	ctx = context.WithValue(ctx, DependencyOutputCacheContextKey, cache.NewCache[*dependencyOutputCache](dependencyOutputCacheName))
	ctx = context.WithValue(ctx, DependencyJSONOutputCacheContextKey, persistentDependencyJSONOutputCache)
	ctx = context.WithValue(ctx, DependencyLocksContextKey, persistentDependencyLocksCache)
	ctx = context.WithValue(ctx, SopsCacheContextKey, persistentSopsCache)
	ctx = context.WithValue(ctx, IAMRoleCacheContextKey, persistentIAMRoleCache)

	return ctx
}
