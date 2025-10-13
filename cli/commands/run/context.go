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
// If a cache already exists in the context, it reuses it. Otherwise, creates a new one.
func WithRunVersionCache(ctx context.Context) context.Context {
	// Check if cache already exists in context
	if existingCache, ok := ctx.Value(versionCacheContextKey).(*cache.Cache[string]); ok && existingCache != nil {
		return ctx // Reuse existing cache
	}

	// Create new cache if none exists
	ctx = context.WithValue(ctx, versionCacheContextKey, cache.NewCache[string](versionCacheName))
	return ctx
}

// GetRunVersionCache retrieves the version cache from the context for the run package.
func GetRunVersionCache(ctx context.Context) *cache.Cache[string] {
	return cache.ContextCache[string](ctx, versionCacheContextKey)
}

// ClearVersionCache clears the version cache from the context. Useful during testing.
func ClearVersionCache(ctx context.Context) {
	cache.ContextCache[string](ctx, versionCacheContextKey).Clear()
}
