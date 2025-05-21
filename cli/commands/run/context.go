package run

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cache"
)

type configKey byte

const (
	versionCacheContextKey configKey = iota
	versionCacheName                 = "versionCache"
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
