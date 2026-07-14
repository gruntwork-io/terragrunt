package run

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/getter"
)

type configKey byte

const (
	versionCacheContextKey configKey = iota
	moduleVersionResolverContextKey
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

// WithModuleVersionResolver installs a shared tfr:// version-constraint
// resolver so that units sharing a module source and constraint query the
// registry once per run instead of once each.
func WithModuleVersionResolver(ctx context.Context) context.Context {
	return context.WithValue(ctx, moduleVersionResolverContextKey, getter.NewVersionResolver())
}

// ModuleVersionResolverFromContext returns the resolver installed by
// [WithModuleVersionResolver]. If none was installed, it returns a fresh
// resolver whose memoization is scoped to the caller alone.
func ModuleVersionResolverFromContext(ctx context.Context) *getter.VersionResolver {
	if resolver, ok := ctx.Value(moduleVersionResolverContextKey).(*getter.VersionResolver); ok {
		return resolver
	}

	return getter.NewVersionResolver()
}
