package config

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/zclconf/go-cty/cty"
)

type configKey byte

const (
	HclCacheContextKey                  configKey = iota
	ReadTerragruntConfigCacheContextKey configKey = iota
	TerragruntConfigCacheContextKey     configKey = iota
	RunCmdCacheContextKey               configKey = iota
	DependencyOutputCacheContextKey     configKey = iota
	JSONOutputCacheContextKey           configKey = iota
	OutputLocksContextKey               configKey = iota
	SopsCacheContextKey                 configKey = iota
	AutoIncludeSuffixCacheContextKey    configKey = iota

	hclCacheName                  = "hclCache"
	readTerragruntConfigCacheName = "readTerragruntConfigCache"
	configCacheName               = "configCache"
	runCmdCacheName               = "runCmdCache"
	dependencyOutputCacheName     = "dependencyOutputCache"
	jsonOutputCacheName           = "jsonOutputCache"
	sopsCacheName                 = "sopsCache"
	autoIncludeSuffixCacheName    = "autoIncludeSuffixCache"
)

// WithConfigValues add to context default values for configuration.
func WithConfigValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, HclCacheContextKey, cache.NewCache[*hclparse.File](hclCacheName))
	ctx = context.WithValue(ctx, ReadTerragruntConfigCacheContextKey, cache.NewCache[cty.Value](readTerragruntConfigCacheName))
	ctx = context.WithValue(ctx, TerragruntConfigCacheContextKey, cache.NewCache[*TerragruntConfig](configCacheName))
	ctx = context.WithValue(ctx, RunCmdCacheContextKey, cache.NewCache[*RunCmdCacheEntry](runCmdCacheName))
	ctx = context.WithValue(ctx, DependencyOutputCacheContextKey, cache.NewCache[*dependencyOutputCache](dependencyOutputCacheName))
	ctx = context.WithValue(ctx, JSONOutputCacheContextKey, cache.NewCache[[]byte](jsonOutputCacheName))
	ctx = context.WithValue(ctx, OutputLocksContextKey, util.NewKeyLocks())
	ctx = context.WithValue(ctx, SopsCacheContextKey, cache.NewCache[string](sopsCacheName))
	ctx = context.WithValue(ctx, AutoIncludeSuffixCacheContextKey, cache.NewCache[string](autoIncludeSuffixCacheName))

	return ctx
}
