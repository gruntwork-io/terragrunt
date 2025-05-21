package config

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/hashicorp/hcl/v2"
)

type configKey byte

const (
	HclCacheContextKey              configKey = iota
	TerragruntConfigCacheContextKey configKey = iota
	EvalCtxCacheContextKey          configKey = iota
	RunCmdCacheContextKey           configKey = iota
	DependencyOutputCacheContextKey configKey = iota
	OptsCacheContextKey             configKey = iota

	hclCacheName              = "hclCache"
	configCacheName           = "configCache"
	evalCtxCacheName          = "evalCtxCache"
	runCmdCacheName           = "runCmdCache"
	dependencyOutputCacheName = "dependencyOutputCache"
	optsCacheName             = "optsCache"
)

// WithConfigValues add to context default values for configuration.
func WithConfigValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, HclCacheContextKey, cache.NewCache[*hclparse.File](hclCacheName))
	ctx = context.WithValue(ctx, TerragruntConfigCacheContextKey, cache.NewCache[*TerragruntConfig](configCacheName))
	ctx = context.WithValue(ctx, EvalCtxCacheContextKey, cache.NewCache[*hcl.EvalContext](evalCtxCacheName))
	ctx = context.WithValue(ctx, RunCmdCacheContextKey, cache.NewCache[string](runCmdCacheName))
	ctx = context.WithValue(ctx, DependencyOutputCacheContextKey, cache.NewCache[*dependencyOutputCache](dependencyOutputCacheName))
	ctx = context.WithValue(ctx, OptsCacheContextKey, cache.NewCache[*options.TerragruntOptions](optsCacheName))

	return ctx
}
