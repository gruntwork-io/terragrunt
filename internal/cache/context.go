package cache

import (
	"context"
)

const (
	// RunCmdCacheContextKey is the context key used to store and retrieve the run command cache
	RunCmdCacheContextKey ctxKey = iota

	// runCmdCacheName is the identifier for the run command cache instance
	runCmdCacheName = "runCmdCache"
)

// ctxKey is a type-safe context key type to prevent key collisions
type ctxKey byte

func ContextWithCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, RunCmdCacheContextKey, NewCache[string](runCmdCacheName))
}
