package run

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
)

type configKey byte

const (
	versionCacheContextKey configKey = iota
	dependentModulesFinderContextKey
	versionCacheName = "versionCache"
)

// WithRunVersionCache initializes the version cache in the context for the run package.
func WithRunVersionCache(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, versionCacheContextKey, cache.NewCache[string](versionCacheName))
	return ctx
}

// GetRunVersionCache retrieves the version cache from the context for the run package.
func GetRunVersionCache(ctx context.Context) *cache.Cache[string] {
	return cache.ContextCache[string](ctx, versionCacheContextKey)
}

// WithDependentModulesFinder adds a DependentModulesFinder to the context.
func WithDependentModulesFinder(ctx context.Context, finder runcfg.DependentModulesFinder) context.Context {
	return context.WithValue(ctx, dependentModulesFinderContextKey, finder)
}

// GetDependentModulesFinder retrieves the DependentModulesFinder from the context.
func GetDependentModulesFinder(ctx context.Context) runcfg.DependentModulesFinder {
	if finder, ok := ctx.Value(dependentModulesFinderContextKey).(runcfg.DependentModulesFinder); ok {
		return finder
	}

	return nil
}
