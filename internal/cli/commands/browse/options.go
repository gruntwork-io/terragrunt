package browse

import (
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

type Options struct {
	*options.TerragruntOptions
}

func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: opts,
	}
}
