package shell

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/options"
)

const (
	TerraformCommandContextKey ctxKey = iota
	RunCmdCacheContextKey      ctxKey = iota

	runCmdCacheName = "runCmdCache"
)

type ctxKey byte

// RunShellCommandFunc is a context value for `TerraformCommandContextKey` key, used to intercept shell commands.
type RunShellCommandFunc func(ctx context.Context, opts *options.TerragruntOptions, args cli.Args) (*util.CmdOutput, error)

func ContextWithTerraformCommandHook(ctx context.Context, fn RunShellCommandFunc) context.Context {
	ctx = context.WithValue(ctx, RunCmdCacheContextKey, cache.NewCache[string](runCmdCacheName))
	return context.WithValue(ctx, TerraformCommandContextKey, fn)
}

// TerraformCommandHookFromContext returns `RunShellCommandFunc` from the context if it has been set, otherwise returns nil.
func TerraformCommandHookFromContext(ctx context.Context) RunShellCommandFunc {
	if val := ctx.Value(TerraformCommandContextKey); val != nil {
		if val, ok := val.(RunShellCommandFunc); ok {
			return val
		}
	}

	return nil
}
