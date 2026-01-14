package config

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

type configKey byte

const (
	HclCacheContextKey              configKey = iota
	TerragruntConfigCacheContextKey configKey = iota
	RunCmdCacheContextKey           configKey = iota
	DependencyOutputCacheContextKey configKey = iota
	TerragruntRunnerContextKey      configKey = iota

	hclCacheName              = "hclCache"
	configCacheName           = "configCache"
	runCmdCacheName           = "runCmdCache"
	dependencyOutputCacheName = "dependencyOutputCache"
)

// WithConfigValues add to context default values for configuration.
func WithConfigValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, HclCacheContextKey, cache.NewCache[*hclparse.File](hclCacheName))
	ctx = context.WithValue(ctx, TerragruntConfigCacheContextKey, cache.NewCache[*TerragruntConfig](configCacheName))
	ctx = context.WithValue(ctx, RunCmdCacheContextKey, cache.NewCache[string](runCmdCacheName))
	ctx = context.WithValue(ctx, DependencyOutputCacheContextKey, cache.NewCache[*dependencyOutputCache](dependencyOutputCacheName))

	return ctx
}

// WithTerragruntRunner adds a TerragruntRunner to the context.
func WithTerragruntRunner(ctx context.Context, runner runcfg.TerragruntRunner) context.Context {
	return context.WithValue(ctx, TerragruntRunnerContextKey, runner)
}

// GetTerragruntRunner retrieves the TerragruntRunner from the context.
func GetTerragruntRunner(ctx context.Context) runcfg.TerragruntRunner {
	if runner, ok := ctx.Value(TerragruntRunnerContextKey).(runcfg.TerragruntRunner); ok {
		return runner
	}

	return nil
}
