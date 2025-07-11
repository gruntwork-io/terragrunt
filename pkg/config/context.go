package config

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/zclconf/go-cty/cty"
)

type configKey byte

const (
	// HclCacheContextKey is the context key for the HCL cache.
	HclCacheContextKey configKey = iota
	// ReadTerragruntConfigCacheContextKey is the context key for the read terragrunt config cache.
	ReadTerragruntConfigCacheContextKey configKey = iota
	// TerragruntConfigCacheContextKey is the context key for the terragrunt config cache.
	TerragruntConfigCacheContextKey configKey = iota
	RunCmdCacheContextKey           configKey = iota
	DependencyOutputCacheContextKey configKey = iota

	hclCacheName                  = "hclCache"
	readTerragruntConfigCacheName = "readTerragruntConfigCache"
	configCacheName               = "configCache"
	runCmdCacheName               = "runCmdCache"
	dependencyOutputCacheName     = "dependencyOutputCache"
)

// WithConfigValues add to context default values for configuration.
func WithConfigValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, HclCacheContextKey, cache.NewCache[*hclparse.File](hclCacheName))
	ctx = context.WithValue(ctx, ReadTerragruntConfigCacheContextKey, cache.NewCache[cty.Value](readTerragruntConfigCacheName))
	ctx = context.WithValue(ctx, TerragruntConfigCacheContextKey, cache.NewCache[*TerragruntConfig](configCacheName))
	ctx = context.WithValue(ctx, RunCmdCacheContextKey, cache.NewCache[string](runCmdCacheName))
	ctx = context.WithValue(ctx, DependencyOutputCacheContextKey, cache.NewCache[*dependencyOutputCache](dependencyOutputCacheName))

	return ctx
}
