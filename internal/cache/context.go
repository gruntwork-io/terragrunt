package cache

import (
	"context"
)

const (
	RunCmdCacheContextKey ctxKey = iota

	runCmdCacheName = "runCmdCache"
)

type ctxKey byte

func ContextWithCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, RunCmdCacheContextKey, NewCache[string](runCmdCacheName))
}
