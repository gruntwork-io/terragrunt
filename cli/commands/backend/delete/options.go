package delete

import (
	"context"

	"github.com/gruntwork-io/terragrunt/options"
)

type Options struct {
	*options.TerragruntOptions

	// DeleteBucket determines whether to delete entire bucket.
	DeleteBucket bool
}

func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: opts,
	}
}

func (opts *Options) OptionsFromContext(ctx context.Context) *Options {
	opts.TerragruntOptions = opts.TerragruntOptions.OptionsFromContext(ctx)

	return opts
}
