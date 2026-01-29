package config

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

type configKey byte

const (
	HclCacheContextKey              configKey = iota
	TerragruntConfigCacheContextKey configKey = iota
	RunCmdCacheContextKey           configKey = iota
	DependencyOutputCacheContextKey configKey = iota
	JSONOutputCacheContextKey       configKey = iota
	OutputLocksContextKey           configKey = iota

	hclCacheName              = "hclCache"
	configCacheName           = "configCache"
	runCmdCacheName           = "runCmdCache"
	dependencyOutputCacheName = "dependencyOutputCache"
	jsonOutputCacheName       = "jsonOutputCache"
)

// WithConfigValues add to context default values for configuration.
func WithConfigValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, HclCacheContextKey, cache.NewCache[*hclparse.File](hclCacheName))
	ctx = context.WithValue(ctx, TerragruntConfigCacheContextKey, cache.NewCache[*TerragruntConfig](configCacheName))
	ctx = context.WithValue(ctx, RunCmdCacheContextKey, cache.NewCache[string](runCmdCacheName))
	depOutputCache := cache.NewCache[*dependencyOutputCache](dependencyOutputCacheName)
	ctx = context.WithValue(ctx, DependencyOutputCacheContextKey, depOutputCache)
	ctx = context.WithValue(ctx, JSONOutputCacheContextKey, cache.NewCache[[]byte](jsonOutputCacheName))
	ctx = context.WithValue(ctx, OutputLocksContextKey, util.NewKeyLocks())

	return ctx
}
