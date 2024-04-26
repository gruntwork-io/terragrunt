package shell

import (
	"context"

	"github.com/gruntwork-io/terragrunt/options"
)

const TerraformCommandContextKey ctxKey = iota

type ctxKey byte

type RunShellCommandFunc func(ctx context.Context, opts *options.TerragruntOptions, args []string) (*CmdOutput, error)

func ContextWithTerraformCommandHook(ctx context.Context, fn RunShellCommandFunc) context.Context {
	return context.WithValue(ctx, TerraformCommandContextKey, fn)
}

func TerraformCommandHookFromContext(ctx context.Context) RunShellCommandFunc {
	if val := ctx.Value(TerraformCommandContextKey); val != nil {
		if val, ok := val.(RunShellCommandFunc); ok {
			return val
		}
	}

	return nil
}
