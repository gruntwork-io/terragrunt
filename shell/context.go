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
	RunCmdCacheContextKey
	DetailedExitCodeContextKey

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

// ContextWithDetailedExitCode returns a new context containing the given DetailedExitCode.
func ContextWithDetailedExitCode(ctx context.Context, detailedExitCode *DetailedExitCode) context.Context {
	return context.WithValue(ctx, DetailedExitCodeContextKey, detailedExitCode)
}

// DetailedExitCodeFromContext returns DetailedExitCode if the give context contains it.
func DetailedExitCodeFromContext(ctx context.Context) *DetailedExitCode {
	if val := ctx.Value(DetailedExitCodeContextKey); val != nil {
		if val, ok := val.(*DetailedExitCode); ok {
			return val
		}
	}

	return nil
}
