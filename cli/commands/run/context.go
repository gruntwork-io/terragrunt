package run

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cache"
)

// WithRunVersionCache initializes the version cache in the context for the run package.
func WithRunVersionCache(ctx context.Context) context.Context {
	return config.WithVersionCache(ctx)
}

// GetRunVersionCache retrieves the version cache from the context for the run package.
func GetRunVersionCache(ctx context.Context) *cache.Cache[string] {
	return cache.ContextCache[string](ctx, config.TerraformVersionCacheContextKey)
}
