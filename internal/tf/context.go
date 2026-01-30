package tf

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	TerraformCommandContextKey ctxKey = iota
	DetailedExitCodeContextKey
)

type ctxKey byte

// RunShellCommandFunc is a context value for `TerraformCommandContextKey` key, used to intercept shell commands.
type RunShellCommandFunc func(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, args clihelper.Args) (*util.CmdOutput, error)

func ContextWithTerraformCommandHook(ctx context.Context, fn RunShellCommandFunc) context.Context {
	ctx = cache.ContextWithCache(ctx)
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

// ContextWithDetailedExitCode returns a new context containing the given DetailedExitCodeMap.
func ContextWithDetailedExitCode(ctx context.Context, detailedExitCode *DetailedExitCodeMap) context.Context {
	return context.WithValue(ctx, DetailedExitCodeContextKey, detailedExitCode)
}

// DetailedExitCodeFromContext returns DetailedExitCodeMap if the given context contains it.
func DetailedExitCodeFromContext(ctx context.Context) *DetailedExitCodeMap {
	if val := ctx.Value(DetailedExitCodeContextKey); val != nil {
		if val, ok := val.(*DetailedExitCodeMap); ok {
			return val
		}
	}

	return nil
}
