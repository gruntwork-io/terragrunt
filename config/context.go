package config

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/hashicorp/hcl/v2"
)

type configKey byte

const (
	HclCacheContextKey configKey = iota
	TerragruntConfigCacheContextKey
	RunCmdCacheContextKey
	DependencyOutputCacheContextKey
	EvalCtxCacheContextKey
)

const (
	hclCacheName              = "hclCache"
	configCacheName           = "configCache"
	runCmdCacheName           = "runCmdCache"
	dependencyOutputCacheName = "dependencyOutputCache"
	evalCtxCacheName          = "evalCtxCache"
)

// WithConfigValues add to context default values for configuration.
func WithConfigValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, HclCacheContextKey, cache.NewCache[*hclparse.File](hclCacheName))
	ctx = context.WithValue(ctx, TerragruntConfigCacheContextKey, cache.NewCache[*TerragruntConfig](configCacheName))
	ctx = context.WithValue(ctx, RunCmdCacheContextKey, cache.NewCache[string](runCmdCacheName))
	ctx = context.WithValue(ctx, DependencyOutputCacheContextKey, cache.NewCache[*dependencyOutputCache](dependencyOutputCacheName))
	ctx = context.WithValue(ctx, EvalCtxCacheContextKey, cache.NewCache[*hcl.EvalContext](evalCtxCacheName))

	return ctx
}
