package config

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/cache"
)

type configKey byte

const (
	// HCLCacheContextKey is the key for the HCL cache in the context.
	HCLCacheContextKey configKey = iota
	// TerragruntConfigCacheContextKey is the key for the Terragrunt config cache in the context.
	TerragruntConfigCacheContextKey configKey = iota
	// RunCmdCacheContextKey is the key for the run command cache in the context.
	RunCmdCacheContextKey configKey = iota
	// DependencyOutputCacheContextKey is the key for the dependency output cache in the context.
	DependencyOutputCacheContextKey configKey = iota

	hclCacheName              = "hclCache"
	configCacheName           = "configCache"
	runCmdCacheName           = "runCmdCache"
	dependencyOutputCacheName = "dependencyOutputCache"
)

// WithConfigValues add to context default values for configuration.
func WithConfigValues(ctx context.Context) context.Context {
	ctx = context.WithValue(
		ctx,
		HCLCacheContextKey,
		cache.NewCache[*hclparse.File](hclCacheName),
	)
	ctx = context.WithValue(
		ctx,
		TerragruntConfigCacheContextKey,
		cache.NewCache[*TerragruntConfig](configCacheName),
	)
	ctx = context.WithValue(
		ctx,
		RunCmdCacheContextKey,
		cache.NewCache[string](runCmdCacheName),
	)
	ctx = context.WithValue(
		ctx,
		DependencyOutputCacheContextKey,
		cache.NewCache[*dependencyOutputCache](dependencyOutputCacheName),
	)

	return ctx
}
