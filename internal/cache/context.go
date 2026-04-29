package cache

import (
	"context"
)

const (
	// RunCmdCacheContextKey is the context key used to store and retrieve the run command cache
	RunCmdCacheContextKey ctxKey = iota

	// RepoRootCacheContextKey is the context key for the repo-root cache.
	RepoRootCacheContextKey

	// runCmdCacheName is the identifier for the run command cache instance
	runCmdCacheName = "runCmdCache"

	// repoRootCacheName is the identifier for the repo-root cache instance
	repoRootCacheName = "repoRootCache"
)

// ctxKey is a type-safe context key type to prevent key collisions
type ctxKey byte

func ContextWithCache(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, RunCmdCacheContextKey, NewCache[string](runCmdCacheName))
	ctx = context.WithValue(ctx, RepoRootCacheContextKey, NewRepoRootCache(repoRootCacheName))

	return ctx
}
