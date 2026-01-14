package run

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
)

type configKey byte

const (
	versionCacheContextKey configKey = iota
	dependentUnitsFinderContextKey
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
func WithDependentModulesFinder(ctx context.Context, finder runcfg.DependentUnitsFinder) context.Context {
	return context.WithValue(ctx, dependentUnitsFinderContextKey, finder)
}

// GetDependentUnitsFinder retrieves the DependentModulesFinder from the context.
//
// We store this finder in context purely to bypass a circular dependency import between the `config` package and
// the `run` package. This isn't ideal, but it's the best we can do for now without further major refactoring.
//
// We can probably move the destroy check higher up such that we don't need to be performing this lookup right before
// performing a run.
func GetDependentUnitsFinder(ctx context.Context) runcfg.DependentUnitsFinder {
	if finder, ok := ctx.Value(dependentUnitsFinderContextKey).(runcfg.DependentUnitsFinder); ok {
		return finder
	}

	return nil
}
