package terraform

import (
	"context"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const retryContextKey ctxKey = iota

type ctxKey byte

type RetryCallback func(ctx context.Context, opts *options.TerragruntOptions) (*shell.CmdOutput, error)

type Retry struct {
	HookFunc func(ctx context.Context, opts *options.TerragruntOptions, callback RetryCallback) (*shell.CmdOutput, error)
}

func (retry *Retry) do(ctx context.Context, opts *options.TerragruntOptions, callback RetryCallback) (*shell.CmdOutput, error) {
	if hookFn := retry.HookFunc; hookFn != nil {
		return hookFn(ctx, opts, callback)
	}

	return callback(ctx, opts)
}

func (retry *Retry) Run(ctx context.Context, opts *options.TerragruntOptions, callback RetryCallback) error {
	// Retry the command configurable time with sleep in between
	for i := 0; i < opts.RetryMaxAttempts; i++ {
		if out, err := retry.do(ctx, opts, callback); err != nil {
			if out == nil || !isRetryable(opts, out) {
				opts.Logger.Errorf("%s invocation failed in %s", opts.TerraformImplementation, opts.WorkingDir)
				return err
			} else {
				opts.Logger.Infof("Encountered an error eligible for retrying. Sleeping %v before retrying.\n", opts.RetrySleepIntervalSec)
				time.Sleep(opts.RetrySleepIntervalSec)
			}
		} else {
			return nil
		}
	}

	return errors.WithStackTrace(MaxRetriesExceeded{opts})
}

func ContextWithRetry(ctx context.Context, val *Retry) context.Context {
	return context.WithValue(ctx, retryContextKey, val)
}

func RetryFromContext(ctx context.Context) *Retry {
	if val := ctx.Value(retryContextKey); val != nil {
		if val, ok := val.(*Retry); ok {
			return val
		}
	}

	return new(Retry)
}

// isRetryable checks whether there was an error and if the output matches any of the configured RetryableErrors
func isRetryable(opts *options.TerragruntOptions, out *shell.CmdOutput) bool {
	if !opts.AutoRetry {
		return false
	}
	// When -json is enabled, Terraform will send all output, errors included, to stdout.
	return util.MatchesAny(opts.RetryableErrors, out.Stderr) || util.MatchesAny(opts.RetryableErrors, out.Stdout)
}
