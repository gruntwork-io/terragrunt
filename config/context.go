package config

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/cache"
)

const (
	HclCacheContextKey              configKey = iota
	TerragruntConfigCacheContextKey configKey = iota
	RunCmdCacheContextKey           configKey = iota
	DependencyOutputCacheContextKey configKey = iota
)

// WithConfigValues add to context default values for configuration.
func WithConfigValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, HclCacheContextKey, cache.NewCache[*hclparse.File]("hclCache"))
	ctx = context.WithValue(ctx, TerragruntConfigCacheContextKey, cache.NewCache[*TerragruntConfig]("configCache"))
	ctx = context.WithValue(ctx, RunCmdCacheContextKey, cache.NewCache[string]("runCmdCache"))
	ctx = context.WithValue(ctx, DependencyOutputCacheContextKey, cache.NewCache[*dependencyOutputCache]("dependencyOutputCache"))
	return ctx
}
